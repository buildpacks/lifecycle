package main

import (
	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

type extendCmd struct {
	extendArgs
}

type extendArgs struct {
	platform Platform
	mode     string
}

func (e *extendCmd) DefineFlags() {
	// no-op
}

func (e *extendCmd) Args(nargs int, args []string) error {
	if len(args) == 0 {
		return nil
	}

	e.extendArgs.mode = args[0]
	return nil
}

func (e *extendCmd) Privileges() error {
	return nil
}

func (e *extendCmd) Exec() error {
	return e.extend()
}

func (e extendArgs) extend() error {
	extender := &lifecycle.Extender{
		Logger: cmd.DefaultLogger,
		Mode:   e.mode,
	}

	return extender.Extend()
}
