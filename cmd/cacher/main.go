package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/imgutil/remote"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image/auth"
)

var (
	cacheImageTag string
	cacheDir      string
	layersDir     string
	groupPath     string
	uid           int
	gid           int
	printVersion  bool
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagCacheImage(&cacheImageTag)
	cmd.FlagCacheDir(&cacheDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUID(&uid)
	cmd.FlagGID(&gid)
	cmd.FlagVersion(&printVersion)
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	flag.Parse()

	if printVersion {
		cmd.ExitWithVersion()
	}

	if flag.NArg() > 0 {
		cmd.Exit(cmd.FailErrCode(errors.New("received unexpected args"), cmd.CodeInvalidArgs, "parse arguments"))
	}
	if cacheImageTag == "" && cacheDir == "" {
		cmd.Exit(cmd.FailErrCode(errors.New("must supply either -image or -path"), cmd.CodeInvalidArgs, "parse arguments"))
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
		origCacheImage, err := remote.NewImage(
			cacheImageTag,
			auth.DefaultEnvKeychain(),
			remote.FromBaseImage(cacheImageTag),
		)
		if err != nil {
			return cmd.FailErr(err, "accessing cache image")
		}

		emptyImage, err := remote.NewImage(
			cacheImageTag,
			auth.DefaultEnvKeychain(),
			remote.WithPreviousImage(cacheImageTag),
		)
		if err != nil {
			return cmd.FailErr(err, "creating new cache image")
		}

		cacheStore = cache.NewImageCache(
			origCacheImage,
			emptyImage,
		)
	} else {
		var err error
		cacheStore, err = cache.NewVolumeCache(cacheDir)
		if err != nil {
			return cmd.FailErr(err, "create volume cache")
		}
	}

	if err := cacher.Cache(layersDir, cacheStore); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed, "cache")
	}

	return nil
}
