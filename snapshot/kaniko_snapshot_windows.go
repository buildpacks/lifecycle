// +build windows

package snapshot

import (
	"errors"

	"github.com/buildpacks/lifecycle"
)

type WindowsSnapshotter struct {
}

var errNotImplemented = errors.New("windows kaniko snapshotter is not implemented")

func NewKanikoSnapshotter(rootDir string) (lifecycle.LayerSnapshotter, error) {
	return &WindowsSnapshotter{}, nil
}

func (ws *WindowsSnapshotter) TakeSnapshot(string) error {
	return errNotImplemented
}
func (ws *WindowsSnapshotter) Init() error {
	return errNotImplemented
}
