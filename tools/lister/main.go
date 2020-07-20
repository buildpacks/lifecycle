package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

type Package struct {
	Dir     string
	GoFiles []string
}

func main() {
	cmd := exec.Command(
		"go",
		"list",
		"-json",
		"./...",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		panic(err)
	}

	var paths []string

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for {
		var pkg Package
		if err := dec.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			panic("Could not parse output:" + string(stdout.Bytes()))
		}

		for _, file := range pkg.GoFiles {
			paths = append(paths, filepath.Join(pkg.Dir, file))
		}
	}

	fmt.Println(strings.Join(paths, "\n"))
}
