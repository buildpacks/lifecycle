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

	// Inputs returns the platform inputs
	Inputs() platform.LifecycleInputs

	// Args validates arguments and flags, and fills in default values
	Args(nargs int, args []string) error

	// Privileges validates the needed privileges
	Privileges() error

	// Exec executes the command
	Exec() error
}

func Run(c Command, withPhaseName string, asSubcommand bool) {
	log.SetOutput(io.Discard)

	var printVersion bool
	FlagVersion(&printVersion)

	// DefineFlags (along with any function FlagXXX) defines the flags that are considered valid,
	// but does not read the provided values; this is done by `flagSet.Parse`.
	// The command `c` (e.g., detectCmd) is at this point already populated with platform inputs from the environment and/or default values,
	// so command-line flags always take precedence.
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

	if printVersion {
		cmd.ExitWithVersion()
	}

	cmd.DisableColor(c.Inputs().NoColor)
	if err := cmd.DefaultLogger.SetLevel(c.Inputs().LogLevel); err != nil {
		cmd.Exit(err)
	}

	// We print a warning here, so we should disable color if needed and set the log level before exercising this logic.
	for _, arg := range flagSet.Args() {
		if len(arg) == 0 {
			continue
		}
		if arg[0:1] == "-" {
			cmd.DefaultLogger.Warnf("Warning: unconsumed flag-like positional arg: \n\t%s\n\t This will not be interpreted as a flag.\n\t Did you mean to put this before the first positional argument?", arg)
		}
	}

	cmd.DefaultLogger.Debugf("Starting %s...", withPhaseName)

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
