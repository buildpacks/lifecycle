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

func (bp Buildpack) Detect(appDir string, in io.Reader, out io.Writer, l *log.Logger) int {
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

type BuildpackGroup []Buildpack

func (bg BuildpackGroup) Detect(appDir string, l *log.Logger) bool {
	buffers := make([]bytes.Buffer, len(bg))
	codes := make([]int, len(bg))
	wg := sync.WaitGroup{}
	wg.Add(len(bg))
	for i := range bg {
		go func(i int) {
			var last io.Reader
			if i > 0 {
				last = &buffers[i-1]
			}
			codes[i] = bg[i].Detect(appDir, last, &buffers[i], l)
			wg.Done()
		}(i)
	}
	wg.Wait()
	summary := "Group:"
	detected := true
	for i, code := range codes {
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

type BuildpackList []BuildpackGroup

func (bl BuildpackList) Detect(appDir string, l *log.Logger) BuildpackGroup {
	for _, group := range bl {
		if group.Detect(appDir, l) {
			return group
		}
	}
	return nil
}
