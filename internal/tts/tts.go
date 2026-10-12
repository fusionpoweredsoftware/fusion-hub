// Package tts speaks text aloud. It uses OpenAI's speech API when a key is
// available (caching each clip as an mp3) and the system voice otherwise.
package tts

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Speaker says text. Sync waits for playback to finish.
type Speaker interface {
	Say(ctx context.Context, text string, male, sync bool) error
}

// Settings is the subset of the settings store tts reads.
type Settings interface{ String(key string) string }

// New picks the speaker: "tts" setting of "openai" or "system", or
// automatic (OpenAI when a key is available).
func New(s Settings, cacheDir string) Speaker {
	key := s.String("openai-api-key")
	if key == "" {
		key = os.Getenv("OPENAI_API_KEY")
	}
	mode := strings.ToLower(s.String("tts"))
	if mode == "openai" || (mode == "" && key != "") {
		return &OpenAI{Key: key, CacheDir: cacheDir, Model: "gpt-4o-mini-tts"}
	}
	return System{}
}

// OpenAI uses the /v1/audio/speech endpoint.
type OpenAI struct {
	Key      string
	CacheDir string
	Model    string
	BaseURL  string
}

func (o *OpenAI) Say(ctx context.Context, text string, male, sync bool) error {
	voice := "nova"
	if male {
		voice = "onyx"
	}
	sum := md5.Sum([]byte(text))
	file := filepath.Join(o.CacheDir, fmt.Sprintf("output-%s-%s.mp3", hex.EncodeToString(sum[:]), voice))
	if st, err := os.Stat(file); err != nil || st.Size() < 1024 {
		if err := o.synthesize(ctx, text, voice, file); err != nil {
			return err
		}
	}
	return play(ctx, file, sync)
}

func (o *OpenAI) synthesize(ctx context.Context, text, voice, file string) error {
	if o.Key == "" {
		return errors.New("text-to-speech needs an OpenAI key: set openai-api-key")
	}
	base := o.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	body, _ := json.Marshal(map[string]string{"model": o.Model, "voice": voice, "input": text, "response_format": "mp3"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/audio/speech", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.Key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("text-to-speech: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, audio, 0o600)
}

// players are tried in order to play an mp3.
var players = [][]string{
	{"afplay"},
	{"mpg123", "-q"},
	{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"},
	{"mpv", "--no-video", "--really-quiet"},
}

func play(ctx context.Context, file string, sync bool) error {
	for _, p := range players {
		if _, err := exec.LookPath(p[0]); err != nil {
			continue
		}
		return run(ctx, exec.Command(p[0], append(p[1:], file)...), sync)
	}
	return fmt.Errorf("no audio player found (install mpg123, ffplay or mpv); audio saved to %s", file)
}

// System uses the operating system's voice (say on macOS, espeak elsewhere).
type System struct{}

func (System) Say(ctx context.Context, text string, male, sync bool) error {
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "darwin":
		voice := "Samantha"
		if male {
			voice = "Alex"
		}
		cmd = exec.Command("say", "-v", voice, text)
	default:
		bin := ""
		for _, b := range []string{"espeak-ng", "espeak"} {
			if _, err := exec.LookPath(b); err == nil {
				bin = b
				break
			}
		}
		if bin == "" {
			return errors.New("no text-to-speech available: set openai-api-key or install espeak")
		}
		voice := "en+f3"
		if male {
			voice = "en+m3"
		}
		cmd = exec.Command(bin, "-v", voice, text)
	}
	return run(ctx, cmd, sync)
}

func run(ctx context.Context, cmd *exec.Cmd, sync bool) error {
	if !sync {
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	}
}

// Recorder is a Speaker that only records what it was asked to say.
type Recorder struct{ Said []string }

func (r *Recorder) Say(_ context.Context, text string, male, sync bool) error {
	prefix := ""
	if male {
		prefix = "(male) "
	}
	r.Said = append(r.Said, prefix+text)
	return nil
}
