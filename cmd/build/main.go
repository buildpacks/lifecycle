package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/cmd"
)

var (
	printVersion bool
	logLevel     string
	env          *lifecycle.Env
)

func init() {
	env = &lifecycle.Env{
		LookupEnv: os.LookupEnv,
		Getenv:    os.Getenv,
		Setenv:    os.Setenv,
		Unsetenv:  os.Unsetenv,
		Environ:   os.Environ,
		Map:       lifecycle.POSIXBuildEnv,
	}
}

func main() {
	// suppress output from libraries, lifecycle will not use standard logger
	log.SetOutput(ioutil.Discard)

	cmd.FlagVersion(&printVersion)
	cmd.FlagLogLevel(&logLevel)

	phase := filepath.Base(os.Args[0])
	switch phase {
	case "make":
		flags, err := parseMakeFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(doMake(flags))
	case "detector":
		flags := parseDetectFlags()
		cmd.Exit(detector(flags))
	case "analyzer":
		flags, err := parseAnalyzeFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(analyzer(flags))
	case "restorer":
		flags, err := parseRestoreFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(restorer(flags))
	case "builder":
		flags, err := parseBuildFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(builder(flags))
	case "exporter":
		flags, err := parseExportFlags()
		if err != nil {
			cmd.Exit(err)
		}
		cmd.Exit(exporter(flags))
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
