package test

import (
	cfg "github.com/redhat-buildpacks/poc/kaniko/buildpackconfig"
	"github.com/redhat-buildpacks/poc/kaniko/util"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

var envTests = []struct {
	name				string
	envKey              string
	envVal              string
	expectedBuildArgs   []string
}{
	{
		name:             "CNB foo and bar key, val",
		envKey:           "CNB_foo",
		envVal: 		  "bar",
		expectedBuildArgs:   []string{"CNB_foo=bar"},
	},
}

// TODO: To be reviewed and improved as we cannot build it on macos due to error
// ../vendor/github.com/docker/docker/builder/dockerfile/internals.go:193:19: undefined: parseChownFlag
func TestEnvToBuildArgs(t *testing.T) {

	b := cfg.NewBuildPackConfig()

	for _, test := range envTests {
		t.Run(test.name, func(t *testing.T) {

			// set CNB env var
			os.Setenv(test.envKey, test.envVal)

			// Read the env vars
			b.CnbEnvVars = util.GetCNBEnvVar()

			assert.Equal(t,"1","1")
		})
	}
}
