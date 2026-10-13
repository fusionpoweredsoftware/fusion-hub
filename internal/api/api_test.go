package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeChat records calls; Reply is streamed in two chunks by Send.
type fakeChat struct {
	mu       sync.Mutex
	sends    []string
	news     []string
	sendErr  error
	reply    string
	onSend   func(ctx context.Context)
	lastSess string
}

func (f *fakeChat) Send(ctx context.Context, sessionID, message string, out io.Writer) error {
	f.mu.Lock()
	f.sends = append(f.sends, message)
	f.lastSess = sessionID
	f.mu.Unlock()
	if f.onSend != nil {
		f.onSend(ctx)
	}
	if f.sendErr != nil {
		return f.sendErr
	}
	reply := f.reply
	if reply == "" {
		reply = "Chess is a game."
	}
	half := len(reply) / 2
	io.WriteString(out, reply[:half])
	io.WriteString(out, reply[half:])
	io.WriteString(out, "\n\n")
	return nil
}

func (f *fakeChat) New(ctx context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.news = append(f.news, sessionID)
	f.lastSess = sessionID
	return nil
}

func (f *fakeChat) calls() (sends, news []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.sends...), append([]string{}, f.news...)
}

func (f *fakeChat) session() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastSess
}

func newServer(t *testing.T, cfg Config, chat Chat) (*Server, *httptest.Server) {
	t.Helper()
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	s := New(cfg, chat)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// body reads the response body and returns it raw and split into its JSON
// lines (each parsed into a map).
func body(t *testing.T, resp *http.Response) (string, []map[string]any) {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	raw := string(b)
	var out []map[string]any
	for _, l := range strings.Split(raw, "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("bad line %q in %q: %v", l, raw, err)
		}
		out = append(out, m)
	}
	return raw, out
}

func TestFramingAndKeyOrder(t *testing.T) {
	chat := &fakeChat{}
	s, srv := newServer(t, Config{Limit: 2}, chat)
	ids := []string{"r1"}
	s.NewID = func() string { id := ids[0]; ids = ids[1:]; return id }

	resp := post(t, srv.URL+"/chat-with-ai/abc", `{"message": "tell me about <chess>"}`)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q", ct)
	}
	if te := resp.TransferEncoding; len(te) != 1 || te[0] != "chunked" {
		t.Errorf("Transfer-Encoding %v", te)
	}
	raw, _ := body(t, resp)
	want := "\n" + `{"request":"tell me about <chess>","responseId":"r1","sessionId":"abc"}` +
		"\n" + `{"message":"Chess is","responseId":"r1","sessionId":"abc"}` +
		"\n" + `{"message":" a game.","responseId":"r1","sessionId":"abc"}` +
		"\n" + `{"message":"\n\n","responseId":"r1","sessionId":"abc"}` +
		"\n" + `{"state":"eom","responseId":"r1","sessionId":"abc"}` + "\n"
	if raw != want {
		t.Errorf("body\n%q\nwant\n%q", raw, want)
	}
	if sends, _ := chat.calls(); len(sends) != 1 || sends[0] != "tell me about <chess>" || chat.session() != "abc" {
		t.Errorf("sends %v session %q", sends, chat.session())
	}
}

func TestGeneratedSessionID(t *testing.T) {
	chat := &fakeChat{}
	_, srv := newServer(t, Config{Limit: 2}, chat)
	resp := post(t, srv.URL+"/chat-with-ai", `{"message": "hi"}`)
	_, ls := body(t, resp)
	id, _ := ls[0]["sessionId"].(string)
	if id == "" || !strings.Contains(id, "-") || ls[0]["responseId"] == id {
		t.Errorf("generated ids %v", ls[0])
	}
	for _, l := range ls[1:] {
		if l["sessionId"] != id || l["responseId"] != ls[0]["responseId"] {
			t.Errorf("ids differ across lines: %v", ls)
		}
	}
}

func TestMessageIsTrimmed(t *testing.T) {
	chat := &fakeChat{}
	_, srv := newServer(t, Config{Limit: 2}, chat)
	resp := post(t, srv.URL+"/chat-with-ai/s", `{"message": "  hello \n"}`)
	_, ls := body(t, resp)
	if ls[0]["request"] != "  hello \n" {
		t.Errorf("request line %v", ls[0])
	}
	if sends, _ := chat.calls(); len(sends) != 1 || sends[0] != "hello" {
		t.Errorf("sends %q", sends)
	}
}

func TestEmptyMessageStartsNewChat(t *testing.T) {
	for _, in := range []string{`{"message": ""}`, `{"message": "   "}`, `{}`, `{"message": null}`, ``} {
		chat := &fakeChat{}
		s, srv := newServer(t, Config{Limit: 2}, chat)
		s.NewID = func() string { return "r" }
		resp := post(t, srv.URL+"/chat-with-ai/sess", in)
		if resp.StatusCode != 200 {
			t.Fatalf("%q: status %d", in, resp.StatusCode)
		}
		raw, _ := body(t, resp)
		want := "\n" + `{"responseId":"r","sessionId":"sess"}` + "\n" + `{"state":"eom","responseId":"r","sessionId":"sess"}` + "\n"
		if raw != want {
			t.Errorf("%q: body %q want %q", in, raw, want)
		}
		if sends, news := chat.calls(); len(sends) != 0 || len(news) != 1 || news[0] != "sess" {
			t.Errorf("%q: sends %v news %v", in, sends, news)
		}
	}
}

func TestBodyParsing(t *testing.T) {
	chat := &fakeChat{}
	_, srv := newServer(t, Config{Limit: 2}, chat)

	// Any Content-Type is parsed as JSON.
	resp, err := http.Post(srv.URL+"/chat-with-ai/s", "text/plain", strings.NewReader(`{"message": "hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	_, ls := body(t, resp)
	if resp.StatusCode != 200 || ls[0]["request"] != "hi" {
		t.Errorf("text/plain: %d %v", resp.StatusCode, ls)
	}

	// A number is sent as its text, like the legacy string interpolation.
	resp = post(t, srv.URL+"/chat-with-ai/s", `{"message": 42}`)
	body(t, resp)
	if sends, _ := chat.calls(); sends[len(sends)-1] != "42" {
		t.Errorf("number: sends %v", sends)
	}

	for _, bad := range []string{`nope`, `{"message": "hi"`, `[1]`, `{"message": {"a": 1}}`} {
		resp = post(t, srv.URL+"/chat-with-ai/s", bad)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%q: status %d", bad, resp.StatusCode)
		}
	}
	resp = post(t, srv.URL+"/chat-with-ai/s", `{"message": "`+strings.Repeat("x", maxBodyBytes)+`"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("huge body: status %d", resp.StatusCode)
	}
	if sends, _ := chat.calls(); len(sends) != 2 {
		t.Errorf("bad bodies reached the chat: %v", sends)
	}
}

func TestRoutes(t *testing.T) {
	chat := &fakeChat{}
	_, srv := newServer(t, Config{Limit: 2}, chat)
	for _, c := range []struct {
		method, path string
		status       int
		session      string
	}{
		{"POST", "/chat-with-ai", 200, ""},
		{"POST", "/chat-with-ai/", 200, ""},
		{"POST", "/Chat-With-AI", 200, ""},
		{"POST", "/CHAT-WITH-AI/MySession/", 200, "MySession"},
		{"POST", "/chat-with-ai/a%20b", 200, "a b"},
		{"GET", "/chat-with-ai", 404, ""},
		{"PUT", "/chat-with-ai/x", 404, ""},
		{"POST", "/chat-with-ai/a/b", 404, ""},
		{"POST", "/chat-with-ai//", 404, ""},
		{"POST", "/chat-with-ai-2", 404, ""},
		{"POST", "/", 404, ""},
		{"POST", "/other", 404, ""},
	} {
		req, _ := http.NewRequest(c.method, srv.URL+c.path, strings.NewReader(`{"message": "hi"}`))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != c.status {
			resp.Body.Close()
			t.Errorf("%s %s: status %d want %d", c.method, c.path, resp.StatusCode, c.status)
			continue
		}
		if c.status != 200 {
			resp.Body.Close()
			continue
		}
		_, ls := body(t, resp)
		got, _ := ls[0]["sessionId"].(string)
		if c.session == "" && got == "" || c.session != "" && got != c.session {
			t.Errorf("%s %s: session %q want %q", c.method, c.path, got, c.session)
		}
	}
}

func TestLimitAndCooldown(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	chat := &fakeChat{onSend: func(context.Context) { started <- struct{}{}; <-release }}
	s, srv := newServer(t, Config{Limit: 2, Cooldown: 200 * time.Millisecond}, chat)
	s.NewID = func() string { return "r" }

	var wg sync.WaitGroup
	results := make(chan *http.Response, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- post(t, srv.URL+"/chat-with-ai/s", fmt.Sprintf(`{"message": "%d"}`, i))
		}()
	}
	<-started
	<-started

	// Exactly Limit requests run; the next one is rejected.
	resp := post(t, srv.URL+"/chat-with-ai/sess", `{"message": "three"}`)
	raw, _ := body(t, resp)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q", ct)
	}
	if want := "\n" + `{"error":429,"message":"Too many requests.","sessionId":"sess","responseId":"r"}` + "\n"; raw != want {
		t.Errorf("body %q want %q", raw, want)
	}
	// An empty message takes a slot too.
	resp = post(t, srv.URL+"/chat-with-ai/sess", `{}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("empty message during limit: %d", resp.StatusCode)
	}

	close(release)
	wg.Wait()
	for i := 0; i < 2; i++ {
		r := <-results
		if r.StatusCode != 200 {
			t.Errorf("running request status %d", r.StatusCode)
		}
		r.Body.Close()
	}

	// Slots stay held for the cooldown, then free up.
	resp = post(t, srv.URL+"/chat-with-ai/s", `{"message": "four"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("during cooldown: %d", resp.StatusCode)
	}
	time.Sleep(300 * time.Millisecond)
	resp = post(t, srv.URL+"/chat-with-ai/s", `{"message": "five"}`)
	body(t, resp)
	if resp.StatusCode != 200 {
		t.Errorf("after cooldown: %d", resp.StatusCode)
	}
	if sends, news := chat.calls(); len(sends) != 3 || len(news) != 0 {
		t.Errorf("rejected requests reached the chat: %v %v", sends, news)
	}
}

func TestTimeoutFreesSlotWithoutCancelling(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var cancelled atomic.Bool
	chat := &fakeChat{onSend: func(ctx context.Context) {
		started <- struct{}{}
		<-release
		cancelled.Store(ctx.Err() != nil)
	}}
	_, srv := newServer(t, Config{Limit: 1, Timeout: 50 * time.Millisecond, Cooldown: 0}, chat)

	first := make(chan *http.Response, 1)
	go func() { first <- post(t, srv.URL+"/chat-with-ai/s", `{"message": "slow"}`) }()
	<-started
	time.Sleep(100 * time.Millisecond)

	// The timeout freed the slot while the first chat is still running.
	resp := post(t, srv.URL+"/chat-with-ai/s", `{"message": "two"}`)
	if resp.StatusCode != 200 {
		t.Errorf("after timeout: %d", resp.StatusCode)
	}
	close(release)
	body(t, resp)
	r := <-first
	_, ls := body(t, r)
	if r.StatusCode != 200 || ls[len(ls)-1]["state"] != "eom" || cancelled.Load() {
		t.Errorf("timed-out request: %d %v cancelled=%v", r.StatusCode, ls, cancelled.Load())
	}
	if sends, _ := chat.calls(); len(sends) != 2 {
		t.Errorf("sends %v", sends)
	}
}

func TestChatErrorIsLoggedNotStreamed(t *testing.T) {
	chat := &fakeChat{sendErr: errors.New("no API key")}
	s, srv := newServer(t, Config{Limit: 1, Cooldown: 0}, chat)
	s.NewID = func() string { return "r" }
	var logged []string
	s.Errorf = func(format string, a ...any) { logged = append(logged, fmt.Sprintf(format, a...)) }

	resp := post(t, srv.URL+"/chat-with-ai/s", `{"message": "hi"}`)
	raw, _ := body(t, resp)
	want := "\n" + `{"request":"hi","responseId":"r","sessionId":"s"}` + "\n" + `{"state":"eom","responseId":"r","sessionId":"s"}` + "\n"
	if resp.StatusCode != 200 || raw != want {
		t.Errorf("status %d body %q", resp.StatusCode, raw)
	}
	if len(logged) != 1 || logged[0] != "no API key" {
		t.Errorf("logged %q", logged)
	}
	// A failed request frees its slot too.
	resp = post(t, srv.URL+"/chat-with-ai/s", `{"message": "hi"}`)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("slot leaked: %d", resp.StatusCode)
	}
}

type fakeSettings map[string]string

func (f fakeSettings) String(k string) string { return f[k] }

func TestConfigFrom(t *testing.T) {
	t.Setenv("FUSION_HUB_INSTANCE_NAME", "Hub")
	t.Setenv("FUSION_HUB_INSTANCE_PORT", "8080")
	c := ConfigFrom(fakeSettings{"api-request-limit": "5", "api-cooldown-ms": "0"})
	if c.Name != "Hub" || c.Port != 8080 || c.Limit != 5 || c.Cooldown != 0 || c.Timeout != 2*time.Minute {
		t.Errorf("config %#v", c)
	}
	t.Setenv("FUSION_HUB_INSTANCE_NAME", "")
	t.Setenv("FUSION_HUB_INSTANCE_PORT", "")
	c = ConfigFrom(fakeSettings{})
	if c.Name != "Fusion Hub" || c.Port != 3000 || c.Limit != 2 || c.Cooldown != 2*time.Second || c.Timeout != 2*time.Minute {
		t.Errorf("defaults %#v", c)
	}
}
