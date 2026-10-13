package output

import (
	"bytes"
	"regexp"
	"testing"
	"time"
)

var stamped = regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}\] - `)

// Log lines go to the output stream and error/warn lines to the error
// stream, all as "[timestamp] - text" with no other prefix; Print is the
// text plus a newline.
func TestStreamsAndPrefixes(t *testing.T) {
	var out, errw bytes.Buffer
	o := NewPair(&out, &errw)
	o.now = func() time.Time { return time.Date(2024, 5, 1, 10, 2, 3, 4_000_000, time.UTC) }

	o.Log("Resuming %s...", "chat")
	o.Print("hello")
	o.Error("Command not found.")
	o.Warn("WARNING: slow")

	if got, want := out.String(), "[2024-05-01 10:02:03.004] - Resuming chat...\nhello\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := errw.String(), "[2024-05-01 10:02:03.004] - Command not found.\n[2024-05-01 10:02:03.004] - WARNING: slow\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if Timestamp(o.now()) != "2024-05-01 10:02:03.004" {
		t.Error("Timestamp")
	}
}

// New(w) keeps both streams on w so a captured buffer sees everything.
func TestNewSingleStream(t *testing.T) {
	var buf bytes.Buffer
	o := New(&buf)
	o.Error("boom")
	o.Log("ok")
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 || !stamped.Match(lines[0]) || !bytes.HasSuffix(lines[0], []byte("] - boom")) || !bytes.HasSuffix(lines[1], []byte("] - ok")) {
		t.Errorf("got %q", buf.String())
	}
}

func TestColorTable(t *testing.T) {
	want := map[string]string{
		"reset": "\x1b[0m", "bright": "\x1b[1m", "dim": "\x1b[2m", "underscore": "\x1b[4m",
		"blink": "\x1b[5m", "reverse": "\x1b[7m", "hidden": "\x1b[8m",
		"fgBlack": "\x1b[30m", "fgRed": "\x1b[31m", "fgGreen": "\x1b[32m", "fgYellow": "\x1b[33m",
		"fgBlue": "\x1b[34m", "fgMagenta": "\x1b[35m", "fgCyan": "\x1b[36m", "fgWhite": "\x1b[37m", "fgGray": "\x1b[90m",
		"bgBlack": "\x1b[40m", "bgRed": "\x1b[41m", "bgGreen": "\x1b[42m", "bgYellow": "\x1b[43m",
		"bgBlue": "\x1b[44m", "bgMagenta": "\x1b[45m", "bgCyan": "\x1b[46m", "bgWhite": "\x1b[47m", "bgGray": "\x1b[100m",
	}
	if len(Colors) != len(want) {
		t.Fatalf("Colors has %d entries, want %d", len(Colors), len(want))
	}
	for k, v := range want {
		if Colors[k] != v {
			t.Errorf("Colors[%q] = %q, want %q", k, Colors[k], v)
		}
	}
	if Gray != FgGray || Blue != FgBlue || Red != FgRed {
		t.Error("short names")
	}
}
