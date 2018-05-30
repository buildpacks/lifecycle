package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/sclevine/lifecycle"
)

var (
	listPath  string
	groupPath string
)

func init() {
	flag.StringVar(&listPath, "list", "", "buildpack list file path (JSON)")
	flag.StringVar(&groupPath, "group", "", "buildpack group output file path (JSON)")
}

func main() {
	flag.Parse()
	listFile, err := os.Open(listPath)
	if err != nil {
		log.Fatalln("Failed to open buildpack list file:", err)
	}
	defer listFile.Close()
	var list lifecycle.BuildpackList
	if err := json.NewDecoder(listFile).Decode(&list); err != nil {
		log.Fatalln("Failed to read buildpack list:", err)
	}
	group := list.Detect(lifecycle.DefaultAppDir, log.New(os.Stderr, "", log.LstdFlags))
	if err != nil {
		log.Fatalln("Failed to detect:", err)
	}
	groupFile, err := os.Create(groupPath)
	if err != nil {
		log.Fatalln("Failed to create buildpack group file:", err)
	}
	defer groupFile.Close()
	if err := json.NewEncoder(groupFile).Encode(group); err != nil {
		log.Fatalln("Failed to write buildpack group:", err)
	}
}
