package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/extender"
	"github.com/buildpacks/lifecycle/extender/kaniko"
)

type extendCmd struct {
	// flags
	appDir      string
	cacheDir    string
	ignorePaths string
	kind        string
	configPath  string
	workDir     string

	// args
	baseImageRef string

	// derived
	platformAPI string
	keychain    authn.Keychain // construct if necessary before dropping privileges
}

func (e *extendCmd) DefineFlags() {
	cmd.FlagAppDir(&e.appDir)
	cmd.FlagCacheDir(&e.cacheDir)
	cmd.FlagExtendIgnorePaths(&e.ignorePaths)
	cmd.FlagExtendKind(&e.kind)
	cmd.FlagExtendConfigPath(&e.configPath)
	cmd.FlagExtendWorkDir(&e.workDir)
}

func (e *extendCmd) Args(nargs int, args []string) error {
	if nargs != 1 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 1", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}
	e.baseImageRef = args[0]
	return e.validateInputs()
}

func (e *extendCmd) validateInputs() error {
	// TODO: add
	return nil
}

func (e *extendCmd) Privileges() error {
	var err error
	e.keychain, err = auth.DefaultKeychain(e.baseImageRef) // for now, assume all FROM images are on the same registry
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	// TODO: verify registry read access to base image
	return nil
}

func (e *extendCmd) Exec() error {
	type extendConfig struct { // TODO: move somewhere else
		Dockerfiles []extender.Dockerfile `toml:"dockerfiles,omitempty" json:"dockerfiles,omitempty"`
	}
	var cfg extendConfig
	_, err := toml.DecodeFile(e.configPath, &cfg)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}

	return extender.Extend(extender.Opts{
		BaseImageRef: e.baseImageRef,
		Kind:         e.kind,
		Applier:      kaniko.NewDockerfileApplier(e.cacheDir, e.appDir, e.workDir, cmd.DefaultLogger),
		Dockerfiles:  cfg.Dockerfiles,
		IgnorePaths:  strings.Split(e.ignorePaths, ","),
	})
}
