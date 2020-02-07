package log

import (
	_log "github.com/apex/log"
	"os"
)

func init() {
	_log.SetHandler(newColored(os.Stderr))
	_log.SetLevel(_log.DebugLevel)
}

type Fields = _log.Fields
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
