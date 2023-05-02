package cli

import (
	"io"
	"log"
	"os"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/env"
)

// Command defines the interface for running the lifecycle phases
type Command interface {
	// DefineFlags defines the flags that are considered valid and reads their values (if provided)
	DefineFlags()

	// Args validates arguments and flags, and fills in default values
	Args(nargs int, args []string) error

	// Privileges validates the needed privileges
	Privileges() error

	// Exec executes the command
	Exec() error
}

func Run(c Command, asSubcommand bool) {
	var (
		printVersion bool
		logLevel     string
		noColor      bool
	)

	log.SetOutput(io.Discard)
	FlagVersion(&printVersion)
	FlagLogLevel(&logLevel)
	FlagNoColor(&noColor)
	c.DefineFlags()
	if asSubcommand {
		if err := flagSet.Parse(os.Args[2:]); err != nil {
			// flagSet exits on error, we shouldn't get here
			cmd.Exit(err)
		}
	} else {
		if err := flagSet.Parse(os.Args[1:]); err != nil {
			// flagSet exits on error, we shouldn't get here
			cmd.Exit(err)
		}
	}
	cmd.DisableColor(noColor)

	if printVersion {
		cmd.ExitWithVersion()
	}
	if err := cmd.DefaultLogger.SetLevel(logLevel); err != nil {
		cmd.Exit(err)
	}

	// Warn when CNB_PLATFORM_API is unset
	if os.Getenv(env.VarPlatformAPI) == "" {
		cmd.DefaultLogger.Warnf("%s is unset; using Platform API version '%s'", env.VarPlatformAPI, platform.DefaultPlatformAPI)
		cmd.DefaultLogger.Infof("%s should be set to avoid breaking changes when upgrading the lifecycle", env.VarPlatformAPI)
	}

	if err := c.Args(flagSet.NArg(), flagSet.Args()); err != nil {
		cmd.Exit(err)
	}
	if err := c.Privileges(); err != nil {
		cmd.Exit(err)
	}
	cmd.Exit(c.Exec())
}
