package logging

import (
	"os"

	"github.com/Sirupsen/logrus"
)

var log *logrus.Logger

func init() {
	log = &logrus.Logger{
		Formatter: new(logrus.TextFormatter),
		Out:       os.Stdout,
		Level:     logrus.DebugLevel,
	}
}

func Log() *logrus.Logger {
	return log
}
