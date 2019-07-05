package lifecycle

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

const (
	CodeDetectPass = 0
	CodeDetectFail = 100
)

var ErrFail = errors.New("detection failed")

type Buildpack struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Optional bool   `toml:"optional,omitempty"`
}

func (bp Buildpack) dir() string {
	return escapeID(bp.ID)
}

func (bp Buildpack) String() string {
	return bp.ID + "@" + bp.Version
}

type DetectPlan struct {
	Entries []DetectPlanEntry ``
}

type DetectPlanEntry struct {
	Providers []Buildpack `toml:"providers"`
	Requires  []Require   `toml:"requires"`
}

type Require struct {
	Name     string                 `toml:"name"`
	Version  string                 `toml:"version"`
	Metadata map[string]interface{} `toml:"metadata"`
}

type Provide struct {
	Name string `toml:"name"`
}

type DetectConfig struct {
	AppDir        string
	PlatformDir   string
	BuildpacksDir string
	Out, Err      *log.Logger
	trials        *sync.Map
}

func (bp Buildpack) lookup(buildpacksDir string) (*buildpackInfo, error) {
	bpTOML := buildpackTOML{}
	bpPath := filepath.Join(buildpacksDir, bp.dir(), bp.Version)
	if _, err := toml.DecodeFile(filepath.Join(bpPath, "buildpack.toml"), &bpTOML); err != nil {
		return nil, err
	}
	info, err := bpTOML.lookup(bp)
	if err != nil {
		return nil, err
	}
	info.Path = filepath.Join(bpPath, info.Path)
	return info, nil
}

func (c *DetectConfig) process(done []Buildpack) ([]Buildpack, []DetectPlanEntry, error) {
	var results detectResults
	detected := true
	c.Out.Printf("======== Results ========")

	for _, bp := range done {
		t, ok := c.trials.Load(bp.String())
		if !ok {
			return nil, nil, errors.Errorf("missing detection of '%s'", bp)
		}
		trial := t.(detectTrial)

		switch trial.Code {
		case CodeDetectPass:
			c.Out.Printf("pass: %s", bp)
			results = append(results, detectResult{bp, trial})
		case CodeDetectFail:
			if bp.Optional {
				c.Out.Printf("skip: %s", bp)
			} else {
				c.Out.Printf("fail: %s", bp)
			}
			detected = detected && bp.Optional
		default:
			c.Out.Printf("err:  %s: (%d)", bp, trial.Code)
			detected = detected && bp.Optional
		}
	}
	if !detected {
		return nil, nil, ErrFail
	}

	var deps depMap
	for retry := true; retry; {
		retry = false
		deps = newDepMap(results)

		if err := deps.eachUnmetRequire(func(name string, bp Buildpack) error {
			retry = true
			if !bp.Optional {
				c.Out.Printf("fail: %s requires %s", bp, name)
				return ErrFail
			}
			c.Out.Printf("skip: %s requires %s", bp, name)
			results = results.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, err
		}

		if err := deps.eachUnmetProvide(func(name string, bp Buildpack) error {
			retry = true
			if !bp.Optional {
				c.Out.Printf("fail: %s provides unused %s", bp, name)
				return ErrFail
			}
			c.Out.Printf("skip: %s provides unused %s", bp, name)
			results = results.remove(bp)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}

	if len(results) == 0 {
		c.Out.Print("fail: no buildpacks detected")
		return nil, nil, ErrFail
	}

	var found []Buildpack
	for _, r := range results {
		found = append(found, r.Buildpack)
	}
	var plan []DetectPlanEntry
	for _, dep := range deps {
		plan = append(plan, dep.DetectPlanEntry)
	}
	return found, plan, nil
}

func (bp *buildpackInfo) Detect(c *DetectConfig) detectTrial {
	detectPath, err := filepath.Abs(filepath.Join(bp.Path, "bin", "detect"))
	if err != nil {
		return detectTrial{Code: -1, Err: err}
	}
	appDir, err := filepath.Abs(c.AppDir)
	if err != nil {
		return detectTrial{Code: -1, Err: err}
	}
	platformDir, err := filepath.Abs(c.PlatformDir)
	if err != nil {
		return detectTrial{Code: -1, Err: err}
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return detectTrial{Code: -1, Err: err}
	}
	defer os.RemoveAll(planDir)
	planPath := filepath.Join(planDir, "plan.toml")
	if err := ioutil.WriteFile(planPath, nil, 0777); err != nil {
		return detectTrial{Code: -1, Err: err}
	}
	log := &bytes.Buffer{}
	defer func() {
		if log.Len() > 0 {
			c.Out.Printf("======== Output: %s ========\n%s", bp.Name, log)
		}
	}()
	cmd := exec.Command(detectPath, platformDir, planPath)
	cmd.Dir = appDir
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return detectTrial{Code: status.ExitStatus()}
			}
		}
		return detectTrial{Code: -1, Err: err}
	}
	var t detectTrial
	if _, err := toml.DecodeFile(planPath, &t); err != nil {
		return detectTrial{Code: -1, Err: err}
	}
	return t
}

type BuildpackGroup struct {
	Group []Buildpack `toml:"group"`
}

func (bg BuildpackGroup) Detect(c *DetectConfig) (BuildpackGroup, DetectPlan, error) {
	if c.trials == nil {
		c.trials = &sync.Map{}
	}
	bps, entries, err := bg.detect(nil, &sync.WaitGroup{}, c)
	return BuildpackGroup{Group: bps}, DetectPlan{Entries: entries}, err
}

func (bg BuildpackGroup) detect(done []Buildpack, wg *sync.WaitGroup, c *DetectConfig) ([]Buildpack, []DetectPlanEntry, error) {
	for i, bp := range bg.Group {
		if hasID(done, bp.ID) {
			continue
		}
		info, err := bp.lookup(c.BuildpacksDir)
		if err != nil {
			return nil, nil, err
		}
		if info.Order != nil {
			// FIXME: double-check slice safety here
			return info.Order.detect(done, bg.Group[i+1:], bp.Optional, wg, c)
		}
		done = append(done, bp)
		wg.Add(1)
		go func() {
			if _, ok := c.trials.Load(bp.String()); !ok {
				c.trials.Store(bp.String(), info.Detect(c))
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

func hasID(bps []Buildpack, id string) bool {
	for _, bp := range bps {
		if bp.ID == id {
			return true
		}
	}
	return false
}

type detectTrial struct {
	Requires []Require `toml:"requires"`
	Provides []Provide `toml:"provides"`
	Code     int       `toml:"-"`
	Err      error     `toml:"-"`
}

type detectResult struct {
	Buildpack
	detectTrial
}

type detectResults []detectResult

func (rs detectResults) remove(bp Buildpack) detectResults {
	var out detectResults
	for _, r := range rs {
		if r.Buildpack != bp {
			out = append(out, r)
		}
	}
	return out
}

type depEntry struct {
	DetectPlanEntry
	earlyReqs []Buildpack
}

type depMap map[string]depEntry

func newDepMap(results []detectResult) depMap {
	m := depMap{}
	for _, result := range results {
		for _, p := range result.Provides {
			m.provide(result.Buildpack, p)
		}
		for _, r := range result.Requires {
			m.require(result.Buildpack, r)
		}
	}
	return m
}

func (m depMap) provide(bp Buildpack, provide Provide) {
	entry := m[provide.Name]
	entry.Providers = append(entry.Providers, bp)
	m[provide.Name] = entry
}

func (m depMap) require(bp Buildpack, require Require) {
	entry := m[require.Name]
	if len(entry.Providers) == 0 {
		entry.earlyReqs = append(entry.earlyReqs, bp)
	} else {
		entry.Requires = append(entry.Requires, require)
	}
	m[require.Name] = entry
}

func (m depMap) eachUnmetProvide(f func(name string, bp Buildpack) error) error {
	for name, entry := range m {
		if len(entry.Requires) == 0 {
			for _, bp := range entry.Providers {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m depMap) eachUnmetRequire(f func(name string, bp Buildpack) error) error {
	for name, entry := range m {
		if len(entry.earlyReqs) != 0 {
			for _, bp := range entry.earlyReqs {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type BuildpackOrder []BuildpackGroup

func (bo BuildpackOrder) Detect(c *DetectConfig) (BuildpackGroup, DetectPlan, error) {
	if c.trials == nil {
		c.trials = &sync.Map{}
	}
	bps, entries, err := bo.detect(nil, nil, false, &sync.WaitGroup{}, c)
	return BuildpackGroup{Group: bps}, DetectPlan{Entries: entries}, err
}

func (bo BuildpackOrder) detect(done, next []Buildpack, optional bool, wg *sync.WaitGroup, c *DetectConfig) ([]Buildpack, []DetectPlanEntry, error) {
	ngroup := BuildpackGroup{Group: next}
	for _, group := range bo {
		// FIXME: double-check slice safety here
		found, plan, err := group.append(ngroup).detect(done, wg, c)
		if err == ErrFail {
			wg = &sync.WaitGroup{}
			continue
		}
		return found, plan, err
	}
	if optional {
		return ngroup.detect(done, wg, c)
	}
	return nil, nil, ErrFail
}
