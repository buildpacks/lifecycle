package platform_test

import (
	"fmt"
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPlatform(t *testing.T) {
	spec.Run(t, "Test Platform", testPlatform)
}

func testPlatform(t *testing.T, when spec.G, it spec.S) {
	type expectedPlatform struct {
		version string
		exiter  interface{}
	}
	toTest := []expectedPlatform{
		{
			version: "0.3",
			exiter:  &platform.LegacyExiter{},
		},
		{
			version: "0.4",
			exiter:  &platform.LegacyExiter{},
		},
		{
			version: "0.5",
			exiter:  &platform.LegacyExiter{},
		},
		{
			version: "0.6",
			exiter:  &platform.DefaultExiter{},
		},
		{
			version: "0.7",
			exiter:  &platform.DefaultExiter{},
		},
		{
			version: "0.8",
			exiter:  &platform.DefaultExiter{},
		},
		{
			version: "0.9",
			exiter:  &platform.DefaultExiter{},
		},
	}
	for _, apiVersion := range api.Platform.Supported {
		for _, expectedPlatform := range toTest {
			if expectedPlatform.version == apiVersion.String() {
				when(fmt.Sprintf("platform %s", expectedPlatform.version), func() {
					it("is configured correctly", func() {
						testedPlatform := platform.NewPlatform(expectedPlatform.version)

						// api version
						h.AssertEq(t, testedPlatform.API().String(), expectedPlatform.version)

						// exiter
						foundExiter := testedPlatform.Exiter
						switch expectedPlatform.exiter.(type) {
						case *platform.DefaultExiter:
							_, ok := foundExiter.(*platform.DefaultExiter)
							h.AssertEq(t, ok, true)
						case *platform.LegacyExiter:
							_, ok := foundExiter.(*platform.LegacyExiter)
							h.AssertEq(t, ok, true)
						default:
							t.Fatalf("unexpected exiter")
						}
					})
				})
			}
		}
	}
}
