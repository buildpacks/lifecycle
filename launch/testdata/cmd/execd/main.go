package main

import (
	"fmt"
	"os"
)

func main() {
	if _, err := os.Stdout.WriteString("stdout from execd\n"); err != nil {
		fmt.Println("ERROR: failed to write to stdout:", err)
		os.Exit(1)
	}
	if _, err := os.Stderr.WriteString("stderr from execd\n"); err != nil {
		fmt.Println("ERROR: failed to write to stderr:", err)
		os.Exit(1)
	}

	f := os.NewFile(3, "/dev/fd/3")
	defer f.Close()
	val := "SOME_VAL"
	if orig := os.Getenv("SOME_VAR"); orig != "" {
		val = orig + "|" + val
	}
	if _, err := f.WriteString(fmt.Sprintf(`SOME_VAR = "%s"`, val)); err != nil {
		fmt.Println("ERROR: failed to write to FD 3:", err)
		os.Exit(1)
	}
	os.Exit(0)
}
