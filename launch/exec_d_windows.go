package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const EnvExecDHandle = "CNB_EXEC_D_HANDLE"

func setHandle(cmd *exec.Cmd, pw *os.File) error {
	handle := pw.Fd()
	if err := syscall.SetHandleInformation(syscall.Handle(handle), syscall.HANDLE_FLAG_INHERIT, 1); err != nil {
		return err
	}

	envVal := "0x" + strconv.FormatUint(uint64(handle), 16)
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", EnvExecDHandle, envVal))
	return nil
}
