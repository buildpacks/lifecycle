package main

import (
	"os"
	"strconv"
)

const EnvExecDHandle = "CNB_EXEC_D_HANDLE"

func outputFile() (*os.File, error) {
	handle, err := strconv.ParseInt(os.Getenv(EnvExecDHandle), 0, 64)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(handle), "outputFile"), nil
}
