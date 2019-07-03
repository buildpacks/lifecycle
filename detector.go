package lifecycle

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
	"github.com/BurntSushi/toml"
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

type Trial struct {
	Requires []Require `toml:"requires"`
	Provides []Provide `toml:"provides"`
	Code     int       `toml:"-"`
	Err      error     `toml:"-"`
}

type Require struct {
	Name     string                 `toml:"name"`
	Version  string                 `toml:"version"`
	Metadata map[string]interface{} `toml:"metadata"`
}

type PlanEntry struct {
	BuildpackID string `toml:"buildpack-id"`
	Require
}

type Provide struct {
	Name string `toml:"name"`
}

type DetectConfig struct {
	AppDir      string
	PlatformDir string
	PathByID    string
	Trials      map[string]Trial
	Out, Err    *log.Logger
}

func (bp *Buildpack) dir() string {
	return escapeID(bp.ID)
}

func (bp *Buildpack) String() string {
	return bp.ID + "@" + bp.Version
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

func (bt *buildpackTOML) lookup(bp Buildpack) (*buildpackInfo, error) {
	for _, b := range bt.Buildpacks {
		if b.ID == bp.ID && b.Version == bp.Version {

			if b.Order != nil && b.Path != "" {
				return nil, errors.Errorf("invalid buildpack '%s@%s'", b.ID, b.Version)
			}

			if b.Order == nil && b.Path == "" {
				b.Path = "."
			}

			// TODO: validate that stack matches $BP_STACK_ID
			// TODO: validate that orders don't have stacks

			return &b, nil
		}
	}
	return nil, errors.Errorf("could not find buildpack '%s'", bp)
}

func (bg *BuildpackGroup) Detect(idx int, c *DetectConfig) ([]Buildpack, error) {
	for i, bp := range bg.Group[idx:] {
		info, err := c.lookup(bp)
		if err != nil {
			return nil, err
		}
		if info.Order != nil {
			// FIXME: double-check slice safety here
			return info.Order.Detect(bg.Group[:idx+i], bg.Group[idx+i+1:], bp.Optional, c)
		}

		if _, ok := c.Trials[bp.String()]; !ok {
			// do detection
			c.Trials[bp.String()] = info.Detect(c)
		}
	}

	// check detection

	return bg.Group, nil

	//group = &BuildpackGroup{}
	//detected := true
	//plan, codes := bg.pDetect(c)
	//c.Out.Printf("======== Results ========")
	//for i, code := range codes {
	//	name := bg.Group[i].Name
	//	optional := bg.Group[i].Optional
	//	switch code {
	//	case CodeDetectPass:
	//		c.Out.Printf("pass: %s", name)
	//		group.Group = append(group.Group, bg.Group[i])
	//	case CodeDetectFail:
	//		if optional {
	//			c.Out.Printf("skip: %s", name)
	//		} else {
	//			c.Out.Printf("fail: %s", name)
	//		}
	//		detected = detected && optional
	//	default:
	//		c.Out.Printf("err:  %s: (%d)", name, code)
	//		detected = detected && optional
	//	}
	//}
	//detected = detected && len(group.Group) > 0
	//return plan, group, detected
}

func (bg BuildpackGroup) append(group ...BuildpackGroup) BuildpackGroup {
	for _, g := range group {
		bg.Group = append(bg.Group, g)
	}
	return bg
}

func (bg BuildpackGroup) len() int {
	return len(bg.Group)
}

//func (bg *BuildpackGroup) pDetect(c *DetectConfig) (plan []byte, codes []int) {
//	codes = make([]int, len(bg.Group))
//	wg := sync.WaitGroup{}
//	defer wg.Wait()
//	wg.Add(len(bg.Group))
//	var lastIn io.ReadCloser
//	for i := range bg.Group {
//		in, out := io.Pipe()
//		go func(i int, last io.ReadCloser) {
//			defer wg.Done()
//			defer out.Close()
//			add := &bytes.Buffer{}
//			if last != nil {
//				defer last.Close()
//				orig := &bytes.Buffer{}
//				last := io.TeeReader(last, orig)
//				codes[i] = bg.Group[i].Detect(c, last, add)
//				io.Copy(ioutil.Discard, last)
//				if codes[i] == CodeDetectPass {
//					mergeTOML(c.Err, out, orig, add)
//				} else {
//					mergeTOML(c.Err, out, orig)
//				}
//			} else {
//				codes[i] = bg.Group[i].Detect(c, nil, add)
//				if codes[i] == CodeDetectPass {
//					mergeTOML(c.Err, out, add)
//				}
//			}
//		}(i, lastIn)
//		lastIn = in
//	}
//	if lastIn != nil {
//		defer lastIn.Close()
//		if p, err := ioutil.ReadAll(lastIn); err != nil {
//			c.Err.Print("Warning: ", err)
//		} else {
//			plan = p
//		}
//	}
//	return plan, codes
//}

//func mergeTOML(l *log.Logger, out io.Writer, in ...io.Reader) {
//	result := map[string]interface{}{}
//	for _, r := range in {
//		var m map[string]interface{}
//		if _, err := toml.DecodeReader(r, &m); err != nil {
//			l.Print("Warning: ", err)
//			continue
//		}
//		for k, v := range m {
//			result[k] = v
//		}
//	}
//	if err := toml.NewEncoder(out).Encode(result); err != nil {
//		l.Print("Warning: ", err)
//	}
//}

type BuildpackOrder []BuildpackGroup

func (bo BuildpackOrder) Detect(prev, next []Buildpack, optional bool, c *DetectConfig) ([]Buildpack, error) {
	pgroup := BuildpackGroup{Group: prev}
	ngroup := BuildpackGroup{Group: next}
	for _, group := range bo {
		//c.Out.Printf("Trying group %d out of %d with %d buildpacks...", i+1, len(bo), len(bo[i].Group))
		// FIXME: double-check slice safety here
		result, err := pgroup.append(group, ngroup).Detect(pgroup.len(), c)
		if err == ErrFail {
			continue
		}
		return result, err
	}
	if optional {
		return pgroup.append(ngroup).Detect(pgroup.len(), c)
	}
	return nil, ErrFail
}
