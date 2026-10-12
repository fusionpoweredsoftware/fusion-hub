package uidmd

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	in := "Sure!\n\n*** plaintext (abc123:response)\nHello there.\nTwo lines.\n(abc123:response)\n\n***json (abc123:data.1)\n{\"a\":1}\n(abc123:data.1)\n*** plaintext (abc123:open)\nnever closed"
	got := Parse(in)
	if len(got) != 2 {
		t.Fatalf("got %d blocks: %#v", len(got), got)
	}
	if r := got["response"]; r.Value != "Hello there.\nTwo lines." || r.Format != "plaintext" || r.UID != "abc123" {
		t.Errorf("response = %#v", r)
	}
	if d := got["data.1"]; d.Value != `{"a":1}` || d.Format != "json" {
		t.Errorf("data.1 = %#v", d)
	}
	if v := Values(in); v["*"] != in || v["response"] != "Hello there.\nTwo lines." {
		t.Errorf("Values = %#v", v)
	}
}

func TestFormatRoundTrip(t *testing.T) {
	uid := NewUID(time.Date(2024, 1, 28, 6, 11, 28, 170e6, time.UTC))
	if uid != "20240128T061128170Z" {
		t.Fatalf("uid = %s", uid)
	}
	b := Format("plaintext", uid, "whales", "Baby orcas are calves.")
	if got := Parse(b)["whales"].Value; got != "Baby orcas are calves." {
		t.Errorf("round trip = %q", got)
	}
}
