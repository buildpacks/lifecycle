// +build windows

package snapshot

import (
	"errors"

	"github.com/buildpacks/lifecycle"
)

type WindowsSnapshotter struct {
}

var notImplemented = errors.New("Windows Kaniko Snapshotter is not implemented")

func NewKanikoSnapshotter(rootDir string) (lifecycle.LayerSnapshotter, error) {
	return &WindowsSnapshotter{}, nil
}

func (ws *WindowsSnapshotter) GetRootDir() string {
	return ""
}
func (ws *WindowsSnapshotter) TakeSnapshot(string) error {
	return notImplemented
}
func (ws *WindowsSnapshotter) Init() error {
	return notImplemented
}
