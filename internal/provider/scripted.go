package provider

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// Scripted is an offline provider that replays canned replies in order.
// Tests use it, and it is handy for dry-running AIL pipelines.
type Scripted struct {
	mu       sync.Mutex
	Replies  []string
	Requests []Request
	// Reply, when set, computes the reply instead of Replies.
	Reply func(Request) string
}

func (s *Scripted) Name() string { return "scripted" }

func (s *Scripted) Stream(ctx context.Context, req Request, onDelta func(string)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	cp := req
	cp.Messages = append(cp.Messages[:0:0], req.Messages...)
	s.Requests = append(s.Requests, cp)
	var reply string
	switch {
	case s.Reply != nil:
		reply = s.Reply(req)
	case len(s.Replies) > 0:
		reply = s.Replies[0]
		s.Replies = s.Replies[1:]
	default:
		s.mu.Unlock()
		return "", errors.New("scripted provider: no replies left")
	}
	s.mu.Unlock()
	if onDelta != nil {
		for _, part := range strings.SplitAfter(reply, " ") {
			onDelta(part)
		}
	}
	return reply, nil
}

// Last returns the last user message of the most recent request.
func (s *Scripted) Last() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Requests) == 0 {
		return ""
	}
	m := s.Requests[len(s.Requests)-1].Messages
	if len(m) == 0 {
		return ""
	}
	return m[len(m)-1].Content
}
