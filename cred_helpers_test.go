package lifecycle_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestCredHelpers(t *testing.T) {
	spec.Run(t, "Cred Helpers", testCredHelpers, spec.Report(report.Terminal{}))
}

func testCredHelpers(t *testing.T, when spec.G, it spec.S) {
	var (
		err    error
		tmpDir string
	)

	it.Before(func() {
		tmpDir, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	it("writes cred helpers for all known registries", func() {
		err := lifecycle.SetupCredHelpers(tmpDir, "fake.gcr.io/test1", "fake.amazonaws.com/test2", "fake.azurecr.io/test3")
		h.AssertNil(t, err)

		config := parseCredHelpers(t, tmpDir)
		h.AssertEq(t, config["fake.gcr.io"], "gcr")
		h.AssertEq(t, config["fake.amazonaws.com"], "ecr-login")
		h.AssertEq(t, config["fake.azurecr.io"], "acr")
	})

	when("a docker config exists", func() {
		when("registry credentials do not exist in the config", func() {
			it.Before(func() {
				copyFile(t, filepath.Join("testdata", "cred_helpers", "config1.json"), filepath.Join(tmpDir, "config.json"))
			})

			it("adds the registry and helper to the credHelpers section", func() {
				err := lifecycle.SetupCredHelpers(tmpDir, "fake.gcr.io/hello-world")
				h.AssertNil(t, err)

				config := parseCredHelpers(t, tmpDir)
				h.AssertEq(t, config["fake.gcr.io"], "gcr")
				h.AssertEq(t, config["fake.amazonaws.com"], "some-aws-helper")
				h.AssertEq(t, config["fake.azurecr.io"], "some-azure-helper")
			})
		})

		when("registry credentials do exist in the config", func() {
			it.Before(func() {
				copyFile(t, filepath.Join("testdata", "cred_helpers", "config2.json"), filepath.Join(tmpDir, "config.json"))
			})

			it("keeps the existing the registry and helper in the credHelpers section", func() {
				err := lifecycle.SetupCredHelpers(tmpDir, "fake.gcr.io/hello-world")
				h.AssertNil(t, err)

				config := parseCredHelpers(t, tmpDir)
				h.AssertEq(t, config["fake.amazonaws.com"], "some-aws-helper")
				h.AssertEq(t, config["fake.azurecr.io"], "some-azure-helper")
				h.AssertEq(t, config["fake.gcr.io"], "gcloud")
			})
		})
	})

	when("a docker config does not exist", func() {
		it("creates a config file with registry and helper in the credHelpers section", func() {
			err := lifecycle.SetupCredHelpers(tmpDir, "fake.gcr.io/hello-world")
			h.AssertNil(t, err)

			config := parseCredHelpers(t, tmpDir)
			h.AssertEq(t, config["fake.gcr.io"], "gcr")
		})
	})
}

func copyFile(t *testing.T, src, dest string) {
	buf, err := ioutil.ReadFile(src)
	h.AssertNil(t, err)
	h.AssertNil(t, ioutil.WriteFile(dest, buf, 0666))
}

func parseCredHelpers(t *testing.T, path string) map[string]interface{} {
	f, err := os.Open(filepath.Join(path, "config.json"))
	h.AssertNil(t, err)
	defer f.Close()

	config := make(map[string]interface{})
	err = json.NewDecoder(f).Decode(&config)
	h.AssertNil(t, err)

	helpers, ok := config["credHelpers"]
	if !ok {
		t.Fatalf("no credhelpers in config: %+v", config)
	}

	return helpers.(map[string]interface{})
}
