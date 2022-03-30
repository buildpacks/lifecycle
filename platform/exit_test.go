package platform_test

import (
	"fmt"
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExiter(t *testing.T) {
	spec.Run(t, "Test Exiter", testExiter)
}

func testExiter(t *testing.T, when spec.G, it spec.S) {
	type expected struct {
		version string
		exiter  interface{}
	}
	toTest := []expected{
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
		for _, expected := range toTest {
			if expected.version == apiVersion.String() {
				when(fmt.Sprintf("NewExiter for platform %s", expected.version), func() {
					it("returns the right type", func() {
						foundExiter := platform.NewExiter(expected.version)

						switch expected.exiter.(type) {
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
