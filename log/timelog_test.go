package log_test

import (
	"testing"
	"time"

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

func (m mockLog) Debug(_ string) {
	m.incr("Debug")
}
func (m mockLog) Debugf(_ string, _ ...any) {
	m.incr("Debug")
}
func (m mockLog) Info(_ string) {
	m.incr("Info")
}
func (m mockLog) Infof(_ string, _ ...any) {
	m.incr("Info")
}
func (m mockLog) Warn(_ string) {
	m.incr("Warn")
}
func (m mockLog) Warnf(_ string, _ ...any) {
	m.incr("Warn")
}
func (m mockLog) Error(_ string) {
	m.incr("Error")
}
func (m mockLog) Errorf(_ string, _ ...any) {
	m.incr("Error")
}

func TestTimeLog(t *testing.T) {
	t.Parallel()
	t.Run("we use the time log", func(t *testing.T) {
		t.Run("the granular api works step by step", func(t *testing.T) {
			logger := mockLog{callCount: map[string]int{}}
			c1 := log.Chronit{}
			nullTime := time.Time{}
			h.AssertEq(t, c1.StartTime, nullTime)
			h.AssertEq(t, c1.EndTime, nullTime)

			c1.Log = logger
			c1.RecordStart()
			h.AssertEq(t, logger.callCount["Debug"], 1)
			h.AssertEq(t, c1.StartTime.Equal(nullTime), false)
			h.AssertEq(t, c1.EndTime, nullTime)

			c1.RecordEnd()
			h.AssertEq(t, logger.callCount["Debug"], 2)
			h.AssertEq(t, c1.EndTime.Equal(nullTime), false)
		})
		t.Run("the convenience functions call the logger", func(t *testing.T) {
			logger := mockLog{callCount: map[string]int{}}
			endfunc := log.NewMeasurement("value", logger)
			h.AssertEq(t, logger.callCount["Debug"], 1)
			endfunc()
			h.AssertEq(t, logger.callCount["Debug"], 2)
		})
	})
}
