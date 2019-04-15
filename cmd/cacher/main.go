package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
)

var (
	cacheImageTag string
	cachePath     string
	layersDir     string
	groupPath     string
	uid           int
	gid           int
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagCacheImage(&cacheImageTag)
	cmd.FlagCachePath(&cachePath)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()
	if flag.NArg() > 0 {
		args := map[string]interface{}{"narg": flag.NArg(), "layersDir": layersDir}
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments", fmt.Sprintf("%+v", args)))
	}
	if cacheImageTag == "" && cachePath == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "must supply either -image or -path"))
	}
	cmd.Exit(doCache())
}

func doCache() error {
	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	artifactsDir, err := ioutil.TempDir("", "lifecycle.exporter.layer")
	if err != nil {
		return cmd.FailErr(err, "create temp directory")
	}
	defer os.RemoveAll(artifactsDir)

	cacher := &lifecycle.Cacher{
		Buildpacks:   group.Buildpacks,
		ArtifactsDir: artifactsDir,
		Out:          log.New(os.Stdout, "", 0),
		Err:          log.New(os.Stderr, "", 0),
		UID:          uid,
		GID:          gid,
	}

	var cacheStore lifecycle.Cache
	if cacheImageTag != "" {
		factory, err := image.NewFactory(image.WithOutWriter(os.Stdout))
		if err != nil {
			return err
		}

		origCacheImage, err := factory.NewLocal(cacheImageTag)
		if err != nil {
			return err
		}

		cacheStore = cache.NewImageCache(factory, origCacheImage)
	} else {
		var err error
		cacheStore, err = cache.NewVolumeCache(cachePath)
		if err != nil {
			return err
		}
	}

	if err := cacher.Cache(layersDir, cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed)
	}

	return nil
}
