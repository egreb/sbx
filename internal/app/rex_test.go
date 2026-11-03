package app

import (
	"strings"
	"testing"
)

func TestRexActionArgsAreSortedAndUnquoted(t *testing.T) {
	got := rexActionArgs("sbx_open_workspace", map[string]string{"prompt": "Fix it; a=b", "name": "fix"})
	want := "do sbx_open_workspace name=fix prompt=Fix it; a=b"
	if strings.Join(got, " ") != want || len(got) != 4 {
		t.Fatalf("got %q", got)
	}
}
