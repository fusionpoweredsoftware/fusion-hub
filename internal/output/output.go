// Package output holds the console helpers shared by every command:
// timestamped log lines and the ANSI colours used for speaker prompts.
//
// The format is the original one: log lines are "[YYYY-MM-DD HH:mm:ss.SSS] - text"
// on stdout, error and warn lines carry the same stamp on stderr with no
// prefix of their own, and Print writes the text plus a newline.
package output

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// The ANSI colour table (colors.mjs).
const (
	Reset      = "\x1b[0m"
	Bright     = "\x1b[1m"
	Dim        = "\x1b[2m"
	Underscore = "\x1b[4m"
	Blink      = "\x1b[5m"
	Reverse    = "\x1b[7m"
	Hidden     = "\x1b[8m"

	FgBlack   = "\x1b[30m"
	FgRed     = "\x1b[31m"
	FgGreen   = "\x1b[32m"
	FgYellow  = "\x1b[33m"
	FgBlue    = "\x1b[34m"
	FgMagenta = "\x1b[35m"
	FgCyan    = "\x1b[36m"
	FgWhite   = "\x1b[37m"
	FgGray    = "\x1b[90m"

	BgBlack   = "\x1b[40m"
	BgRed     = "\x1b[41m"
	BgGreen   = "\x1b[42m"
	BgYellow  = "\x1b[43m"
	BgBlue    = "\x1b[44m"
	BgMagenta = "\x1b[45m"
	BgCyan    = "\x1b[46m"
	BgWhite   = "\x1b[47m"
	BgGray    = "\x1b[100m"
)

// Short names for the foreground colours.
const (
	Red    = FgRed
	Green  = FgGreen
	Yellow = FgYellow
	Blue   = FgBlue
	Cyan   = FgCyan
	Gray   = FgGray
)

// Colors is the colour table under its original names, for scripts that
// receive it as an object (cliColors).
var Colors = map[string]string{
	"reset":      Reset,
	"bright":     Bright,
	"dim":        Dim,
	"underscore": Underscore,
	"blink":      Blink,
	"reverse":    Reverse,
	"hidden":     Hidden,

	"fgBlack":   FgBlack,
	"fgRed":     FgRed,
	"fgGreen":   FgGreen,
	"fgYellow":  FgYellow,
	"fgBlue":    FgBlue,
	"fgMagenta": FgMagenta,
	"fgCyan":    FgCyan,
	"fgWhite":   FgWhite,
	"fgGray":    FgGray,

	"bgBlack":   BgBlack,
	"bgRed":     BgRed,
	"bgGreen":   BgGreen,
	"bgYellow":  BgYellow,
	"bgBlue":    BgBlue,
	"bgMagenta": BgMagenta,
	"bgCyan":    BgCyan,
	"bgWhite":   BgWhite,
	"bgGray":    BgGray,
}

// Timestamp formats t the way fusion-hub has always stamped log lines.
func Timestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000")
}

// Writer is a mutex-guarded pair of streams with log helpers. Commands write
// their payload with Print/Write, status lines with Log, and problems with
// Error/Warn, which go to the error stream.
type Writer struct {
	mu  sync.Mutex
	w   io.Writer
	err io.Writer
	now func() time.Time
}

// New returns a Writer whose output and error streams are both w, except
// that os.Stdout is paired with os.Stderr (the console).
func New(w io.Writer) *Writer {
	if w == os.Stdout {
		return NewPair(os.Stdout, os.Stderr)
	}
	return NewPair(w, w)
}

// NewPair returns a Writer with separate output and error streams.
func NewPair(out, errw io.Writer) *Writer {
	return &Writer{w: out, err: errw, now: time.Now}
}

func (o *Writer) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.w.Write(p)
}

// Print writes s followed by a newline.
func (o *Writer) Print(s string) { _, _ = io.WriteString(o, s+"\n") }

func (o *Writer) Printf(format string, a ...any) { o.Print(fmt.Sprintf(format, a...)) }

// Stamp returns the "[timestamp] - " prefix of a log line written now.
func (o *Writer) Stamp() string { return "[" + Timestamp(o.now()) + "] - " }

func (o *Writer) event(w io.Writer, msg string) {
	line := o.Stamp() + msg + "\n"
	o.mu.Lock()
	defer o.mu.Unlock()
	_, _ = io.WriteString(w, line)
}

// Log writes a timestamped line to the output stream.
func (o *Writer) Log(format string, a ...any) { o.event(o.w, fmt.Sprintf(format, a...)) }

// Warn writes a timestamped line to the error stream.
func (o *Writer) Warn(format string, a ...any) { o.event(o.err, fmt.Sprintf(format, a...)) }

// Error writes a timestamped line to the error stream.
func (o *Writer) Error(format string, a ...any) { o.event(o.err, fmt.Sprintf(format, a...)) }

// Discard is a Writer that drops everything.
func Discard() *Writer { return New(io.Discard) }
