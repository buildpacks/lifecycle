package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/packs"

	"github.com/sclevine/lifecycle"
)

var (
	buildpackPath string
	orderPath     string
	groupPath     string
	infoPath      string

	buildpacks lifecycle.BuildpackMap
)

func init() {
	packs.InputBPPath(&buildpackPath)
	packs.InputBPOrderPath(&orderPath)

	packs.InputBPGroupPath(&groupPath)
	packs.InputDetectInfoPath(&infoPath)

	buildpacks = lifecycle.BuildpackMap{}
}

func main() {
	flag.Parse()
	if flag.NArg() != 0 || buildpackPath == "" || orderPath == "" || groupPath == "" || infoPath == "" {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse arguments"))
	}
	packs.Exit(detect())
}

func detect() error {
	var err error
	buildpacks, err = lifecycle.NewBuildpackMap(buildpackPath)
	if err != nil {
		return packs.FailErr(err, "read buildpack directory")
	}

	var order buildpackRefOrder
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return packs.FailErr(err, "read buildpack order")
	}

	log := log.New(os.Stderr, "", log.LstdFlags)
	info, group := order.order().Detect(log, lifecycle.DefaultAppDir)
	if len(group.Buildpacks) == 0 {
		return packs.FailCode(packs.CodeFailedDetect, "detect")
	}

	groupFile, err := os.Create(groupPath)
	if err != nil {
		return packs.FailErr(err, "create buildpack group file")
	}
	defer groupFile.Close()
	if err := toml.NewEncoder(groupFile).Encode(struct {
		Buildpacks []string `toml:"buildpacks"`
		Repository string   `toml:"repository"`
	}{
		Buildpacks: group.List(),
		Repository: group.Repository,
	}); err != nil {
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
	var ref string
	if err := toml.Unmarshal(b, &ref); err != nil {
		return err
	}
	if !strings.Contains(ref, "@") {
		ref = ref + "@latest"
	}
	var ok bool
	if bp.Buildpack, ok = buildpacks[ref]; !ok {
		return fmt.Errorf("invalid buildpack reference: %s", ref)
	}
	return nil
}

type buildpackRefGroup struct {
	Buildpacks []buildpackRef
	Repository string
}

func (bps buildpackRefGroup) group() lifecycle.BuildpackGroup {
	var group lifecycle.BuildpackGroup
	for _, bp := range bps.Buildpacks {
		group.Buildpacks = append(group.Buildpacks, bp.Buildpack)
	}
	group.Repository = bps.Repository
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
