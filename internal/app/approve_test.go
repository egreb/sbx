package app

import "testing"

func TestParseApprovalTable(t *testing.T) {
	output := `
  APPROVAL                SANDBOX   TITLE                OPTIONS
  network:qJZd-y_uoy4L    smokey    claude.ai:443        allow (Allow), dismiss (Dismiss)
  network:q6_TvOX0rcrf    pooh      cloud.google.com:443 allow (Allow), dismiss (Dismiss)

`
	header, rows := parseApprovalTable(output)
	if header != "APPROVAL                SANDBOX   TITLE                OPTIONS" {
		t.Fatalf("unexpected header: %q", header)
	}
	if len(rows) != 2 || rows[0][:20] != "network:qJZd-y_uoy4L" {
		t.Fatalf("unexpected rows: %q", rows)
	}
}

func TestParseApprovalTableEmpty(t *testing.T) {
	for _, output := range []string{"", "\n", "No pending approvals\n", "  APPROVAL   SANDBOX\n"} {
		if _, rows := parseApprovalTable(output); len(rows) != 0 {
			t.Fatalf("%q: expected no rows, got %q", output, rows)
		}
	}
}
