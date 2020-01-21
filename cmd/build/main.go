package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/cmd"
)

var (
	printVersion bool
	logLevel     string
)

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	cmd.FlagVersion(&printVersion)
	cmd.FlagLogLevel(&logLevel)

	phase := filepath.Base(os.Args[0])
	switch phase {
	case "detector":
		flags := parseDetectFlags()
		cmd.Exit(detect(flags))
	case "analyzer":
		flags, err := parseAnalyzeFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(analyze(flags))
	case "restorer":
		flags, err := parseRestoreFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(restore(flags))
	case "builder":
		flags, err := parseBuildFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(build(flags))
	case "exporter":
		flags, err := parseExportFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(export(flags))
	case "rebaser":
		flags, err := parseRebaseFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(rebase(flags))
	default:
		cmd.Exit(cmd.FailCode(cmd.CodeInvalidArgs, "unknown phase:", phase))
	}
}
