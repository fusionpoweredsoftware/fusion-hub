// Package api serves fusion-hub over HTTP. POST /chat-with-ai[/{sessionId}]
// with {"message": "..."} streams the reply as newline-delimited JSON:
//
//	{"request": "...", "responseId": "...", "sessionId": "..."}
//	{"message": "<chunk>", "responseId": "...", "sessionId": "..."}   (repeated)
//	{"state": "eom", "responseId": "...", "sessionId": "..."}
//
// At most Limit requests run at once; each slot is held for Cooldown after
// its request ends. Requests beyond that get HTTP 429 and an error line.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config is the server configuration.
type Config struct {
	Name     string
	Port     int
	Limit    int
	Cooldown time.Duration
	Timeout  time.Duration
}

// Settings is the subset of the settings store the API reads.
type Settings interface{ String(key string) string }

// ConfigFrom reads FUSION_HUB_INSTANCE_NAME / FUSION_HUB_INSTANCE_PORT and
// the api-request-limit, api-cooldown-ms and api-timeout-ms settings.
func ConfigFrom(s Settings) Config {
	c := Config{Name: "Fusion Hub", Port: 3000, Limit: 2, Cooldown: 2 * time.Second, Timeout: 2 * time.Minute}
	if v := os.Getenv("FUSION_HUB_INSTANCE_NAME"); v != "" {
		c.Name = v
	}
	if v, err := strconv.Atoi(os.Getenv("FUSION_HUB_INSTANCE_PORT")); err == nil {
		c.Port = v
	}
	if v, err := strconv.Atoi(s.String("api-request-limit")); err == nil && v > 0 {
		c.Limit = v
	}
	if v, err := strconv.Atoi(s.String("api-cooldown-ms")); err == nil && v >= 0 {
		c.Cooldown = time.Duration(v) * time.Millisecond
	}
	if v, err := strconv.Atoi(s.String("api-timeout-ms")); err == nil && v > 0 {
		c.Timeout = time.Duration(v) * time.Millisecond
	}
	return c
}

// ChatFunc sends message in sessionID's conversation, writing the streamed
// reply to out.
type ChatFunc func(ctx context.Context, sessionID, message string, out io.Writer) error

// Server is the HTTP API.
type Server struct {
	cfg   Config
	chat  ChatFunc
	slots chan struct{}
	// NewID makes session and response ids.
	NewID func() string
}

func New(cfg Config, chat ChatFunc) *Server {
	if cfg.Limit < 1 {
		cfg.Limit = 1
	}
	return &Server{cfg: cfg, chat: chat, slots: make(chan struct{}, cfg.Limit), NewID: newID}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return strconv.FormatInt(time.Now().UnixMilli(), 16) + "-" + hex.EncodeToString(b)
}

// Handler returns the routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /chat-with-ai", s.handleChat)
	mux.HandleFunc("POST /chat-with-ai/{sessionId}", s.handleChat)
	return mux
}

// ListenAndServe serves until ctx is done.
func (s *Server) ListenAndServe(ctx context.Context, onListening func(name string, port int)) error {
	srv := &http.Server{Addr: fmt.Sprintf(":%d", s.cfg.Port), Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	if onListening != nil {
		onListening(s.cfg.Name, s.cfg.Port)
	}
	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// lineWriter writes NDJSON lines and flushes each one.
type lineWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
}

func (l *lineWriter) line(v any) {
	b, _ := json.Marshal(v)
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(append(b, '\n'))
	if f, ok := l.w.(http.Flusher); ok {
		f.Flush()
	}
}

// chunkWriter turns streamed text into message lines.
type chunkWriter struct {
	lw                    *lineWriter
	sessionID, responseID string
}

func (c *chunkWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		c.lw.line(map[string]any{"message": string(p), "responseId": c.responseID, "sessionId": c.sessionID})
	}
	return len(p), nil
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		sessionID = s.NewID()
	}
	responseID := s.NewID()
	w.Header().Set("Content-Type", "application/x-ndjson")
	lw := &lineWriter{w: w}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		w.WriteHeader(http.StatusBadRequest)
		lw.line(map[string]any{"error": http.StatusBadRequest, "message": `Send JSON like {"message": "Hello"}.`, "responseId": responseID, "sessionId": sessionID})
		return
	}

	select {
	case s.slots <- struct{}{}:
	default:
		w.WriteHeader(http.StatusTooManyRequests)
		lw.line(map[string]any{"error": http.StatusTooManyRequests, "message": "Too many requests.", "responseId": responseID, "sessionId": sessionID})
		return
	}
	defer func() {
		if s.cfg.Cooldown > 0 {
			time.AfterFunc(s.cfg.Cooldown, func() { <-s.slots })
		} else {
			<-s.slots
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout)
	defer cancel()
	lw.line(map[string]any{"request": body.Message, "responseId": responseID, "sessionId": sessionID})
	if err := s.chat(ctx, sessionID, body.Message, &chunkWriter{lw: lw, sessionID: sessionID, responseID: responseID}); err != nil {
		lw.line(map[string]any{"error": 500, "message": err.Error(), "responseId": responseID, "sessionId": sessionID})
	}
	lw.line(map[string]any{"state": "eom", "responseId": responseID, "sessionId": sessionID})
}
