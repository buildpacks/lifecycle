package lifecycle_test

import (
	"os"
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/common"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestStackValidation(t *testing.T) {
	spec.Run(t, "StackValidation", testStackValidation, spec.Report(report.Terminal{}))
}

func testStackValidation(t *testing.T, when spec.G, it spec.S) {
	when("ValidateStack", func() {
		when("build and run stack ids match", func() {
			it("should not err", func() {
				md := common.StackMetadata{BuildImage: common.StackBuildImageMetadata{StackID: "my-stack"}}
				runImage := fakes.NewImage("runimg", "", nil)
				h.AssertNil(t, runImage.SetLabel(platform.StackIDLabel, "my-stack"))
				err := lifecycle.ValidateStack(md, runImage)
				h.AssertNil(t, err)
			})
		})

		when("build and run stack ids do not match", func() {
			it("should fail", func() {
				md := common.StackMetadata{BuildImage: common.StackBuildImageMetadata{StackID: "my-stack"}}
				runImage := fakes.NewImage("runimg", "", nil)
				h.AssertNil(t, runImage.SetLabel(platform.StackIDLabel, "my-other-stack"))
				err := lifecycle.ValidateStack(md, runImage)
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "incompatible stack: 'my-other-stack' is not compatible with 'my-stack'")
			})
		})

		when("run image is missing io.buildpacks.stack.id label", func() {
			it("should fail", func() {
				md := common.StackMetadata{BuildImage: common.StackBuildImageMetadata{StackID: "my-stack"}}
				runImage := fakes.NewImage("runimg", "", nil)
				err := lifecycle.ValidateStack(md, runImage)
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "get run image label: io.buildpacks.stack.id")
			})
		})

		when("CNB_STACK_ID is present", func() {
			it.Before(func() {
				os.Setenv("CNB_STACK_ID", "my-stack")
			})

			it.After(func() {
				h.AssertNil(t, os.Unsetenv("CNB_STACK_ID"))
			})

			it("prefers that value", func() {
				md := common.StackMetadata{BuildImage: common.StackBuildImageMetadata{StackID: "my-other-stack"}}
				runImage := fakes.NewImage("runimg", "", nil)
				h.AssertNil(t, runImage.SetLabel(platform.StackIDLabel, "my-stack"))
				err := lifecycle.ValidateStack(md, runImage)
				h.AssertNil(t, err)
			})
		})

		when("no build stack is present", func() {
			it("should fail", func() {
				md := common.StackMetadata{BuildImage: common.StackBuildImageMetadata{StackID: ""}}
				runImage := fakes.NewImage("runimg", "", nil)
				h.AssertNil(t, runImage.SetLabel(platform.StackIDLabel, "my-stack"))
				err := lifecycle.ValidateStack(md, runImage)
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "CNB_STACK_ID is required when there is no stack metadata available")
			})
		})
	})
}
