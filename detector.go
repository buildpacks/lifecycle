package lifecycle

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

const (
	CodeDetectPass  = 0
	CodeDetectFail  = 100
	EnvBuildpackDir = "CNB_BUILDPACK_DIR"
)

var errFailedDetection = errors.New("no buildpacks participating")
var errBuildpack = errors.New("buildpack(s) failed with err")
var errInconsistentVersion = `buildpack %s has a "version" key that does not match "metadata.version"`
var errDoublySpecifiedVersions = `buildpack %s has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`
var warnTopLevelVersion = `Warning: buildpack %s has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`
var errInvalidRequirementsBuildpack = `priviledged buildpack %s has defined "requires", which is not allowed.`

type BuildPlan struct {
	Entries []BuildPlanEntry `toml:"entries"`
}

type BuildPlanEntry struct {
	Providers []Buildpack `toml:"providers"`
	Requires  []Require   `toml:"requires"`
}

func (be BuildPlanEntry) noOpt() BuildPlanEntry {
	var out []Buildpack
	for _, p := range be.Providers {
		out = append(out, p.noOpt().noAPI())
	}
	be.Providers = out
	return be
}

type Require struct {
	Name     string                 `toml:"name" json:"name"`
	Version  string                 `toml:"version,omitempty" json:"version,omitempty"`
	Mixin    bool                   `toml:"mixin,omitempty" json:"mixin,omitempty"`
	Metadata map[string]interface{} `toml:"metadata" json:"metadata"`
}

func (r *Require) parseName() (string, string) {
	parts := strings.SplitN(r.Name, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

func (p *Provide) parseName() (string, string) {
	parts := strings.SplitN(p.Name, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

func (r *Require) convertMetadataToVersion() {
	if version, ok := r.Metadata["version"]; ok {
		r.Version = fmt.Sprintf("%v", version)
	}
}

func (r *Require) convertVersionToMetadata() {
	if r.Version != "" {
		if r.Metadata == nil {
			r.Metadata = make(map[string]interface{})
		}
		r.Metadata["version"] = r.Version
		r.Version = ""
	}
}

func (r *Require) hasInconsistentVersions() bool {
	if version, ok := r.Metadata["version"]; ok {
		return r.Version != "" && r.Version != version
	}
	return false
}

func (r *Require) hasDoublySpecifiedVersions() bool {
	if _, ok := r.Metadata["version"]; ok {
		return r.Version != ""
	}
	return false
}

func (r *Require) hasTopLevelVersions() bool {
	return r.Version != ""
}

type depKey struct {
	name  string
	mixin bool
	any   bool
}

func (r *Require) depKey() depKey {
	_, name := r.parseName()
	return depKey{
		name,
		r.Mixin,
		false,
	}
}

func (p *Provide) depKey() depKey {
	if p.Any {
		return anyMixinKey
	}
	_, name := p.parseName()
	return depKey{
		name,
		p.Mixin,
		false,
	}
}

type Provide struct {
	Name  string `toml:"name"`
	Mixin bool   `toml:"mixin,omitempty" json:"mixin,omitempty"`
	Any   bool   `toml:"any,omitempty" json:"any,omitempty"`
}

type DetectConfig struct {
	FullEnv            []string
	ClearEnv           []string
	AppDir             string
	PlatformDir        string
	BuildpacksDir      string
	StackBuildpacksDir string
	Logger             Logger
	runs               *sync.Map
}

func (c *DetectConfig) process(done []Buildpack) (*DetectResult, error) {
	var runs []DetectRun
	for _, bp := range done {
		t, ok := c.runs.Load(bp.String())
		if !ok {
			return nil, errors.Errorf("missing detection of '%s'", bp)
		}
		run := t.(DetectRun)
		outputLogf := c.Logger.Debugf

		switch run.Code {
		case CodeDetectPass, CodeDetectFail:
		default:
			outputLogf = c.Logger.Infof
		}

		if len(run.Output) > 0 {
			outputLogf("======== Output: %s ========", bp)
			outputLogf(string(run.Output))
		}
		if run.Err != nil {
			outputLogf("======== Error: %s ========", bp)
			outputLogf(run.Err.Error())
		}
		runs = append(runs, run)
	}

	c.Logger.Debugf("======== Results ========")

	results := detectResults{}
	detected := true
	buildpackErr := false
	for i, bp := range done {
		run := runs[i]
		switch run.Code {
		case CodeDetectPass:
			c.Logger.Debugf("pass: %s", bp)
			results = append(results, detectResult{bp, run})
		case CodeDetectFail:
			if bp.Optional {
				c.Logger.Debugf("skip: %s", bp)
			} else {
				c.Logger.Debugf("fail: %s", bp)
			}
			detected = detected && bp.Optional
		case -1:
			c.Logger.Infof("err:  %s", bp)
			buildpackErr = true
			detected = detected && bp.Optional
		default:
			c.Logger.Infof("err:  %s (%d)", bp, run.Code)
			buildpackErr = true
			detected = detected && bp.Optional
		}
	}

	if len(results) > 0 && detected {
		detected = false
		for _, res := range results {
			if !res.Buildpack.Privileged {
				detected = true
				break
			}
		}
	}

	if !detected {
		if buildpackErr {
			return nil, errBuildpack
		}
		return nil, errFailedDetection
	}

	i := 0
	result, err := results.runTrials(func(trial detectTrial) (*trialResult, error) {
		i++
		return c.runTrial(i, trial)
	})
	if err != nil {
		return nil, err
	}

	participatingBuildpacks := append(result.PrivOptions, result.BuildOptions...)

	if len(done) != len(participatingBuildpacks) {
		c.Logger.Infof("%d of %d buildpacks participating", len(participatingBuildpacks), len(done))
	}

	maxLength := 0
	for _, bp := range participatingBuildpacks {
		l := len(bp.ID)
		if l > maxLength {
			maxLength = l
		}
	}

	f := fmt.Sprintf("%%-%ds %%s", maxLength)

	for _, t := range participatingBuildpacks {
		c.Logger.Infof(f, t.ID, t.Version)
	}

	// TODO: can we do this earlier?
	var found []Buildpack
	for _, r := range result.BuildOptions {
		found = append(found, r.Buildpack.noOpt())
	}
	var privFound []Buildpack
	for _, r := range result.PrivOptions {
		privFound = append(privFound, r.Buildpack.noOpt())
	}
	var plan []BuildPlanEntry
	for _, dep := range result.BuildDeps {
		plan = append(plan, dep.BuildPlanEntry.noOpt())
	}

	var runFound []Buildpack
	for _, r := range result.RunOptions {
		runFound = append(runFound, r.Buildpack.noOpt())
	}
	var runPlan []BuildPlanEntry
	for _, dep := range result.RunDeps {
		runPlan = append(runPlan, dep.BuildPlanEntry.noOpt())
	}

	return &DetectResult{
		BuildGroup:           BuildpackGroup{found},
		BuildPrivilegedGroup: BuildpackGroup{privFound},
		BuildPlan:            BuildPlan{plan},
		RunGroup:             BuildpackGroup{runFound},
		RunPlan:              BuildPlan{runPlan},
	}, nil
}

type trialResult struct {
	BuildDeps    depMap
	BuildOptions []detectOption
	PrivOptions  []detectOption
	RunDeps      depMap
	RunOptions   []detectOption
}

func (c *DetectConfig) runTrial(i int, trial detectTrial) (*trialResult, error) {
	c.Logger.Debugf("Resolving plan... (try #%d)", i)

	buildTrial := append([]detectOption{}, trial...)
	buildDeps, buildOptions, privOptions, err := c.runTrialForStage(buildTrial, "")
	if err != nil {
		return nil, err
	}

	runTrial := append([]detectOption{}, trial...)
	runDeps, _, runOptions, err := c.runTrialForStage(runTrial, "run")
	if err != nil {
		return nil, err
	}

	if len(buildTrial) == 0 && len(runTrial) == 0 {
		c.Logger.Debugf("fail: no viable buildpacks in group")
		return nil, errFailedDetection
	}

	return &trialResult{
		BuildDeps:    buildDeps,
		BuildOptions: buildOptions,
		PrivOptions:  privOptions,

		RunDeps:    runDeps,
		RunOptions: runOptions,
	}, nil
}

func (c *DetectConfig) runTrialForStage(trial detectTrial, stage string) (depMap, []detectOption, []detectOption, error) {
	var depMap depMap
	loggedStage := ""
	retry := true
	for retry {
		retry = false
		if stage == "run" {
			loggedStage = "[run]"
			depMap = newRunDepMap(trial)
		} else {
			depMap = newBuildDepMap(trial)
		}

		if err := depMap.eachUnmetRequire(func(name string, bp Buildpack) error {
			retry = true
			if !bp.Optional {
				c.Logger.Debugf("fail: %s%s requires %s", bp, loggedStage, name)
				return errFailedDetection
			}
			c.Logger.Debugf("skip: %s%s requires %s", bp, loggedStage, name)
			trial = trial.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, nil, err
		}

		if err := depMap.eachUnmetProvide(func(name string, bp Buildpack) error {
			if bp.Privileged {
				return nil
			}
			retry = true
			if !bp.Optional && !bp.Privileged {
				c.Logger.Debugf("fail: %s%s provides unused %s", bp, loggedStage, name)
				return errFailedDetection
			}
			c.Logger.Debugf("skip: %s%s provides unused %s", bp, loggedStage, name)
			trial = trial.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, nil, err
		}
	}

	depMap.eachUnusedPrivilegedBuildpack(func(bp Buildpack) {
		c.Logger.Debugf("skip: %s%s provides unused deps", bp, loggedStage)
		trial = trial.remove(bp)
	})

	options := []detectOption{}
	privOptions := []detectOption{}

	for _, detectOption := range trial {
		if detectOption.Privileged {
			privOptions = append(privOptions, detectOption)
		} else {
			options = append(options, detectOption)
		}
	}

	return depMap, options, privOptions, nil
}

func (bp *BuildpackTOML) Detect(c *DetectConfig) DetectRun {
	appDir, err := filepath.Abs(c.AppDir)
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}
	platformDir, err := filepath.Abs(c.PlatformDir)
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}
	defer os.RemoveAll(planDir)

	planPath := filepath.Join(planDir, "plan.toml")
	if err := ioutil.WriteFile(planPath, nil, 0777); err != nil {
		return DetectRun{Code: -1, Err: err}
	}

	out := &bytes.Buffer{}
	cmd := exec.Command(
		filepath.Join(bp.Path, "bin", "detect"),
		platformDir,
		planPath,
	)
	cmd.Dir = appDir
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Env = c.FullEnv
	if bp.Buildpack.ClearEnv {
		cmd.Env = c.ClearEnv
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+bp.Path)

	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return DetectRun{Code: status.ExitStatus(), Output: out.Bytes()}
			}
		}
		return DetectRun{Code: -1, Err: err, Output: out.Bytes()}
	}
	var t DetectRun
	if _, err := toml.DecodeFile(planPath, &t); err != nil {
		return DetectRun{Code: -1, Err: err}
	}

	if bp.Buildpack.Privileged {
		if len(t.Requires) > 0 {
			return DetectRun{Code: -1, Err: errors.Errorf(errInvalidRequirementsBuildpack, bp.Buildpack.ID)}
		}
	}

	if api.MustParse(bp.API).Equal(api.MustParse("0.2")) {
		if t.hasInconsistentVersions() || t.Or.hasInconsistentVersions() {
			t.Err = errors.Errorf(errInconsistentVersion, bp.Buildpack.ID)
			t.Code = -1
		}
	}
	if api.MustParse(bp.API).Compare(api.MustParse("0.3")) >= 0 {
		if t.hasDoublySpecifiedVersions() || t.Or.hasDoublySpecifiedVersions() {
			t.Err = errors.Errorf(errDoublySpecifiedVersions, bp.Buildpack.ID)
			t.Code = -1
		}
	}
	if api.MustParse(bp.API).Compare(api.MustParse("0.3")) >= 0 {
		if t.hasTopLevelVersions() || t.Or.hasTopLevelVersions() {
			c.Logger.Warnf(warnTopLevelVersion, bp.Buildpack.ID)
		}
	}
	t.Output = out.Bytes()
	return t
}

type BuildpackGroup struct {
	Group []Buildpack `toml:"group"`
}

func (bg BuildpackGroup) detect(done []Buildpack, wg *sync.WaitGroup, c *DetectConfig) (*DetectResult, error) {
	for i, bp := range bg.Group {
		key := bp.String()
		if hasID(done, bp.ID) {
			continue
		}
		var (
			err  error
			info *BuildpackTOML
		)
		bpDir := c.BuildpacksDir
		if bp.Privileged {
			bpDir = c.StackBuildpacksDir
		}
		info, err = bp.Lookup(bpDir)
		if err != nil {
			return nil, err
		}
		bp.API = info.API
		if info.Order != nil {
			// TODO: double-check slice safety here
			// FIXME: cyclical references lead to infinite recursion
			return info.Order.detect(done, bg.Group[i+1:], bp.Optional, wg, c)
		}
		done = append(done, bp)
		wg.Add(1)
		go func() {
			if _, ok := c.runs.Load(key); !ok {
				c.runs.Store(key, info.Detect(c))
			}
			wg.Done()
		}()
	}

	wg.Wait()

	return c.process(done)
}

func (bg BuildpackGroup) append(group ...BuildpackGroup) BuildpackGroup {
	for _, g := range group {
		bg.Group = append(bg.Group, g.Group...)
	}
	return bg
}

type BuildpackOrder []BuildpackGroup

type DetectResult struct {
	BuildGroup           BuildpackGroup
	BuildPrivilegedGroup BuildpackGroup
	BuildPlan            BuildPlan
	RunGroup             BuildpackGroup
	RunPlan              BuildPlan
}

func (bo BuildpackOrder) Detect(c *DetectConfig) (*DetectResult, error) {
	if c.runs == nil {
		c.runs = &sync.Map{}
	}

	dr, err := bo.detect(nil, nil, false, &sync.WaitGroup{}, c)
	if err == errBuildpack {
		err = NewLifecycleError(err, ErrTypeBuildpack)
	} else if err == errFailedDetection {
		err = NewLifecycleError(err, ErrTypeFailedDetection)
	}

	if dr != nil {
		for i := range dr.BuildPlan.Entries {
			for j := range dr.BuildPlan.Entries[i].Requires {
				dr.BuildPlan.Entries[i].Requires[j].convertVersionToMetadata()
			}
		}

		for i := range dr.RunPlan.Entries {
			for j := range dr.RunPlan.Entries[i].Requires {
				dr.RunPlan.Entries[i].Requires[j].convertVersionToMetadata()
			}
		}
	}

	return dr, err
}

func (bo BuildpackOrder) detect(done, next []Buildpack, optional bool, wg *sync.WaitGroup, c *DetectConfig) (*DetectResult, error) {
	ngroup := BuildpackGroup{Group: next}
	buildpackErr := false
	for _, group := range bo {
		// FIXME: double-check slice safety here
		tr, err := group.append(ngroup).detect(done, wg, c)
		if err == errBuildpack {
			buildpackErr = true
		}
		if err == errFailedDetection || err == errBuildpack {
			wg = &sync.WaitGroup{}
			continue
		}
		return tr, err
	}
	if optional {
		return ngroup.detect(done, wg, c)
	}

	if buildpackErr {
		return nil, errBuildpack
	}
	return nil, errFailedDetection
}

func hasID(bps []Buildpack, id string) bool {
	for _, bp := range bps {
		if bp.ID == id {
			return true
		}
	}
	return false
}

type DetectRun struct {
	planSections
	Or     planSectionsList `toml:"or"`
	Output []byte           `toml:"-"`
	Code   int              `toml:"-"`
	Err    error            `toml:"-"`
}

type planSections struct {
	Requires []Require `toml:"requires"`
	Provides []Provide `toml:"provides"`
}

func (p *planSections) hasInconsistentVersions() bool {
	for _, req := range p.Requires {
		if req.hasInconsistentVersions() {
			return true
		}
	}
	return false
}

func (p *planSections) hasDoublySpecifiedVersions() bool {
	for _, req := range p.Requires {
		if req.hasDoublySpecifiedVersions() {
			return true
		}
	}
	return false
}

func (p *planSections) hasTopLevelVersions() bool {
	for _, req := range p.Requires {
		if req.hasTopLevelVersions() {
			return true
		}
	}
	return false
}

type planSectionsList []planSections

func (p *planSectionsList) hasInconsistentVersions() bool {
	for _, planSection := range *p {
		if planSection.hasInconsistentVersions() {
			return true
		}
	}
	return false
}

func (p *planSectionsList) hasDoublySpecifiedVersions() bool {
	for _, planSection := range *p {
		if planSection.hasDoublySpecifiedVersions() {
			return true
		}
	}
	return false
}

func (p *planSectionsList) hasTopLevelVersions() bool {
	for _, planSection := range *p {
		if planSection.hasTopLevelVersions() {
			return true
		}
	}
	return false
}

type detectResult struct {
	Buildpack
	DetectRun
}

func (r *detectResult) options() []detectOption {
	var out []detectOption
	for i, sections := range append([]planSections{r.planSections}, r.Or...) {
		bp := r.Buildpack
		bp.Optional = bp.Optional && i == len(r.Or)
		out = append(out, detectOption{bp, sections})
	}
	return out
}

type detectResults []detectResult
type trialFunc func(detectTrial) (*trialResult, error)

func (rs detectResults) runTrials(f trialFunc) (*trialResult, error) {
	return rs.runTrialsFrom(nil, f)
}

func (rs detectResults) runTrialsFrom(prefix detectTrial, f trialFunc) (*trialResult, error) {
	if len(rs) == 0 {
		return f(prefix)
	}

	var lastErr error
	for _, option := range rs[0].options() {
		result, err := rs[1:].runTrialsFrom(append(prefix, option), f)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

type detectOption struct {
	Buildpack
	planSections
}

type detectTrial []detectOption

func (ts detectTrial) remove(bp Buildpack) detectTrial {
	var out detectTrial
	for _, t := range ts {
		if t.Buildpack != bp {
			out = append(out, t)
		}
	}
	return out
}

type depEntry struct {
	BuildPlanEntry
	earlyRequires []Buildpack
	extraProvides []Buildpack
}

type depMap map[depKey]depEntry

var anyMixinKey depKey = depKey{name: "any", mixin: true, any: true}

func newBuildDepMap(trial detectTrial) depMap {
	m := depMap{}
	stage := "build"
	for _, option := range trial {
		for _, p := range option.Provides {
			pstage, _ := p.parseName()
			if pstage == "" || pstage == stage {
				m.provide(option.Buildpack, p)
			}
		}
		for _, r := range option.Requires {
			rstage, _ := r.parseName()
			if rstage == "" || rstage == stage {
				m.require(option.Buildpack, r)
			}
		}
	}
	return m
}

func newRunDepMap(trial detectTrial) depMap {
	m := depMap{}
	stage := "run"
	for _, option := range trial {
		for _, p := range option.Provides {
			// only privileged buildpacks can provide something for run image extension
			if !option.Buildpack.Privileged {
				continue
			}

			pstage, _ := p.parseName()
			if pstage == "" || pstage == stage {
				m.provide(option.Buildpack, p)
			}
		}
		for _, r := range option.Requires {
			// buildpacks can only require something for run image that is _not_ a mixin
			if !r.Mixin {
				continue
			}
			rstage, _ := r.parseName()
			if rstage == "" || rstage == stage {
				m.require(option.Buildpack, r)
			}
		}
	}
	return m
}

func (m depMap) provide(bp Buildpack, provide Provide) {
	entry := m[provide.depKey()]
	entry.extraProvides = append(entry.extraProvides, bp)
	m[provide.depKey()] = entry
}

func (m depMap) require(bp Buildpack, require Require) {
	reqKey := require.depKey()
	if require.Mixin && len(m[anyMixinKey].extraProvides) != 0 {
		reqKey = anyMixinKey
	}
	entry := m[reqKey]
	entry.Providers = append(entry.Providers, entry.extraProvides...)
	entry.extraProvides = nil

	if len(entry.Providers) == 0 {
		entry.earlyRequires = append(entry.earlyRequires, bp)
	} else {
		entry.Requires = append(entry.Requires, require)
	}
	m[reqKey] = entry
}

func (m depMap) eachUnmetProvide(f func(name string, bp Buildpack) error) error {
	for key, entry := range m {
		if len(entry.extraProvides) != 0 {
			for _, bp := range entry.extraProvides {
				if err := f(key.name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m depMap) eachUnmetRequire(f func(name string, bp Buildpack) error) error {
	for key, entry := range m {
		if len(entry.earlyRequires) != 0 {
			for _, bp := range entry.earlyRequires {
				if err := f(key.name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m depMap) eachUnusedPrivilegedBuildpack(f func(bp Buildpack)) {
	candidateRemovals := make(map[string]Buildpack)
	candidateRemovals2 := make(map[string]depKey)
	for k, entry := range m {
		if len(entry.extraProvides) != 0 {
			for _, bp := range entry.extraProvides {
				candidateRemovals[bp.ID] = bp
				candidateRemovals2[bp.ID] = k
			}
		}
	}

	for _, entry := range m {
		for _, bp := range entry.Providers {
			delete(candidateRemovals, bp.ID)
		}
	}

	// TODO: this feels...bad
	for _, bp := range candidateRemovals {
		f(bp)
		delete(m, candidateRemovals2[bp.ID])
	}
}
