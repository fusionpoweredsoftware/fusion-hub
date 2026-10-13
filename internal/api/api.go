// Package api serves fusion-hub over HTTP. POST /chat-with-ai[/{sessionId}]
// with {"message": "..."} streams the reply as JSON objects, each written
// after a newline, with a final newline closing the body:
//
//	{"request": "...", "responseId": "...", "sessionId": "..."}
//	{"message": "<chunk>", "responseId": "...", "sessionId": "..."}   (repeated)
//	{"state": "eom", "responseId": "...", "sessionId": "..."}
//
// An empty or missing message starts a new chat for the session instead
// (the request line then has no "request" key). At most Limit requests run
// at once; each slot is held for Cooldown after its response ends and is
// freed after Timeout even while its chat is still running. Requests beyond
// the limit get HTTP 429 with a single
// {"error":429,"message":"Too many requests.","sessionId":...,"responseId":...}
// line.
package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// Chat is what the API drives: the chat sessions.
type Chat interface {
	// Send sends message in sessionID's conversation, writing the streamed
	// reply to out.
	Send(ctx context.Context, sessionID, message string, out io.Writer) error
	// New archives sessionID's conversation and starts a new one.
	New(ctx context.Context, sessionID string) error
}

// maxBodyBytes caps a request body (the legacy body parser's default).
const maxBodyBytes = 100 << 10

// Server is the HTTP API.
type Server struct {
	cfg   Config
	chat  Chat
	slots chan struct{}
	// NewID makes session and response ids.
	NewID func() string
	// Errorf logs a failed chat to the server console.
	Errorf func(format string, a ...any)
}

func New(cfg Config, chat Chat) *Server {
	if cfg.Limit < 1 {
		cfg.Limit = 1
	}
	return &Server{
		cfg:    cfg,
		chat:   chat,
		slots:  make(chan struct{}, cfg.Limit),
		NewID:  newID,
		Errorf: func(string, ...any) {},
	}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return strconv.FormatInt(time.Now().UnixMilli(), 16) + "-" + hex.EncodeToString(b)
}

// Handler returns the routes.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.route) }

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

const routePath = "/chat-with-ai"

// route matches POST /chat-with-ai[/{sessionId}] case-insensitively, with
// or without a trailing slash. Anything else, including other methods, is
// a 404.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSuffix(r.URL.Path, "/")
	rest, ok := strings.CutPrefix(strings.ToLower(p), routePath)
	sessionID := ""
	if ok && rest != "" {
		if rest[0] == '/' && len(rest) > 1 && !strings.Contains(rest[1:], "/") {
			sessionID = p[len(routePath)+1:]
		} else {
			ok = false
		}
	}
	if !ok || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	s.handleChat(w, r, sessionID)
}

// The response lines, with their keys in the legacy order.
type (
	requestLine struct {
		Request    string `json:"request,omitempty"`
		ResponseID string `json:"responseId"`
		SessionID  string `json:"sessionId"`
	}
	messageLine struct {
		Message    string `json:"message"`
		ResponseID string `json:"responseId"`
		SessionID  string `json:"sessionId"`
	}
	endLine struct {
		State      string `json:"state"`
		ResponseID string `json:"responseId"`
		SessionID  string `json:"sessionId"`
	}
	limitLine struct {
		Error      int    `json:"error"`
		Message    string `json:"message"`
		SessionID  string `json:"sessionId"`
		ResponseID string `json:"responseId"`
	}
)

// lineWriter writes the stream: a newline, then the JSON object, flushed.
type lineWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
}

func (l *lineWriter) line(v any) {
	var buf bytes.Buffer
	buf.WriteByte('\n')
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	l.write(bytes.TrimSuffix(buf.Bytes(), []byte{'\n'}))
}

// end closes the body with the final newline.
func (l *lineWriter) end() { l.write([]byte{'\n'}) }

func (l *lineWriter) write(b []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.w.Write(b)
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
		c.lw.line(messageLine{Message: string(p), ResponseID: c.responseID, SessionID: c.sessionID})
	}
	return len(p), nil
}

// readMessage parses the body, whatever its Content-Type, and returns the
// message as text. A missing or null message is "". A non-string scalar is
// its JSON text; an object or array is an error.
func readMessage(w http.ResponseWriter, r *http.Request) (string, error) {
	var body struct {
		Message json.RawMessage `json:"message"`
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&body)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	raw := bytes.TrimSpace(body.Message)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	if raw[0] == '{' || raw[0] == '[' {
		return "", errors.New("message must be a string")
	}
	return string(raw), nil
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request, sessionID string) {
	message, err := readMessage(w, r)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if sessionID == "" {
		sessionID = s.NewID()
	}
	responseID := s.NewID()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")
	lw := &lineWriter{w: w}

	select {
	case s.slots <- struct{}{}:
	default:
		w.WriteHeader(http.StatusTooManyRequests)
		lw.line(limitLine{Error: http.StatusTooManyRequests, Message: "Too many requests.", SessionID: sessionID, ResponseID: responseID})
		lw.end()
		return
	}
	// The slot is freed once: after the timeout (the chat keeps running)
	// or Cooldown after the response, whichever comes first.
	release := sync.OnceFunc(func() { <-s.slots })
	var timeout *time.Timer
	if s.cfg.Timeout > 0 {
		timeout = time.AfterFunc(s.cfg.Timeout, release)
	}
	defer func() {
		if timeout != nil {
			timeout.Stop()
		}
		if s.cfg.Cooldown > 0 {
			time.AfterFunc(s.cfg.Cooldown, release)
		} else {
			release()
		}
	}()

	text := strings.TrimSpace(message)
	req := requestLine{ResponseID: responseID, SessionID: sessionID}
	if text != "" {
		req.Request = message
	}
	lw.line(req)
	// Neither a timeout nor the client going away stops the chat; the
	// conversation is completed and saved as if the request had been read.
	ctx := context.WithoutCancel(r.Context())
	if text != "" {
		err = s.chat.Send(ctx, sessionID, text, &chunkWriter{lw: lw, sessionID: sessionID, responseID: responseID})
	} else {
		err = s.chat.New(ctx, sessionID)
	}
	if err != nil {
		s.Errorf("%v", err)
	}
	lw.line(endLine{State: "eom", ResponseID: responseID, SessionID: sessionID})
	lw.end()
}
