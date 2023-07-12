package cli

import (
	"io"
	"log"
	"os"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
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

func Run(c Command, withPhaseName string, asSubcommand bool) {
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
	cmd.DefaultLogger.Debugf("Starting %s...", withPhaseName)

	for _, arg := range flagSet.Args() {
		if arg[0:1] == "-" {
			cmd.DefaultLogger.Warnf("Warning: unconsumed flag-like positional arg: \n\t%s\n\t This will not be interpreted as a flag.\n\t Did you mean to put this before the first positional argument?", arg)
		}
	}

	// Warn when CNB_PLATFORM_API is unset
	if os.Getenv(platform.EnvPlatformAPI) == "" {
		cmd.DefaultLogger.Warnf("%s is unset; using Platform API version '%s'", platform.EnvPlatformAPI, platform.DefaultPlatformAPI)
		cmd.DefaultLogger.Infof("%s should be set to avoid breaking changes when upgrading the lifecycle", platform.EnvPlatformAPI)
	}

	cmd.DefaultLogger.Debugf("Parsing inputs...")
	if err := c.Args(flagSet.NArg(), flagSet.Args()); err != nil {
		cmd.Exit(err)
	}
	cmd.DefaultLogger.Debugf("Ensuring privileges...")
	if err := c.Privileges(); err != nil {
		cmd.Exit(err)
	}
	cmd.DefaultLogger.Debugf("Executing command...")
	cmd.Exit(c.Exec())
}
