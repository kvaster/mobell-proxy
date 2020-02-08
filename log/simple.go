package log

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/apex/log"
)

// colors.
const (
	none   = 0
	red    = 31
	green  = 32
	yellow = 33
	blue   = 34
	gray   = 37
)

// Colors mapping.
var colors = [...]int{
	log.DebugLevel: gray,
	log.InfoLevel:  blue,
	log.WarnLevel:  yellow,
	log.ErrorLevel: red,
	log.FatalLevel: red,
}

// Strings mapping.
var strings = [...]string{
	log.DebugLevel: "debug",
	log.InfoLevel:  "info",
	log.WarnLevel:  "warn",
	log.ErrorLevel: "error",
	log.FatalLevel: "fatal",
}

// Handler implementation.
type Handler struct {
	mu              sync.Mutex
	Writer          io.Writer
	TimestampFormat string
	colored         bool
}

func newColored(w io.Writer) *Handler {
	return &Handler{
		Writer:          w,
		TimestampFormat: "2006-01-02 15:04:05",
		colored:         true,
	}
}

func newPlain(w io.Writer) *Handler {
	return &Handler{
		Writer:          w,
		TimestampFormat: "2006-01-02 15:04:05",
		colored:         false,
	}
}

// HandleLog implements log.Handler.
func (h *Handler) HandleLog(e *log.Entry) error {
	color := colors[e.Level]
	level := strings[e.Level]
	names := e.Fields.Names()

	h.mu.Lock()
	defer h.mu.Unlock()
	ts := time.Now().Format(h.TimestampFormat)

	if h.colored {
		fmt.Fprintf(h.Writer, "%s [\033[%dm%6s\033[0m] %s", ts, color, level, e.Message)

		for _, name := range names {
			fmt.Fprintf(h.Writer, " \033[%dm%s\033[0m=%v", color, name, e.Fields.Get(name))
		}
	} else {
		fmt.Fprintf(h.Writer, "%s [%6s] %s", ts, level, e.Message)

		for _, name := range names {
			fmt.Fprintf(h.Writer, " %s=%v", name, e.Fields.Get(name))
		}
	}

	fmt.Fprintln(h.Writer)

	return nil
}
