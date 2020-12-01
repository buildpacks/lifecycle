package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	fmt.Printf("%s was executed\n", os.Args[0])
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("ERROR: failed to get working dir:", err)
		os.Exit(1)
	}
	fmt.Println("Exec.d Working Dir:", wd)
	f := os.NewFile(3, "fd3")

	parent := filepath.Base(filepath.Dir(os.Args[0]))
	val := "val-from-exec.d"
	if parent != "exec.d" {
		val = "val-from-exec.d-for-process-type-" + parent
	}
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
