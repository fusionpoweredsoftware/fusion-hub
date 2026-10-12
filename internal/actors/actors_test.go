package actors

import (
	"strings"
	"testing"
)

func TestOpenResolveRename(t *testing.T) {
	book, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, id, err := book.Resolve("", "", "")
	if err != nil || id != DefaultID || a.Person != "Albert Einstein" {
		t.Fatalf("default = %#v %s %v", a, id, err)
	}
	a, id, _ = book.Resolve("earhart:amelia", "", "")
	if id != "AMELIA" || a.Person != "Amelia Earhart" {
		t.Fatalf("rename = %#v %s", a, id)
	}
	again, _ := Open(strings.TrimSuffix(book.Path, "/actors.json"))
	if _, ok := again.Profiles["AMELIA"]; !ok {
		t.Error("rename not saved")
	}
	if _, _, err := book.Resolve("nobody", "", ""); err == nil {
		t.Error("unknown actor accepted")
	}
	if a, _, _ := book.Resolve("nobody", "Ada Lovelace", "I wrote the first program."); a.Person != "Ada Lovelace" {
		t.Error("explicit person ignored")
	}
}

func TestPromptAndSpeakable(t *testing.T) {
	a := Actor{Person: "Ada", Biosignature: "bio"}
	p := Prompt(a, "hello", "U1", true, map[int]string{1: "!!"})
	for _, want := range []string{"*** plaintext (U1:response)", "(U1:input)\n", "non-English characters!!", "as if you were Ada"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	got := Speakable("Hello — world!\n\n(more)\n\nBye.")
	if len(got) != 2 || got[0] != "Hello  world!" || got[1] != "Bye." {
		t.Errorf("speakable %q", got)
	}
	if !HasCJK("abc 你好") || HasCJK("café") {
		t.Error("HasCJK")
	}
}
