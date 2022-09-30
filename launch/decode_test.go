package launch_test

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDecodeMetadataTOML(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "DecodeMetadataTOML", testDecodeMetataTOML, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testDecodeMetataTOML(t *testing.T, when spec.G, it spec.S) {
	when("decoding metadata.toml", func() {
		var (
			tmpDir string
		)

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "test-decode-metadata-toml")
			h.AssertNil(t, err)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		it("decodes array commands into command array", func() {
			path := filepath.Join(tmpDir, "launch.toml")
			h.Mkfile(t,
				`[[processes]]`+"\n"+
					`type = "some-type"`+"\n"+
					`command = ["some-cmd", "more"]`+"\n"+
					`default = true`+"\n"+
					`[[processes]]`+"\n"+
					`type = "web"`+"\n"+
					`command = ["other cmd with spaces", "other more"]`+"\n",
				// default is false and therefore doesn't appear
				filepath.Join(tmpDir, "launch.toml"),
			)

			metadata := launch.Metadata{}

			_, err := toml.DecodeFile(path, &metadata)
			h.AssertNil(t, err)
			h.AssertEq(t, metadata.Processes[0].Command[0], "some-cmd")
			h.AssertEq(t, metadata.Processes[0].Command[1], "more")

			h.AssertEq(t, metadata.Processes[1].Command[0], "other cmd with spaces")
			h.AssertEq(t, metadata.Processes[1].Command[1], "other more")
		})

		when("string commands", func() {
			it("decodes string commands into command array", func() {
				path := filepath.Join(tmpDir, "launch.toml")
				h.Mkfile(t,
					`[[processes]]`+"\n"+
						`type = "some-type"`+"\n"+
						`command = "some-cmd"`+"\n"+
						`default = true`+"\n"+
						`[[processes]]`+"\n"+
						`type = "web"`+"\n"+
						`command = "other cmd with spaces"`+"\n",
					// default is false and therefore doesn't appear
					filepath.Join(tmpDir, "launch.toml"),
				)

				metadata := launch.Metadata{}

				_, err := toml.DecodeFile(path, &metadata)
				h.AssertNil(t, err)
				h.AssertEq(t, metadata.Processes[0].Command[0], "some-cmd")
				h.AssertEq(t, metadata.Processes[1].Command[0], "other cmd with spaces")
			})
		})
	})
}
