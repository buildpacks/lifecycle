package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/sclevine/packs"

	"github.com/sclevine/lifecycle"
)

var (
	listPath  string
	groupPath string
)

func init() {
	packs.InputListPath(&listPath)
	packs.InputGroupPath(&groupPath)
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || listPath == "" || groupPath == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(detect())
}

func detect() error {
	flag.Parse()
	listFile, err := os.Open(listPath)
	if err != nil {
		return packs.FailErr(err, "open buildpack list file")
	}
	defer listFile.Close()
	var list lifecycle.BuildpackList
	if err := json.NewDecoder(listFile).Decode(&list); err != nil {
		return packs.FailErr(err, "read buildpack list")
	}
	group := list.Detect(lifecycle.DefaultAppDir, log.New(os.Stderr, "", log.LstdFlags))
	if len(group) == 0 {
		return packs.FailCode(packs.CodeFailedDetect, "detect")

	}
	groupFile, err := os.Create(groupPath)
	if err != nil {
		return packs.FailErr(err, "create buildpack group file")
	}
	defer groupFile.Close()
	if err := json.NewEncoder(groupFile).Encode(group); err != nil {
		return packs.FailErr(err, "write buildpack group")
	}
}
