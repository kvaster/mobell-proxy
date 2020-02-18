package log

import (
	"flag"
	_log "github.com/apex/log"
	"os"
)

var logFile = flag.String("log.file", "", "log file")
var logLevel = flag.String("log.level", "", "log level")

func init() {
	_log.SetHandler(newColored(os.Stderr))
	_log.SetLevel(_log.DebugLevel)
}

func ParseFlags() {
	if *logLevel != "" {
		l, err := _log.ParseLevel(*logLevel)
		if err != nil {
			_log.WithField("level", *logLevel).Error("error parsing log level")
			os.Exit(1)
		}

		_log.SetLevel(l)
	}

	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0660)
		if err != nil {
			_log.WithError(err).Error("error opening log file")
			os.Exit(1)
		}
		_log.SetHandler(newPlain(f))
	}
}

type Interface = _log.Interface
type Fields = _log.Fields
var Log = _log.Log
var WithFields = _log.WithFields
var WithField = _log.WithField
var WithError = _log.WithError
var Debug = _log.Debug
var Info = _log.Info
var Warn = _log.Warn
var Error = _log.Error
var Fatal = _log.Fatal
var Debugf = _log.Debugf
var Infof = _log.Infof
var Warnf = _log.Warnf
var Errorf = _log.Errorf
var Fatalf = _log.Fatalf
var Trace = _log.Trace
