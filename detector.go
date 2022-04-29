package lifecycle

import (
	"fmt"
	"os"
	"sync"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/internal/lifecycle"
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
	Resolve(done []buildpack.GroupElement, detectRuns *sync.Map) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error)
}

type Detector struct {
	buildpack.DetectConfig

	DirStore     DirStore
	OrderHandler lifecycle.OrderHandler
	Resolver     Resolver
	Runs         *sync.Map
}

// TODO: test
func NewDetector(platformAPI *api.Version, config buildpack.DetectConfig, dirStore DirStore) *Detector {
	var orderHandler lifecycle.OrderHandler
	if platformAPI.AtLeast("0.9") { // TODO: change
		orderHandler = &lifecycle.DefaultOrderHandler{}
	} else {
		orderHandler = &lifecycle.LegacyOrderHandler{}
	}
	resolver := &DefaultResolver{
		Logger: config.Logger,
	}
	return &Detector{
		DetectConfig: config,
		DirStore:     dirStore,
		OrderHandler: orderHandler,
		Resolver:     resolver,
		Runs:         &sync.Map{},
	}
}

func (d *Detector) Detect(orderBp, orderExt buildpack.Order) (buildpack.Group, platform.BuildPlan, error) {
	d.OrderHandler.PrependExtensions(orderBp, orderExt)
	return d.DetectOrder(orderBp)
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

func (d *Detector) detectOrder(order buildpack.Order, done, next []buildpack.GroupElement, optional bool, wg *sync.WaitGroup) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
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

func (d *Detector) detectGroup(group buildpack.Group, done []buildpack.GroupElement, wg *sync.WaitGroup) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
	for i, groupEl := range group.Group {
		// Continue if element has already been processed.
		if hasID(done, groupEl.ID) { // TODO: assign id to order-ext?
			continue
		}

		// Resolve order if element is the order for extensions.
		if len(groupEl.OrderExt) > 0 { // TODO: make helper
			return d.detectOrder(groupEl.OrderExt, done, group.Group[i+1:], groupEl.Optional, wg) // TODO: should the entire order for extensions be optional? Should it be configurable?
		}

		// Lookup element in store.
		var (
			detectable platform.Buildpack // TODO: move interface to buildpack package
			err        error
		)
		switch {
		case groupEl.Extension:
			detectable, err = d.DirStore.LookupExt(groupEl.ID, groupEl.Version)
		default:
			detectable, err = d.DirStore.LookupBp(groupEl.ID, groupEl.Version)
		}
		if err != nil {
			return nil, nil, err
		}
		descriptor := detectable.ConfigFile()

		// Resolve order if element is a composite buildpack.
		if descriptor.IsComposite() {
			// TODO: double-check slice safety here
			// FIXME: cyclical references lead to infinite recursion
			return d.detectOrder(descriptor.Order, done, group.Group[i+1:], groupEl.Optional, wg)
		}

		// Mark element as done.
		done = append(done, descriptor.ToGroupElement())

		// Run detect if element is a component buildpack or an extension.
		key := groupEl.String()
		wg.Add(1)
		go func(key string, bp platform.Buildpack) {
			if _, ok := d.Runs.Load(key); !ok {
				d.Runs.Store(key, bp.Detect(&d.DetectConfig, env.NewBuildEnv(os.Environ())))
			}
			wg.Done()
		}(key, detectable)
	}

	wg.Wait()

	return d.Resolver.Resolve(done, d.Runs)
}

func hasID(bps []buildpack.GroupElement, id string) bool {
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
func (r *DefaultResolver) Resolve(done []buildpack.GroupElement, detectRuns *sync.Map) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
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

	var found []buildpack.GroupElement
	for _, r := range trial {
		found = append(found, r.GroupElement.NoOpt())
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

		if err := deps.eachUnmetRequire(func(name string, bp buildpack.GroupElement) error {
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

		if err := deps.eachUnmetProvide(func(name string, bp buildpack.GroupElement) error {
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
	buildpack.GroupElement
	buildpack.DetectRun
}

func (r *detectResult) options() []detectOption {
	var out []detectOption
	for i, sections := range append([]buildpack.PlanSections{r.PlanSections}, r.Or...) {
		bp := r.GroupElement
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
	buildpack.GroupElement
	buildpack.PlanSections
}

type detectTrial []detectOption

func (ts detectTrial) remove(bp buildpack.GroupElement) detectTrial {
	var out detectTrial
	for _, t := range ts {
		if t.GroupElement.ID != bp.ID { // TODO: check
			out = append(out, t)
		}
	}
	return out
}

type depEntry struct {
	platform.BuildPlanEntry
	earlyRequires []buildpack.GroupElement
	extraProvides []buildpack.GroupElement
}

type depMap map[string]depEntry

func newDepMap(trial detectTrial) depMap {
	m := depMap{}
	for _, option := range trial {
		for _, p := range option.Provides {
			m.provide(option.GroupElement, p)
		}
		for _, r := range option.Requires {
			m.require(option.GroupElement, r)
		}
	}
	return m
}

func (m depMap) provide(bp buildpack.GroupElement, provide buildpack.Provide) {
	entry := m[provide.Name]
	entry.extraProvides = append(entry.extraProvides, bp)
	m[provide.Name] = entry
}

func (m depMap) require(bp buildpack.GroupElement, require buildpack.Require) {
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

func (m depMap) eachUnmetProvide(f func(name string, bp buildpack.GroupElement) error) error {
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

func (m depMap) eachUnmetRequire(f func(name string, bp buildpack.GroupElement) error) error {
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
