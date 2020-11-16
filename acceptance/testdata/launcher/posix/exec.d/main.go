package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Printf("%s was executed\n", os.Args[0])
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("ERROR: failed to get working dir:", err)
		os.Exit(1)
	}
	fmt.Println("Exec.d Working Dir:", wd)
	f := os.NewFile(3, "/dev/fd/3")
	val := "VAL_FROM_EXEC_D"
	if orig := os.Getenv("VAR_FROM_EXEC_D"); orig != "" {
		val = orig + ":" + val
	}
	defer f.Close()
	if _, err := f.WriteString(fmt.Sprintf(`VAR_FROM_EXEC_D = "%s"`, val)); err != nil {
		fmt.Println("ERROR: failed to write to FD 3:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
