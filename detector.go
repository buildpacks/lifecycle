package lifecycle

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"encoding/json"
)

const (
	CodeDetectPass = iota
	CodeDetectError
	CodeDetectFail = 100
)

type Buildpack struct {
	ID   string
	Name string
	Dir  string
}

func (bp *Buildpack) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, bp); err == nil {
		return nil
	}
	*bp = Buildpack{}
	return json.Unmarshal(b, &bp.ID)
}

func (bp *Buildpack) Detect(appDir string, in io.Reader, out io.Writer, l *log.Logger) int {
	path, err := filepath.Abs(filepath.Join(bp.Dir, "bin", "detect"))
	if err != nil {
		l.Print("Error: ", err)
		return CodeDetectError
	}
	stderr := &bytes.Buffer{}
	defer func() {
		if stderr.Len() > 0 {
			l.Print(stderr)
		}
	}()
	cmd := exec.Command(path)
	cmd.Dir = appDir
	cmd.Stdin = in
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus()
			}
		}
		l.Print("Error: ", err)
		return CodeDetectError
	}
	return CodeDetectPass
}


type BuildpackGroup []*Buildpack

func (bg BuildpackGroup) Detect(appDir string, l *log.Logger) bool {
	summary := "Group:"
	detected := true
	for i, code := range bg.pDetect(appDir, l) {
		if i > 0 {
			summary += " |"
		}
		switch code {
		case CodeDetectPass:
			summary += fmt.Sprintf(" %s: pass", bg[i].Name)
		case CodeDetectFail:
			summary += fmt.Sprintf(" %s: fail", bg[i].Name)
			detected = false
		default:
			summary += fmt.Sprintf(" %s: error (%d)", bg[i].Name, code)
			detected = false
		}
	}
	l.Println(summary)
	return detected
}

func (bg BuildpackGroup) pDetect(appDir string, l *log.Logger) []int {
	codes := make([]int, len(bg))
	wg := sync.WaitGroup{}
	defer wg.Wait()
	wg.Add(len(bg))
	var lastIn io.ReadCloser
	defer func() {
		if lastIn != nil {
			lastIn.Close()
		}
	}()
	for i := range bg {
		in, out := io.Pipe()
		go func(i int, r io.ReadCloser) {
			defer wg.Done()
			defer out.Close()
			if r != nil {
				defer r.Close()
			}
			w := &bytes.Buffer{}
			codes[i] = bg[i].Detect(appDir, r, w, l)
			io.Copy(out, w)
		}(i, lastIn)
		lastIn = in
	}
	return codes
}

type BuildpackOrder []BuildpackGroup

func (bo BuildpackOrder) Detect(appDir string, l *log.Logger) BuildpackGroup {
	for _, group := range bo {
		if group.Detect(appDir, l) {
			return group
		}
	}
	return nil
}