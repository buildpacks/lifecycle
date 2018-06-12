package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/packs"

	"github.com/sclevine/lifecycle"
)

var (
	listPath  string
	orderPath string
	groupPath string

	list []*lifecycle.Buildpack
)

func init() {
	packs.InputListPath(&listPath)
	packs.InputOrderPath(&orderPath)
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

	if _, err := toml.DecodeFile(listPath, &list); err != nil {
		return packs.FailErr(err, "read buildpack list")
	}

	var order lifecycle.BuildpackOrder
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return packs.FailErr(err, "read buildpack order")
	}

	group := order.Detect(lifecycle.DefaultAppDir, log.New(os.Stderr, "", log.LstdFlags))
	if len(group) == 0 {
		return packs.FailCode(packs.CodeFailedDetect, "detect")
	}

	groupFile, err := os.Create(groupPath)
	if err != nil {
		return packs.FailErr(err, "create buildpack group file")
	}
	defer groupFile.Close()
	if err := toml.NewEncoder(groupFile).Encode(group); err != nil {
		return packs.FailErr(err, "write buildpack group")
	}

	return nil
}

type buildpackGroup []*lifecycle.Buildpack

func (g buildpackGroup) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &g); err != nil {
		return err
	}

Group:
	for i, newBP := range g {
		for _, listBP := range list {
			if listBP.ID == newBP.ID {
				g[i] = listBP
				continue Group
			}
		}
		return fmt.Errorf("invalid buildpack ID: %s", newBP.ID)
	}
	return nil
}

type buildpackOrder []buildpackGroup

func (bo buildpackOrder) Order() lifecycle.BuildpackOrder {
	var order lifecycle.BuildpackOrder
	for _, bg := range bo {
		order = append(order, lifecycle.BuildpackGroup(bg))
	}
	return order
}
