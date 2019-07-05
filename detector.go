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

type Trial struct {
	Requires []Require `toml:"requires"`
	Provides []Provide `toml:"provides"`
	Code     int       `toml:"-"`
	Err      error     `toml:"-"`
}

type Run struct {
	Buildpack
	Trial
}

type Require struct {
	Name     string                 `toml:"name"`
	Version  string                 `toml:"version"`
	Metadata map[string]interface{} `toml:"metadata"`
}

type Provide struct {
	Name string `toml:"name"`
}

type PlanEntry struct {
	BuildpackID string `toml:"buildpack-id"`
	Require
}

type DetectConfig struct {
	AppDir      string
	PlatformDir string
	PathByID    string
	Trials      *sync.Map
	Out, Err    *log.Logger
}

func (c *DetectConfig) lookup(bp Buildpack) (*buildpackInfo, error) {
	bpTOML := buildpackTOML{}
	bpPath := filepath.Join(c.PathByID, bp.dir(), bp.Version)
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

func (bp *buildpackInfo) Detect(c *DetectConfig) Trial {
	detectPath, err := filepath.Abs(filepath.Join(bp.Path, "bin", "detect"))
	if err != nil {
		return Trial{Code: -1, Err: err}
	}
	appDir, err := filepath.Abs(c.AppDir)
	if err != nil {
		return Trial{Code: -1, Err: err}
	}
	platformDir, err := filepath.Abs(c.PlatformDir)
	if err != nil {
		return Trial{Code: -1, Err: err}
	}
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return Trial{Code: -1, Err: err}
	}
	defer os.RemoveAll(planDir)
	planPath := filepath.Join(planDir, "plan.toml")
	if err := ioutil.WriteFile(planPath, nil, 0777); err != nil {
		return Trial{Code: -1, Err: err}
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
				return Trial{Code: status.ExitStatus()}
			}
		}
		return Trial{Code: -1, Err: err}
	}
	var t Trial
	if _, err := toml.DecodeFile(planPath, &t); err != nil {
		return Trial{Code: -1, Err: err}
	}
	return t
}

type BuildpackGroup struct {
	Group []Buildpack `toml:"group"`
}

func hasID(bps []Buildpack, id string) bool {
	for _, bp := range bps {
		if bp.ID == id {
			return true
		}
	}
	return false
}

func (bg *BuildpackGroup) Detect(done []Buildpack, wg *sync.WaitGroup, c *DetectConfig) ([]Buildpack, error) {
	return bg.detect(nil, &sync.WaitGroup{}, c)
}

func (bg *BuildpackGroup) detect(done []Buildpack, wg *sync.WaitGroup, c *DetectConfig) ([]Buildpack, error) {
	for i, bp := range bg.Group {
		if hasID(done, bp.ID) {
			continue
		}

		info, err := c.lookup(bp)
		if err != nil {
			return nil, err
		}
		if info.Order != nil {
			// FIXME: double-check slice safety here
			return info.Order.detect(done, bg.Group[i+1:], bp.Optional, wg, c)
		}
		done = append(done, bp)
		wg.Add(1)
		go func() {
			if _, ok := c.Trials.Load(bp.String()); !ok {
				c.Trials.Store(bp.String(), info.Detect(c))
			}
			wg.Done()
		}()
	}

	wg.Wait()

	var out []Buildpack
	var trials []Trial
	detected := true
	c.Out.Printf("======== Results ========")

	for _, bp := range done {
		t, ok := c.Trials.Load(bp.String())
		if !ok {
			return nil, errors.Errorf("missing detection of '%s'", bp)
		}
		trial := t.(Trial)

		switch trial.Code {
		case CodeDetectPass:
			c.Out.Printf("pass: %s", bp)
			out = append(out, bp)
			trials = append(trials, trial)
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
	if !detected || len(out) == 0 {
		return nil, ErrFail
	}

	provides := provideMap{}
	for i, bp := range out {
		for _, p := range trials[i].Provides {
			provides[p.Name] = append(provides[p.Name], PlanEntry{BuildpackID: bp.ID})
		}
		for _, r := range trials[i].Requires {
			if _, ok := provides[r.Name]; !ok {
				// fail
			}
			provides[r.Name]


			if _, ok := provides[r.Name]; !ok {
				requires[r.Name] = append(requires[r.Name], bp.ID)
			}
			delete(provides, r.Name)
		}
	}

	//provides := constraintMap{}
	//requires := constraintMap{}
	//for i, bp := range out {
	//	for _, p := range trials[i].Provides {
	//		provides[p.Name] = append(provides[p.Name], bp.ID)
	//	}
	//	for _, r := range trials[i].Requires {
	//		if _, ok := provides[r.Name]; !ok {
	//			requires[r.Name] = append(requires[r.Name], bp.ID)
	//		}
	//		delete(provides, r.Name)
	//	}
	//}
	//var newOut []Buildpack
	//for _, bp := range out {
	//	if provides.contains(bp.ID) {
	//		if !bp.Optional {
	//			c.Out.Printf("fail: %s provides unrequired %s", bp, )
	//			return nil, ErrFail
	//		}
	//		continue
	//	}
	//	if requires.contains(bp.ID) {
	//		continue
	//	}
	//	newOut = append(newOut, bp)
	//}

	// validate requires / provides

	return out, nil
}

//type constraintMap map[string][]string
//
//func (c constraintMap) contains(elem string) bool {
//	for _, v := range c {
//		for _, e := range v {
//			if elem == e {
//				return true
//			}
//		}
//	}
//	return false
//}

// play

type provideMap map[string][]PlanEntry

/////

func (bg BuildpackGroup) append(group ...BuildpackGroup) BuildpackGroup {
	for _, g := range group {
		bg.Group = append(bg.Group, g.Group...)
	}
	return bg
}

type BuildpackOrder []BuildpackGroup

func (bo BuildpackOrder) Detect(c *DetectConfig) ([]Buildpack, error) {
	return bo.detect(nil, nil, false, &sync.WaitGroup{}, c)
}

func (bo BuildpackOrder) detect(done, next []Buildpack, optional bool, wg *sync.WaitGroup, c *DetectConfig) ([]Buildpack, error) {
	ngroup := BuildpackGroup{Group: next}
	for _, group := range bo {
		// FIXME: double-check slice safety here
		result, err := group.append(ngroup).detect(done, wg, c)
		if err == ErrFail {
			wg = &sync.WaitGroup{}
			continue
		}
		return result, err
	}
	if optional {
		return ngroup.detect(done, wg, c)
	}
	return nil, ErrFail
}
