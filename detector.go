package lifecycle

import (
	"fmt"
	"os"
	"sync"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/platform"
)

const (
	CodeDetectPass = 0
	CodeDetectFail = 100
)

var (
	ErrFailedDetection = errors.New("no buildpacks participating")
	ErrBuildpack       = errors.New("buildpack(s) failed with err")
)

type Resolver interface {
	Resolve(done []buildpack.GroupBuildable, detectRuns *sync.Map) ([]buildpack.GroupBuildable, []platform.BuildPlanEntry, error)
}

type Detector struct {
	buildpack.DetectConfig
	Platform       Platform
	Resolver       Resolver
	Runs           *sync.Map
	BuildpackStore BuildableStore
	ExtensionStore BuildableStore
}

func NewDetector(config buildpack.DetectConfig, buildpacksDir string, extensionsDir string, platform Platform) (*Detector, error) {
	resolver := &DefaultResolver{
		Logger: config.Logger,
	}
	bpStore, err := buildpack.NewBuildpackStore(buildpacksDir)
	if err != nil {
		return nil, err
	}
	var extStore BuildableStore
	if extensionsDir != "" {
		extStore, err = buildpack.NewExtensionStore(extensionsDir)
		if err != nil {
			return nil, err
		}
	}
	return &Detector{
		DetectConfig:   config,
		Platform:       platform,
		Resolver:       resolver,
		Runs:           &sync.Map{},
		BuildpackStore: bpStore,
		ExtensionStore: extStore,
	}, nil
}

func (d *Detector) Detect(order buildpack.Order) (buildpack.Group, platform.BuildPlan, error) {
	return d.DetectOrder(order)
}

func (d *Detector) DetectOrder(order buildpack.Order) (buildpack.Group, platform.BuildPlan, error) {
	bps, entries, err := d.detectOrder(order, nil, nil, false, &sync.WaitGroup{})
	if err == ErrBuildpack {
		err = buildpack.NewError(err, buildpack.ErrTypeBuildpack)
	} else if err == ErrFailedDetection {
		err = buildpack.NewError(err, buildpack.ErrTypeFailedDetection)
	}
	for i := range entries {
		for j := range entries[i].Requires {
			entries[i].Requires[j].ConvertVersionToMetadata()
		}
	}
	return buildpack.Group{Group: bps}, platform.BuildPlan{Entries: entries}, err
}

func (d *Detector) detectOrder(order buildpack.Order, done, next []buildpack.GroupBuildable, optional bool, wg *sync.WaitGroup) ([]buildpack.GroupBuildable, []platform.BuildPlanEntry, error) {
	ngroup := buildpack.Group{Group: next}
	buildpackErr := false
	for _, group := range order {
		// FIXME: double-check slice safety here
		found, plan, err := d.detectGroup(group.Append(ngroup), done, wg)
		if err == ErrBuildpack {
			buildpackErr = true
		}
		if err == ErrFailedDetection || err == ErrBuildpack {
			wg = &sync.WaitGroup{}
			continue
		}
		return found, plan, err
	}
	if optional {
		return d.detectGroup(ngroup, done, wg)
	}

	if buildpackErr {
		return nil, nil, ErrBuildpack
	}
	return nil, nil, ErrFailedDetection
}

func (d *Detector) detectGroup(group buildpack.Group, done []buildpack.GroupBuildable, wg *sync.WaitGroup) ([]buildpack.GroupBuildable, []platform.BuildPlanEntry, error) {
	for i, groupBuildable := range group.Group {
		key := groupBuildable.String()
		if hasID(done, groupBuildable.ID) {
			continue
		}

		var (
			buildable buildpack.Buildable
			err       error
		)
		if groupBuildable.Extension {
			if !groupBuildable.Optional {
				// TODO: warn or error?
				groupBuildable.Optional = true
			}
			buildable, err = d.ExtensionStore.Lookup(groupBuildable.ID, groupBuildable.Version)
		} else {
			buildable, err = d.BuildpackStore.Lookup(groupBuildable.ID, groupBuildable.Version)
		}
		if err != nil {
			return nil, nil, err
		}

		bpDesc := buildable.ConfigFile()
		groupBuildable.API = bpDesc.API
		groupBuildable.Homepage = bpDesc.Buildpack.Homepage

		if bpDesc.IsMetaBuildpack() {
			if groupBuildable.Extension {
				return nil, nil, fmt.Errorf(`extension %s contains an "order" definition which is not allowed`, groupBuildable.ID)
			}
			// TODO: double-check slice safety here
			// FIXME: cyclical references lead to infinite recursion
			return d.detectOrder(bpDesc.Order, done, group.Group[i+1:], groupBuildable.Optional, wg)
		}

		bpEnv := env.NewBuildEnv(os.Environ())

		done = append(done, groupBuildable)
		wg.Add(1)
		go func(key string, bp Buildable) {
			if _, ok := d.Runs.Load(key); !ok {
				d.Runs.Store(key, bp.Detect(&d.DetectConfig, bpEnv))
			}
			wg.Done()
		}(key, buildable)
	}

	wg.Wait()

	return d.Resolver.Resolve(done, d.Runs)
}

func hasID(bps []buildpack.GroupBuildable, id string) bool {
	for _, bp := range bps {
		if bp.ID == id {
			return true
		}
	}
	return false
}

type DefaultResolver struct {
	Logger Logger
}

// Resolve aggregates the detect output for a group of buildpacks and tries to resolve a build plan for the group.
// If any required buildpack in the group failed detection or a build plan cannot be resolved, it returns an error.
func (r *DefaultResolver) Resolve(done []buildpack.GroupBuildable, detectRuns *sync.Map) ([]buildpack.GroupBuildable, []platform.BuildPlanEntry, error) {
	var groupRuns []buildpack.DetectRun
	for _, bp := range done {
		t, ok := detectRuns.Load(bp.String())
		if !ok {
			return nil, nil, errors.Errorf("missing detection of '%s'", bp)
		}
		run := t.(buildpack.DetectRun)
		outputLogf := r.Logger.Debugf

		switch run.Code {
		case CodeDetectPass, CodeDetectFail:
		default:
			outputLogf = r.Logger.Infof
		}

		if len(run.Output) > 0 {
			outputLogf("======== Output: %s ========", bp)
			outputLogf(string(run.Output))
		}
		if run.Err != nil {
			outputLogf("======== Error: %s ========", bp)
			outputLogf(run.Err.Error())
		}
		groupRuns = append(groupRuns, run)
	}

	r.Logger.Debugf("======== Results ========")

	results := detectResults{}
	detected := true
	buildpackErr := false
	for i, bp := range done {
		run := groupRuns[i]
		switch run.Code {
		case CodeDetectPass:
			r.Logger.Debugf("pass: %s", bp)
			results = append(results, detectResult{bp, run})
		case CodeDetectFail:
			if bp.Optional {
				r.Logger.Debugf("skip: %s", bp)
			} else {
				r.Logger.Debugf("fail: %s", bp)
			}
			detected = detected && bp.Optional
		case -1:
			r.Logger.Infof("err:  %s", bp)
			buildpackErr = true
			detected = detected && bp.Optional
		default:
			r.Logger.Infof("err:  %s (%d)", bp, run.Code)
			buildpackErr = true
			detected = detected && bp.Optional
		}
	}
	if !detected {
		if buildpackErr {
			return nil, nil, ErrBuildpack
		}
		return nil, nil, ErrFailedDetection
	}

	i := 0
	deps, trial, err := results.runTrials(func(trial detectTrial) (depMap, detectTrial, error) {
		i++
		return r.runTrial(i, trial)
	})
	if err != nil {
		return nil, nil, err
	}

	if len(done) != len(trial) {
		r.Logger.Infof("%d of %d buildpacks participating", len(trial), len(done))
	}

	maxLength := 0
	for _, t := range trial {
		l := len(t.ID)
		if l > maxLength {
			maxLength = l
		}
	}

	f := fmt.Sprintf("%%-%ds %%s", maxLength)

	for _, t := range trial {
		r.Logger.Infof(f, t.ID, t.Version)
	}

	var found []buildpack.GroupBuildable
	for _, r := range trial {
		found = append(found, r.GroupBuildable.NoOpt())
	}
	var plan []platform.BuildPlanEntry
	for _, dep := range deps {
		plan = append(plan, dep.BuildPlanEntry.NoOpt())
	}
	return found, plan, nil
}

func (r *DefaultResolver) runTrial(i int, trial detectTrial) (depMap, detectTrial, error) {
	r.Logger.Debugf("Resolving plan... (try #%d)", i)

	var deps depMap
	retry := true
	for retry {
		retry = false
		deps = newDepMap(trial)

		if err := deps.eachUnmetRequire(func(name string, bp buildpack.GroupBuildable) error {
			retry = true
			if !bp.Optional {
				r.Logger.Debugf("fail: %s requires %s", bp, name)
				return ErrFailedDetection
			}
			r.Logger.Debugf("skip: %s requires %s", bp, name)
			trial = trial.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, err
		}

		if err := deps.eachUnmetProvide(func(name string, bp buildpack.GroupBuildable) error {
			retry = true
			if !bp.Optional {
				r.Logger.Debugf("fail: %s provides unused %s", bp, name)
				return ErrFailedDetection
			}
			r.Logger.Debugf("skip: %s provides unused %s", bp, name)
			trial = trial.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}

	if len(trial) == 0 {
		r.Logger.Debugf("fail: no viable buildpacks in group")
		return nil, nil, ErrFailedDetection
	}
	return deps, trial, nil
}

type detectResult struct {
	buildpack.GroupBuildable
	buildpack.DetectRun
}

func (r *detectResult) options() []detectOption {
	var out []detectOption
	for i, sections := range append([]buildpack.PlanSections{r.PlanSections}, r.Or...) {
		bp := r.GroupBuildable
		bp.Optional = bp.Optional && i == len(r.Or)
		out = append(out, detectOption{bp, sections})
	}
	return out
}

type detectResults []detectResult

type trialFunc func(detectTrial) (depMap, detectTrial, error)

func (rs detectResults) runTrials(f trialFunc) (depMap, detectTrial, error) {
	return rs.runTrialsFrom(nil, f)
}

func (rs detectResults) runTrialsFrom(prefix detectTrial, f trialFunc) (depMap, detectTrial, error) {
	if len(rs) == 0 {
		deps, trial, err := f(prefix)
		return deps, trial, err
	}

	var lastErr error
	for _, option := range rs[0].options() {
		deps, trial, err := rs[1:].runTrialsFrom(append(prefix, option), f)
		if err == nil {
			return deps, trial, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

type detectOption struct {
	buildpack.GroupBuildable
	buildpack.PlanSections
}

type detectTrial []detectOption

func (ts detectTrial) remove(bp buildpack.GroupBuildable) detectTrial {
	var out detectTrial
	for _, t := range ts {
		if t.GroupBuildable != bp {
			out = append(out, t)
		}
	}
	return out
}

type depEntry struct {
	platform.BuildPlanEntry
	earlyRequires []buildpack.GroupBuildable
	extraProvides []buildpack.GroupBuildable
}

type depMap map[string]depEntry

func newDepMap(trial detectTrial) depMap {
	m := depMap{}
	for _, option := range trial {
		for _, p := range option.Provides {
			m.provide(option.GroupBuildable, p)
		}
		for _, r := range option.Requires {
			m.require(option.GroupBuildable, r)
		}
	}
	return m
}

func (m depMap) provide(bp buildpack.GroupBuildable, provide buildpack.Provide) {
	entry := m[provide.Name]
	entry.extraProvides = append(entry.extraProvides, bp)
	m[provide.Name] = entry
}

func (m depMap) require(bp buildpack.GroupBuildable, require buildpack.Require) {
	entry := m[require.Name]
	entry.Providers = append(entry.Providers, entry.extraProvides...)
	entry.extraProvides = nil

	if len(entry.Providers) == 0 {
		entry.earlyRequires = append(entry.earlyRequires, bp)
	} else {
		entry.Requires = append(entry.Requires, require)
	}
	m[require.Name] = entry
}

func (m depMap) eachUnmetProvide(f func(name string, bp buildpack.GroupBuildable) error) error {
	for name, entry := range m {
		if len(entry.extraProvides) != 0 {
			for _, bp := range entry.extraProvides {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m depMap) eachUnmetRequire(f func(name string, bp buildpack.GroupBuildable) error) error {
	for name, entry := range m {
		if len(entry.earlyRequires) != 0 {
			for _, bp := range entry.earlyRequires {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
