package mocks

import (
	"fmt"
	"io"
	"strings"

	"github.com/buildpack/lifecycle/logging"

	"github.com/apex/log"
)

type mockLog struct {
	log.Logger
	w io.Writer
}

// NewMockLogger create a logger to capture output for testing purposes.
func NewMockLogger(w io.Writer) logging.Logger {
	ml := &mockLog{
		w: w,
	}
	ml.Logger.Handler = ml
	ml.Logger.Level = log.DebugLevel
	return ml
}

func (ml *mockLog) HandleLog(e *log.Entry) error {
	switch e.Level {
	case log.WarnLevel:
		_, _ = fmt.Fprintf(ml.w, "Warning: %s", e.Message)
	case log.ErrorLevel:
		_, _ = fmt.Fprintf(ml.w, "ERROR: %s", e.Message)
	default:
		_, _ = fmt.Fprintln(ml.w, strings.TrimRight(e.Message, "\n"))
	}

	return nil
}

func (ml *mockLog) Writer() io.Writer {
	return ml.w
}

func (ml *mockLog) WantLevel(level string) {
	switch {
	case level == logging.InfoLevel:
		ml.Logger.Level = log.InfoLevel
	case level == logging.DebugLevel:
		ml.Logger.Level = log.DebugLevel
	case level == logging.WarnLevel:
		ml.Logger.Level = log.WarnLevel
	default:
		ml.Logger.Level = log.ErrorLevel
	}
}
