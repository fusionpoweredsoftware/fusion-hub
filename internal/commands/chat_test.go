package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fusionpoweredsoftware/fusion-hub/internal/chat"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/config"
	"github.com/fusionpoweredsoftware/fusion-hub/internal/history"
)

// lines returns the texts of the timestamped log lines in out, in order,
// without any ERROR:/WARNING: prefix the output package may add.
func lines(out string) []string {
	var got []string
	for _, l := range strings.Split(out, "\n") {
		if _, text, ok := strings.Cut(l, "] - "); ok {
			text = strings.TrimPrefix(strings.TrimPrefix(text, "ERROR: "), "WARNING: ")
			got = append(got, text)
		}
	}
	return got
}

func TestChatWithAIStartsAndLogsOnce(t *testing.T) {
	h := newHarness(t, "ok", "Hello", "Again")
	out := h.run(t, "chat-with-ai hi")
	if got := lines(out); len(got) != 1 || got[0] != "Initializing chat with AI..." {
		t.Errorf("log lines %q", got)
	}
	if !strings.HasSuffix(out, "Hello\n\n") {
		t.Errorf("out %q", out)
	}
	if out := h.run(t, "chat-with-ai more"); out != "Again\n\n" {
		t.Errorf("second message out %q", out)
	}
	if h.sp.Requests[0].Messages[0].Content != chat.DefaultInstructions {
		t.Errorf("instructions not sent first: %#v", h.sp.Requests[0].Messages)
	}
	// Legacy file format and location.
	b, err := os.ReadFile(filepath.Join(h.env.Paths.Root, config.Histories, ".ai-chat-history.json"))
	if err != nil || !strings.HasPrefix(string(b), `{"0":[{"role":"user","parts":[{"text":"`) {
		t.Errorf("history file: %s %v", b, err)
	}
	if h.reg.Get("chat-with-ai").Description != "Send a message to the AI. Returns the AI's response." {
		t.Errorf("description %q", h.reg.Get("chat-with-ai").Description)
	}
}

func TestStartNewLogsAndNames(t *testing.T) {
	h := newHarness(t, "ok", "Chess!", `"chess.json"`, "ok2", "ok3")
	h.run(t, "chat-with-ai chess")
	out := h.run(t, "start new chat-with-ai")
	want := []string{"Finalizing previous chat with AI...", "Previous conversation saved as: chess.json", "Starting new chat with AI...", "New chat with AI started."}
	if got := lines(out); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("log lines %q", got)
	}
	h.env.State.SetMode("")
	// The new conversation (instructions only) is archived too, as foo.
	h.sp.Replies = append(h.sp.Replies, `"chess.json"`)
	out = h.run(t, "start new chat-with-ai foo")
	if got := lines(out); got[len(got)-2] != "Current conversation will be saved as: foo" || got[len(got)-1] != "New chat with AI started." {
		t.Errorf("log lines %q", got)
	}
	if h.env.Settings.String(chat.SaveAsKey) != "foo" {
		t.Errorf("setting %q", h.env.Settings.String(chat.SaveAsKey))
	}
	if keys := h.env.Settings.Keys(); len(keys) == 0 || keys[0] != chat.SaveAsKey {
		t.Errorf("save-as key not public: %v", keys)
	}
	if again, _ := config.Load(h.env.Paths.SettingsFile()); again.String(chat.SaveAsKey) != "foo" {
		t.Error("setting not saved")
	}
	h.env.State.SetMode("")
	if _, err := h.reg.Execute(context.Background(), h.env, "start new chat-with-ai a b"); err == nil || err.Error() != "Command limited to 1 argument." {
		t.Errorf("err %v", err)
	}
}

func TestStartOldSendAndTemplates(t *testing.T) {
	h := newHarness(t, "A2", "ok")
	c := &history.Conversation{}
	c.Append(history.RoleUser, "q1")
	c.Append(history.RoleAssistant, "a1")
	if err := c.Save(filepath.Join(h.env.Paths.Root, config.Histories, "old.json")); err != nil {
		t.Fatal(err)
	}
	for line, want := range map[string]string{
		"start old chat-with-ai":            "Command takes 1 argument.",
		"start send chat-with-ai":           "Command takes 1 argument.",
		"start templated chat-with-ai":      "Command takes 1 argument.",
		"start new-templated chat-with-ai":  "Command takes 1 argument.",
		"start old chat-with-ai nope":       "conversation not found: nope",
		"start templated chat-with-ai nope": "Template not found",
	} {
		if _, err := h.reg.Execute(context.Background(), h.env, line); err == nil || err.Error() != want {
			t.Errorf("%q: %v", line, err)
		}
	}
	if h.env.State.Mode() != "" {
		t.Fatal("mode switched on a failed start")
	}
	out := h.run(t, "start old chat-with-ai old.json extra args ignored")
	if got := lines(out); strings.Join(got, "|") != "Starting old chat with AI...|Old chat with AI started." {
		t.Errorf("log lines %q", got)
	}
	h.env.State.SetMode("")
	file := filepath.Join(t.TempDir(), "msg.txt")
	os.WriteFile(file, []byte("q2"), 0o644)
	out = h.run(t, "start send chat-with-ai "+file)
	if !strings.Contains(out, "A2\n\n") || !strings.Contains(out, "Send chat with AI started.") || strings.Contains(out, "Resuming") {
		t.Errorf("send out %q", out)
	}
	h.env.State.SetMode("")
	out = h.run(t, "start new-templated chat-with-ai tpl")
	tmpl := filepath.Join(h.env.Paths.Root, config.Templates, "tpl")
	if !strings.Contains(out, "Template created: "+tmpl) || !history.Exists(tmpl) {
		t.Errorf("new-templated %q", out)
	}
	h.env.State.SetMode("")
	// Archiving the current conversation saves it back to old.json. The
	// template has no histories/ source, which the original reported.
	out = h.run(t, "start templated chat-with-ai tpl")
	if got := lines(out); strings.Join(got, "|") != "Template not found|Finalizing previous chat with AI...|Previous conversation saved as: old.json|Starting template chat with AI...|Templated chat with AI started." {
		t.Errorf("log lines %q", got)
	}
	saved, _ := history.Load(filepath.Join(h.env.Paths.Root, config.Histories, "old.json"))
	if len(saved.Messages) != 4 {
		t.Errorf("saved back %d messages", len(saved.Messages))
	}
	// An empty reply from "send" does not start.
	h.env.State.SetMode("")
	h.sp.Replies = []string{""}
	// (runStart swallows ErrNotStarted: no log, no mode change, no error.)
	_, err := h.reg.Execute(context.Background(), h.env, "start send chat-with-ai "+file)
	if err != nil || h.env.State.Mode() != "" {
		t.Errorf("empty send: %v mode %q", err, h.env.State.Mode())
	}
}

func TestLastAndRecapOutput(t *testing.T) {
	h := newHarness(t, "ok", "a1", "a2")
	for _, line := range []string{"start last chat-with-ai", "start recap chat-with-ai"} {
		if _, err := h.reg.Execute(context.Background(), h.env, line); err != nil || h.env.State.Mode() != "" {
			t.Errorf("%q on empty conversation: %v (mode %q)", line, err, h.env.State.Mode())
		}
	}
	h.run(t, "chat-with-ai q1")
	h.run(t, "chat-with-ai q2")
	out := h.run(t, "start recap chat-with-ai")
	want := []string{"----------------", "user-chat-input>  q2", "----------------", "a2", "Recap chat with AI started."}
	if got := lines(out); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("recap %q", got)
	}
	h.env.State.SetMode("")
	out = h.run(t, "start last chat-with-ai")
	want = []string{"----------------", "user-chat-input>  q2", "----------------", "a1", "Last chat with AI started."}
	if got := lines(out); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("last %q", got)
	}
	if h.env.Chat.Session("").Len() != 4 {
		t.Errorf("len %d", h.env.Chat.Session("").Len())
	}
	h.env.State.SetMode("")
	h.run(t, "start last chat-with-ai")
	h.env.State.SetMode("")
	// Undoing down to nothing is refused.
	if _, err := h.reg.Execute(context.Background(), h.env, "start last chat-with-ai"); err != nil || h.env.State.Mode() != "" || h.env.Chat.Session("").Len() != 2 {
		t.Errorf("undo to empty: %v len %d", err, h.env.Chat.Session("").Len())
	}
	if h.reg.Get("chat-with-ai").Start["undo"] != nil {
		t.Error("undo alias still registered")
	}
}
