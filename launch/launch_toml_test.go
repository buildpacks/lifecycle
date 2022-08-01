package launch_test

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/google/go-cmp/cmp"
)

func TestNewLaunchTOML(t *testing.T) {
	launchTOMLContents := `[[processes]]
	type = "some-type"
	command = ["some-cmd", "some-cmd-arg"]
	args = ["some-arg"]`
	parsed := buildpack.LaunchTOML{}
	_, err := toml.Decode(launchTOMLContents, &parsed)
	if err != nil {
		t.Fatal(err)
	}
	expected := buildpack.LaunchTOML{
		Processes: []launch.Process{
			{
				Type:    "some-type",
				Command: "some-cmd",
				Args:    []string{"some-cmd-arg", "some-arg"},
			},
		},
	}
	if s := cmp.Diff(parsed, expected); s != "" {
		t.Fatalf("Unexpected:\n%s\n", s)
	}
}

func TestOldLaunchTOML(t *testing.T) {
	launchTOMLContents := `[[processes]]
	type = "some-type"
	command = "some-cmd"
	args = ["some-arg"]`
	parsed := buildpack.LaunchTOML{}
	_, err := toml.Decode(launchTOMLContents, &parsed)
	if err != nil {
		t.Fatal(err)
	}

	expected := buildpack.LaunchTOML{
		Processes: []launch.Process{
			{
				Type:    "some-type",
				Command: "some-cmd",
				Args:    []string{"some-arg"},
			},
		},
	}
	if s := cmp.Diff(parsed, expected); s != "" {
		t.Fatalf("Unexpected:\n%s\n", s)
	}
}
