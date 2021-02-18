package main

import (
	"os"
	"strconv"
	"unsafe"
)

const EnvExecDHandle = "CNB_EXEC_D_HANDLE"

func outputFile() (*os.File, error) {
	var i uintptr
	handle, err := strconv.ParseInt(os.Getenv(EnvExecDHandle), 0, int(unsafe.Sizeof(i)))
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(handle), "outputFile"), nil
}
