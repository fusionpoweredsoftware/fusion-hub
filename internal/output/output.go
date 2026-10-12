// Package output holds the console helpers shared by every command:
// timestamped log lines and the ANSI colours used for speaker prompts.
package output

import (
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	Reset  = "\x1b[0m"
	Bright = "\x1b[1m"
	Dim    = "\x1b[2m"
	Red    = "\x1b[31m"
	Green  = "\x1b[32m"
	Yellow = "\x1b[33m"
	Blue   = "\x1b[34m"
	Cyan   = "\x1b[36m"
	Gray   = "\x1b[90m"
)

// Timestamp formats t the way fusion-hub has always stamped log lines.
func Timestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000")
}

// Writer is a mutex-guarded writer with log helpers. Commands write their
// payload with Print/Write and their status lines with Log/Error/Warn.
type Writer struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
}

func New(w io.Writer) *Writer { return &Writer{w: w, now: time.Now} }

func (o *Writer) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.w.Write(p)
}

// Print writes s followed by a newline.
func (o *Writer) Print(s string) { _, _ = io.WriteString(o, s+"\n") }

func (o *Writer) Printf(format string, a ...any) { o.Print(fmt.Sprintf(format, a...)) }

func (o *Writer) event(msg string) {
	o.Print("[" + Timestamp(o.now()) + "] - " + msg)
}

func (o *Writer) Log(format string, a ...any)   { o.event(fmt.Sprintf(format, a...)) }
func (o *Writer) Warn(format string, a ...any)  { o.event("WARNING: " + fmt.Sprintf(format, a...)) }
func (o *Writer) Error(format string, a ...any) { o.event("ERROR: " + fmt.Sprintf(format, a...)) }

// Discard is a Writer that drops everything.
func Discard() *Writer { return New(io.Discard) }
