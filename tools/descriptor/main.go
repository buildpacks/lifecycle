package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Descriptor struct {
	APIs      `toml:"apis"`
	Lifecycle Lifecycle `toml:"lifecycle"`
}

type Lifecycle struct {
	Version string `toml:"version"`
}

type APIs struct {
	Buildpack APISet `toml:"buildpack"`
	Platform  APISet `toml:"platform"`
}

type APISet struct {
	Deprecated []string
	Supported  []string
}

const (
	gitRepository = "github.com/buildpacks/lifecycle"
)

// build-args generates ldflags from descriptor
// version parses and print version from descriptor
func main() {
	if len(os.Args) != 3 {
		usageAndExit()
	}
	descriptorPath := os.Args[2]
	descriptor := Descriptor{}
	_, err := toml.DecodeFile(descriptorPath, &descriptor)
	if err != nil {
		fmt.Printf("Failed to decode '%s': %s", descriptorPath, err)
		os.Exit(2)
	}

	switch os.Args[1] { //just print the version
	case "version":
		fmt.Print(descriptor.Lifecycle.Version)
	case "build-args":
		fmt.Print(buildArgs(descriptor))
	default:
		usageAndExit()
	}
}

func buildArgs(descriptor Descriptor) string {
	flags := []string{
		fmt.Sprintf(
			"-X 'github.com/buildpacks/lifecycle/cmd.DeprecatedBuildpackAPIs=%s'",
			strings.Join(descriptor.Buildpack.Deprecated, `,`),
		),
		fmt.Sprintf(
			"-X 'github.com/buildpacks/lifecycle/cmd.SupportedBuildpackAPIs=%s'",
			strings.Join(descriptor.Buildpack.Supported, `,`),
		),
		fmt.Sprintf(
			"-X 'github.com/buildpacks/lifecycle/cmd.DeprecatedPlatformAPIs=%s'",
			strings.Join(descriptor.Platform.Deprecated, `,`),
		),
		fmt.Sprintf(
			"-X 'github.com/buildpacks/lifecycle/cmd.SupportedPlatformAPIs=%s'",
			strings.Join(descriptor.Platform.Supported, `,`),
		),
		fmt.Sprintf(
			"-X 'github.com/buildpacks/lifecycle/cmd.Version=%s'",
			descriptor.Lifecycle.Version,
		),
	}
	return strings.Join(flags, " ")
}

func usageAndExit() {
	fmt.Println("USAGE: tools/descriptor/main.go [build-args, version] <path-to-lifecycle-descriptor-file>")
	os.Exit(1)
}
