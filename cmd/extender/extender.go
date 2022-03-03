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
	appDir        string
	cacheDir      string
	cacheImageRef string
	ignorePaths   string
	kind          string
	configPath    string
	workDir       string

	// flags for builder subcommand
	// appDir
	buildpacksDir string
	groupPath     string
	layersDir     string
	planPath      string
	platformDir   string

	// args
	baseImageRef   string
	targetImageRef string

	// derived
	platformAPI string
	keychain    authn.Keychain // construct if necessary before dropping privileges
}

func (e *extendCmd) DefineFlags() {
	cmd.FlagAppDir(&e.appDir)
	cmd.FlagCacheDir(&e.cacheDir) // TODO: will this ever be used by kaniko? Should we remove it?
	cmd.FlagCacheImage(&e.cacheImageRef)
	cmd.FlagExtendIgnorePaths(&e.ignorePaths)
	cmd.FlagExtendKind(&e.kind)
	cmd.FlagExtendConfigPath(&e.configPath)
	cmd.FlagExtendWorkDir(&e.workDir)

	// flags for builder subcommand
	// TODO: it would be nice not to have to repeat this code... perhaps extendCmd could provide itself to buildCmd somehow
	cmd.FlagBuildpacksDir(&e.buildpacksDir)
	cmd.FlagGroupPath(&e.groupPath)
	cmd.FlagLayersDir(&e.layersDir)
	cmd.FlagPlanPath(&e.planPath)
	cmd.FlagPlatformDir(&e.platformDir)
}

func (e *extendCmd) Args(nargs int, args []string) error {
	if nargs != 2 {
		return cmd.FailErrCode(fmt.Errorf("received %d arguments, but expected 2", nargs), cmd.CodeInvalidArgs, "parse arguments")
	}
	e.baseImageRef = args[0]
	e.targetImageRef = args[1]
	return e.validateInputs()
}

func (e *extendCmd) validateInputs() error {
	// TODO: add
	return nil
}

func (e *extendCmd) Privileges() error {
	var err error
	e.keychain, err = auth.DefaultKeychain(e.baseImageRef) // for now, assume all FROM images, and the target image, are on the same registry
	if err != nil {
		return cmd.FailErr(err, "resolve keychain")
	}
	// TODO: verify registry read access to base image, read/write access to cache image
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
		BaseImageRef:   e.baseImageRef,
		TargetImageRef: e.targetImageRef,
		Kind:           e.kind,
		Applier:        kaniko.NewDockerfileApplier(e.cacheDir, e.appDir, e.workDir),
		Dockerfiles:    cfg.Dockerfiles,
		IgnorePaths:    strings.Split(e.ignorePaths, ","),
		BuilderOpts: extender.BuilderOpts{
			AppDir:        e.appDir,
			BuildpacksDir: e.buildpacksDir,
			GroupPath:     e.groupPath,
			LayersDir:     e.layersDir,
			LogLevel:      "debug", // TODO: make this configurable
			PlanPath:      e.planPath,
			PlatformDir:   e.platformDir,
		},
	}, cmd.DefaultLogger)
}
