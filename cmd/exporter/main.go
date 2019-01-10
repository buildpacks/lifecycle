package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
	"github.com/google/go-containerregistry/pkg/name"
)

var (
	repoName      string
	cacheTag      string
	runImageRef   string
	cacheImageRef string
	layersDir     string
	appDir        string
	groupPath     string
	useDaemon     bool
	useHelpers    bool
	uid           int
	gid           int
)

const launcherPath = "/lifecycle/launcher"

func init() {
	cmd.FlagRunImage(&runImageRef)
	cmd.FlagCacheImage(&cacheImageRef)
	cmd.FlagCacheTag(&cacheTag)
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagAppDir(&appDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	flag.Parse()
	if flag.NArg() > 1 || flag.Arg(0) == "" || runImageRef == "" {
		args := map[string]interface{}{"narg": flag.NArg(), "runImage": runImageRef, "launchDir": layersDir}
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments", fmt.Sprintf("%+v", args)))
	}
	repoName = flag.Arg(0)
	cmd.Exit(export())
}

func export() error {
	var group lifecycle.BuildpackGroup
	var err error
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}
	if useHelpers {
		if err := lifecycle.SetupCredHelpers(repoName, runImageRef); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	tagRef, err := name.NewTag(repoName, name.WeakValidation)
	if err != nil {
		return err
	}

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)
	exporter := &lifecycle.Exporter{
		Buildpacks:   group.Buildpacks,
		Out:          log.New(os.Stdout, "", log.LstdFlags),
		Err:          log.New(os.Stderr, "", log.LstdFlags),
		UID:          uid,
		GID:          gid,
		ArtifactsDir: artifactsDir,
	}

	factory, err := image.DefaultFactory()
	if err != nil {
		return err
	}
	var runImage, origImage image.Image
	var cacher lifecycle.Cacher
	if useDaemon {
		runImage, err = factory.NewLocal(runImageRef, false)
		if err != nil {
			return err
		}
		origImage, err = factory.NewLocal(tagRef.Name(), false)
		if err != nil {
			return err
		}

		if len(cacheImageRef) > 0 {
			cacheTagRef, _ := name.NewTag(fmt.Sprintf("%s:%s", tagRef.Repository.Name(), cacheTag), name.WeakValidation)
			if err != nil {
				return err
			}
			cacher, err = lifecycle.NewLocalImageCacher(cacheImageRef, cacheTagRef.Name(), *factory, exporter.Out)
			if err != nil {
				return err
			}
		}
	} else {
		runImage, err = factory.NewRemote(runImageRef)
		if err != nil {
			return err
		}
		origImage, err = factory.NewRemote(tagRef.Name())
		if err != nil {
			return err
		}
	}

	if cacher == nil {
		cacher = &lifecycle.NoopCacher{}
	}

	if err := exporter.Export(layersDir, appDir, runImage, origImage, launcherPath, cacher); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	return nil
}
