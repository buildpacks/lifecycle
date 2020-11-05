package cmd

import (
	"io/ioutil"
	"log"
	"os"
)

// Command defines the interface for running the lifecycle phases
type Command interface {
	// Flags should be defined in DefineFlags
	DefineFlags()

	// Validation of the arguments and updates of the flags should happen in Args
	Args(nargs int, args []string) error

	// Validation of the needed priviledges should happen in Privileges
	Privileges() error

	// The command execution should happen in Exec
	Exec() error
}

func Run(c Command, asSubcommand bool) {
	var (
		printVersion bool
		logLevel     string
		noColor      bool
	)

	log.SetOutput(ioutil.Discard)
	FlagVersion(&printVersion)
	FlagLogLevel(&logLevel)
	FlagNoColor(&noColor)
	c.DefineFlags()
	if asSubcommand {
		if err := flagSet.Parse(os.Args[2:]); err != nil {
			//flagSet exits on error, we shouldn't get here
			Exit(err)
		}
	} else {
		if err := flagSet.Parse(os.Args[1:]); err != nil {
			//flagSet exits on error, we shouldn't get here
			Exit(err)
		}
	}
	DisableColor(noColor)

	if printVersion {
		ExitWithVersion()
	}
	if err := SetLogLevel(logLevel); err != nil {
		Exit(err)
	}
	if err := c.Args(flagSet.NArg(), flagSet.Args()); err != nil {
		Exit(err)
	}
	if err := c.Privileges(); err != nil {
		Exit(err)
	}
	Exit(c.Exec())
}
