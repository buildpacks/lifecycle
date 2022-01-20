package logging

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// Text format
	FormatText = "text"
	// Colored text format
	FormatColor = "color"
	// JSON format
	FormatJSON = "json"
)

// Configure sets the logrus logging level and formatter
func Configure(level, format string, logTimestamp bool) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return errors.Wrap(err, "parsing log level")
	}
	logrus.SetLevel(lvl)

	var formatter logrus.Formatter
	switch format {
	case FormatText:
		formatter = &logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: logTimestamp,
		}
	case FormatColor:
		formatter = &logrus.TextFormatter{
			ForceColors:   true,
			FullTimestamp: logTimestamp,
		}
	case FormatJSON:
		formatter = &logrus.JSONFormatter{}
	default:
		return fmt.Errorf("not a valid log format: %q. Please specify one of (text, color, json)", format)
	}
	logrus.SetFormatter(formatter)

	return nil
}

