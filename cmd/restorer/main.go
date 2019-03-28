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
)

var (
	cacheImageTag string
	layersDir     string
	groupPath     string
	uid           int
	gid           int
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagCacheImage(&cacheImageTag)
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
	if cacheImageTag == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "-image flag is required"))
	}
	cmd.Exit(restore())
}

func restore() error {
	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	restorer := &lifecycle.Restorer{
		LayersDir:  layersDir,
		Buildpacks: group.Buildpacks,
		Out:        log.New(os.Stdout, "", 0),
		Err:        log.New(os.Stderr, "", 0),
		UID:        uid,
		GID:        gid,
	}

	factory, err := image.NewFactory(image.WithOutWriter(os.Stdout))
	if err != nil {
		return err
	}

	cacheImage, err := factory.NewLocal(cacheImageTag)
	if err != nil {
		return err
	}

	if err := restorer.Restore(cacheImage); err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailed)
	}
	return nil
}
