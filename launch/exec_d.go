package launch

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/BurntSushi/toml"
)

type ExecDRunner struct {
	Out, Err io.Writer
}

func NewExecDRunner() *ExecDRunner {
	return &ExecDRunner{
		Out: os.Stdout,
		Err: os.Stderr,
	}
}

func (e *ExecDRunner) ExecD(path string, env Env) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	errChan := make(chan error, 1)
	go func() {
		defer pw.Close()
		cmd := exec.Command(path)
		cmd.Stdout = e.Out
		cmd.Stderr = e.Err
		cmd.ExtraFiles = []*os.File{pw}
		cmd.Env = env.List()
		errChan <- cmd.Run()
	}()

	out, err := ioutil.ReadAll(pr)
	if cmdErr := <-errChan; cmdErr != nil {
		return cmdErr // prefer the error from the command
	} else if err != nil {
		return err // return the read error only if the command succeeded
	}

	envVars := map[string]string{}
	if _, err := toml.Decode(string(out), &envVars); err != nil {
		return err
	}
	for k, v := range envVars {
		env.Set(k, v)
	}
	return nil
}
