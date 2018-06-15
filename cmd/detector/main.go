package main

import (
	"flag"
	"fmt"
	"io/ioutil"
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
	infoPath  string

	list []lifecycle.Buildpack
)

func init() {
	packs.InputBPListPath(&listPath)
	packs.InputBPOrderPath(&orderPath)
	packs.InputBPGroupPath(&groupPath)
	packs.InputDetectInfoPath(&infoPath)
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

	var order buildpackRefOrder
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return packs.FailErr(err, "read buildpack order")
	}

	info, group := order.order().Detect(log.New(os.Stderr, "", log.LstdFlags), lifecycle.DefaultAppDir)
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

	if err := ioutil.WriteFile(infoPath, info, 0666); err != nil {
		return packs.FailErr(err, "write detect info")
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

func (bps buildpackRefGroup) group() lifecycle.BuildpackGroup {
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
