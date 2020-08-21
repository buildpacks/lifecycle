package launch

import (
	"fmt"
	"io/ioutil"
	"os"
	"syscall"

	"github.com/pkg/errors"
)

type CmdShell struct {
	Exec ExecFunc
}

// Launch launches the given ShellProcess with cmd
func (c *CmdShell) Launch(proc ShellProcess) error {
	var commandTokens []string
	for _, profile := range proc.Profiles {
		commandTokens = append(commandTokens, "call", profile, "&&")
	}
	script, err := batchScript(proc)
	if err != nil {
		return errors.Wrap(err, "failed to create batch script from process")
	}
	defer os.RemoveAll(script)
	commandTokens = append(commandTokens, script)
	if err := c.Exec("cmd",
		append([]string{"cmd", "/q", "/c"}, commandTokens...), proc.Env,
	); err != nil {
		return errors.Wrap(err, "cmd execute")
	}
	return nil
}

// batchScript writes the process to a temporarily batch script.
//
// The process must be run from a script instead provided in the command line so that environment evaluation is delayed
// until after the profile scripts have been called.
func batchScript(proc ShellProcess) (path string, err error) {
	script := syscall.EscapeArg(proc.Command)
	for _, arg := range proc.Args {
		script += fmt.Sprintf(" %s", syscall.EscapeArg(arg))
	}

	f, err := ioutil.TempFile("", "proc-*.bat")
	if err != nil {
		return "", errors.Wrap(err, "failed to create temporary script file")
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	_, err = f.WriteString(script)
	if err != nil {
		return "", errors.Wrap(err, "failed to write script to file")
	}
	return f.Name(), nil
}
