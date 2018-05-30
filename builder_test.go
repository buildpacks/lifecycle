package lifecycle_test

import (
	"io/ioutil"
	"testing"
	"path/filepath"

	"github.com/sclevine/spec"

	"github.com/sclevine/lifecycle"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "#Build", testBuild)
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	it("builds an app", func() {
		tmpDir, err := ioutil.TempDir("", "test.lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		_ := lifecycle.Builder{
			PlatformDir: filepath.Join(tmpDir, "platform"),
			Buildpacks: lifecycle.BuildpackGroup{
				{ID: "some-id-1", Name: "some-name-1", Dir: filepath.Join(tmpDir, "some-dir-1")},
				{ID: "some-id-2", Name: "some-name-2", Dir: filepath.Join(tmpDir, "some-dir-2")},
			},
			Out: it.Out(), Err: it.Out(), // TODO: test output
		}
	})
}
