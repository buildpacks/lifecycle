package main

import (
	"flag"
	"fmt"
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
)

func init() {
	cmd.FlagLayersDir(&layersDir)
	cmd.FlagCacheImage(&cacheImageTag)
	cmd.FlagGroupPath(&groupPath)
}

func main() {
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
		Out:        log.New(os.Stdout, "", log.LstdFlags),
		Err:        log.New(os.Stderr, "", log.LstdFlags),
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
