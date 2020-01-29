package cmd

import (
	"io/ioutil"
	"log"
)

type Command interface {
	Flags()
	Args() error
	Exec() error
}

func Run(c Command) {
	var (
		printVersion bool
		logLevel     string
	)

	log.SetOutput(ioutil.Discard)
	FlagVersion(&printVersion)
	FlagLogLevel(&logLevel)
	c.Flags()

	if printVersion {
		ExitWithVersion()
	}
	if err := SetLogLevel(logLevel); err != nil {
		Exit(err)
	}
	if err := c.Args(); err != nil {
		Exit(err)
	}
	Exit(c.Exec())
}
