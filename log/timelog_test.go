package log_test

import (
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

type mockLog struct {
	callCount map[string]int
}

func (m mockLog) incr(key string) {
	val, ok := m.callCount[key]
	if !ok {
		m.callCount[key] = 1
	} else {
		m.callCount[key] = val + 1
	}
}

func (m mockLog) Debug(msg string) {
	m.incr("Debug")
}
func (m mockLog) Debugf(fmt string, v ...interface{}) {
	m.incr("Debug")
}
func (m mockLog) Info(msg string) {
	m.incr("Info")
}
func (m mockLog) Infof(fmt string, v ...interface{}) {
	m.incr("Info")
}
func (m mockLog) Warn(msg string) {
	m.incr("Warn")
}
func (m mockLog) Warnf(fmt string, v ...interface{}) {
	m.incr("Warn")
}
func (m mockLog) Error(msg string) {
	m.incr("Error")
}
func (m mockLog) Errorf(fmt string, v ...interface{}) {
	m.incr("Error")
}

func TestTimeLog(t *testing.T) {
	spec.Run(t, "Exporter", testTimeLog, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testTimeLog(t *testing.T, when spec.G, it spec.S) {
	when("we use the time log", func() {
		it("the granular api works step by step", func() {
			logger := mockLog{callCount: map[string]int{}}
			c1 := log.Chronit{}
			nullTime := time.Time{}
			h.AssertEq(t, c1.StartTime, nullTime)
			h.AssertEq(t, c1.EndTime, nullTime)

			c1.Log = logger
			c1.RecordStart()
			h.AssertEq(t, logger.callCount["Info"], 1)
			h.AssertEq(t, c1.StartTime == nullTime, false)
			h.AssertEq(t, c1.EndTime, nullTime)

			c1.RecordEnd()
			h.AssertEq(t, logger.callCount["Info"], 2)
			h.AssertEq(t, c1.EndTime == nullTime, false)
		})
		it("the convenience functions call the logger", func() {
			logger := mockLog{callCount: map[string]int{}}
			endfunc := log.NewMeasurement("value", logger)
			h.AssertEq(t, logger.callCount["Info"], 1)
			endfunc()
			h.AssertEq(t, logger.callCount["Info"], 2)
		})
	})
}
