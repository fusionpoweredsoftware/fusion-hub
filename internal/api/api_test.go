package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func lines(t *testing.T, r io.Reader) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestStreamsNDJSON(t *testing.T) {
	var gotSession string
	s := New(Config{Limit: 2, Timeout: time.Second}, func(_ context.Context, sessionID, msg string, out io.Writer) error {
		gotSession = sessionID
		io.WriteString(out, "Chess is ")
		io.WriteString(out, "a game.")
		return nil
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp := post(t, srv.URL+"/chat-with-ai/abc", `{"message": "tell me about chess"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("status %d type %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	ls := lines(t, resp.Body)
	if len(ls) != 4 || ls[0]["request"] != "tell me about chess" || ls[1]["message"] != "Chess is " || ls[3]["state"] != "eom" {
		t.Fatalf("lines %v", ls)
	}
	if gotSession != "abc" || ls[2]["sessionId"] != "abc" || ls[0]["responseId"] != ls[3]["responseId"] {
		t.Errorf("ids %v session %s", ls, gotSession)
	}

	resp2 := post(t, srv.URL+"/chat-with-ai", `{"message": "hi"}`)
	l2 := lines(t, resp2.Body)
	resp2.Body.Close()
	if l2[0]["sessionId"] == "" || l2[0]["sessionId"] == "abc" {
		t.Errorf("no generated session id: %v", l2[0])
	}
}

func TestTooManyRequests(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	s := New(Config{Limit: 1, Timeout: 5 * time.Second, Cooldown: 50 * time.Millisecond}, func(ctx context.Context, _, _ string, out io.Writer) error {
		started <- struct{}{}
		<-release
		io.WriteString(out, "done")
		return nil
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	first := make(chan *http.Response)
	go func() { first <- post(t, srv.URL+"/chat-with-ai", `{"message": "one"}`) }()
	<-started

	resp := post(t, srv.URL+"/chat-with-ai", `{"message": "two"}`)
	ls := lines(t, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests || len(ls) != 1 || ls[0]["error"] != float64(429) || ls[0]["message"] != "Too many requests." {
		t.Fatalf("status %d lines %v", resp.StatusCode, ls)
	}

	close(release)
	r1 := <-first
	if r1.StatusCode != 200 {
		t.Fatalf("first status %d", r1.StatusCode)
	}
	r1.Body.Close()

	// The slot is held for the cooldown, then freed.
	resp = post(t, srv.URL+"/chat-with-ai", `{"message": "three"}`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("during cooldown: %d", resp.StatusCode)
	}
	time.Sleep(100 * time.Millisecond)
	resp = post(t, srv.URL+"/chat-with-ai", `{"message": "four"}`)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("after cooldown: %d", resp.StatusCode)
	}
}

func TestBadRequestAndChatError(t *testing.T) {
	s := New(Config{Limit: 1, Timeout: time.Second}, func(context.Context, string, string, io.Writer) error {
		return errors.New("no API key")
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp := post(t, srv.URL+"/chat-with-ai", `nope`)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status %d", resp.StatusCode)
	}
	resp = post(t, srv.URL+"/chat-with-ai", `{"message": "hi"}`)
	ls := lines(t, resp.Body)
	resp.Body.Close()
	if ls[1]["message"] != "no API key" || ls[2]["state"] != "eom" {
		t.Errorf("lines %v", ls)
	}
	// A failed request frees its slot too.
	resp = post(t, srv.URL+"/chat-with-ai", `{"message": "hi"}`)
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
}
