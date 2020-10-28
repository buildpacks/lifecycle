package cmd

import (
	"io/ioutil"
	"log"
	"os"
)

// PreInit defines all the flags that are going to be used.
// If the default value is not going to be used,
// the flags will be set as part of the flags.Parse function.
// In Args, several paths will be changed to be under the updated layers directory (if the user didn't set them using a flag)
// TODO: should we add more documentation?
type Command interface {
	PreInit()
	Args(nargs int, args []string) error
	Privileges() error
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
	c.PreInit()
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
