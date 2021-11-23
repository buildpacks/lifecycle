package lifecycle

import (
	"os"
	"os/exec"
)

type Extender struct {
	Logger Logger
	Mode   string
}

func (e *Extender) Extend() error {
	mode := "kaniko"
	if e.Mode != ""{
		mode = e.Mode
	}

	cmd := exec.Command("/cnb/lifecycle/extender", mode)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
