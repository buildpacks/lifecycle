package lifecycle_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestAnalyzer(t *testing.T) {
	spec.Run(t, "Analyzer", testAnalyzer06, spec.Report(report.Terminal{}))
	spec.Run(t, "Analyzer", testAnalyzer07, spec.Report(report.Terminal{}))
}
