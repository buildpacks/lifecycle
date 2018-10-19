package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/img"
)

var (
	repoName     string
	launchDir    string
	groupPath    string
	useDaemon    bool
	useHelpers   bool
	metadataPath string
)

func init() {
	cmd.FlagLaunchDir(&launchDir)
	cmd.FlagGroupPath(&groupPath)
	cmd.FlagUseDaemon(&useDaemon)
	cmd.FlagUseCredHelpers(&useHelpers)
	cmd.FlagMetadataPath(&metadataPath)
}

func main() {
	flag.Parse()
	repoName = flag.Arg(0)
	if flag.NArg() > 1 || repoName == "" {
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "parse arguments"))
	}
	cmd.Exit(analyzer())
}

func analyzer() error {
	if useHelpers {
		if err := img.SetupCredHelpers(repoName); err != nil {
			return cmd.FailErr(err, "setup credential helpers")
		}
	}

	var group lifecycle.BuildpackGroup
	if _, err := toml.DecodeFile(groupPath, &group); err != nil {
		return cmd.FailErr(err, "read group")
	}

	analyzer := &lifecycle.Analyzer{
		Buildpacks: group.Buildpacks,
		Out:        os.Stdout,
		Err:        os.Stderr,
	}

	var metadata string
	if metadataPath != "" {
		bMetadata, err := ioutil.ReadFile(metadataPath)
		if err != nil {
			return cmd.FailErr(err, "access image metadata from path", metadataPath)
		}
		metadata = string(bMetadata)
	} else {
		var err error
		newRepoStore := img.NewRegistry
		if useDaemon {
			newRepoStore = img.NewDaemon
		}
		metadata, err = analyzer.GetMetadata(newRepoStore, repoName)
		if err != nil {
			return cmd.FailErr(err, "access image metadata from image", metadataPath)
		}
	}

	if metadata == "" {
		return nil
	}

	config := lifecycle.AppImageMetadata{}
	if err := json.Unmarshal([]byte(metadata), &config); err != nil {
		log.Printf("WARNING: skipping analyze, previous image metadata was incompatible")
		return nil
	}

	err := analyzer.Analyze(
		launchDir,
		config,
	)
	if err != nil {
		return cmd.FailErrCode(err, cmd.CodeFailedBuild)
	}

	return nil
}
