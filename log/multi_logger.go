package log

import (
	"io"

	"github.com/apex/log"
)

var _ Logger = &MultiLogger{}

type MultiLogger struct {
	defaultLogger *DefaultLogger
	infoLogger    *DefaultLogger
}

func NewMultiLogger(defaultReadWriter, infoReadWriter io.ReadWriter) *MultiLogger {
	return &MultiLogger{
		defaultLogger: &DefaultLogger{
			Logger: &log.Logger{
				Handler: &handler{
					writer: defaultReadWriter,
				},
			},
			Reader: defaultReadWriter,
		},
		infoLogger: &DefaultLogger{
			Logger: &log.Logger{
				Level: log.DebugLevel,
				Handler: &handler{
					writer: infoReadWriter,
				},
			},
			Reader: infoReadWriter,
		},
	}
}

func (l *MultiLogger) DefaultReader() io.Reader {
	return l.defaultLogger
}

func (l *MultiLogger) InfoReader() io.Reader {
	return l.infoLogger
}

func (l *MultiLogger) SetLevel(requested string) error {
	return l.defaultLogger.SetLevel(requested)
}

func (l *MultiLogger) Debug(msg string) {
	l.defaultLogger.Debug(msg)
	l.infoLogger.Info(msg)
}

func (l *MultiLogger) Debugf(fmt string, v ...interface{}) {
	l.defaultLogger.Debugf(fmt, v...)
	l.infoLogger.Infof(fmt, v...)
}

func (l *MultiLogger) Info(msg string) {
	l.defaultLogger.Info(msg)
	l.infoLogger.Info(msg)
}

func (l *MultiLogger) Infof(fmt string, v ...interface{}) {
	l.defaultLogger.Infof(fmt, v...)
	l.infoLogger.Infof(fmt, v...)
}

func (l *MultiLogger) Warn(msg string) {
	l.defaultLogger.Warn(msg)
	l.infoLogger.Warn(msg)
}

func (l *MultiLogger) Warnf(fmt string, v ...interface{}) {
	l.defaultLogger.Warnf(fmt, v...)
	l.infoLogger.Warnf(fmt, v...)
}

func (l *MultiLogger) Error(msg string) {
	l.defaultLogger.Error(msg)
	l.infoLogger.Error(msg)
}

func (l *MultiLogger) Errorf(fmt string, v ...interface{}) {
	l.defaultLogger.Errorf(fmt, v...)
	l.infoLogger.Errorf(fmt, v...)
}
