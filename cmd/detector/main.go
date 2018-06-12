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

	list []lifecycle.Buildpack
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
		return packs.FailErr(err, "read buildpackRef list")
	}

	var order buildpackRefOrder
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return packs.FailErr(err, "read buildpackRef order")
	}

	group := order.order().Detect(lifecycle.DefaultAppDir, log.New(os.Stderr, "", log.LstdFlags))
	if len(group) == 0 {
		return packs.FailCode(packs.CodeFailedDetect, "detect")
	}

	groupFile, err := os.Create(groupPath)
	if err != nil {
		return packs.FailErr(err, "create buildpackRef group file")
	}
	defer groupFile.Close()
	if err := toml.NewEncoder(groupFile).Encode(group); err != nil {
		return packs.FailErr(err, "write buildpackRef group")
	}

	return nil
}

type buildpackRef struct {
	*lifecycle.Buildpack
}

func (bp *buildpackRef) UnmarshalText(b []byte) error {
	var id string
	if err := toml.Unmarshal(b, &id); err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			bp.Buildpack = &list[i]
			return nil
		}
	}
	return fmt.Errorf("invalid buildpackRef ID: %s", id)
}

type buildpackRefGroup []buildpackRef

func (bps buildpackRefGroup) group() (lifecycle.BuildpackGroup) {
	var group lifecycle.BuildpackGroup
	for _, bp := range bps {
		group = append(group, bp.Buildpack)
	}
	return group
}

type buildpackRefOrder []buildpackRefGroup

func (gs buildpackRefOrder) order() lifecycle.BuildpackOrder {
	var order lifecycle.BuildpackOrder
	for _, g := range gs {
		order = append(order, g.group())
	}
	return order
}
