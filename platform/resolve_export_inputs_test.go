package platform_test

import (
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExportInputs(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-export-inputs/"+api.String(), testResolveExportInputs(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testResolveExportInputs(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			inputs     *platform.LifecycleInputs
			logHandler *memory.Handler
			logger     llog.Logger
		)

		it.Before(func() {
			inputs = platform.NewLifecycleInputs(api.MustParse(platformAPI))
			inputs.OutputImageRef = "some-output-image" // satisfy validation
			logHandler = memory.New()
			logger = &log.Logger{Handler: logHandler}
			inputs.UseDaemon = true // to prevent access checking of run images
		})

		when("output image ref is empty", func() {
			it.Before(func() {
				inputs.OutputImageRef = ""
			})

			it("errors properly", func() {
				err := platform.ResolveInputs(platform.Export, inputs, logger)
				h.AssertError(t, err, "image argument is required")
			})
		})
	}
}
