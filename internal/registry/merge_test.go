package registry

import (
	"strings"
	"testing"
)

func lines(s string) []string { return splitLines(s) }

func TestMerge3(t *testing.T) {
	cases := []struct {
		name            string
		base, local, up string
		want            string
		wantConflict    bool
	}{
		{
			name:  "no changes",
			base:  "a\nb\nc\n",
			local: "a\nb\nc\n",
			up:    "a\nb\nc\n",
			want:  "a\nb\nc\n",
		},
		{
			name:  "local only",
			base:  "a\nb\nc\n",
			local: "a\nB\nc\n",
			up:    "a\nb\nc\n",
			want:  "a\nB\nc\n",
		},
		{
			name:  "upstream only",
			base:  "a\nb\nc\n",
			local: "a\nb\nc\n",
			up:    "a\nb\nC\n",
			want:  "a\nb\nC\n",
		},
		{
			name:  "both same change",
			base:  "a\nb\nc\n",
			local: "a\nX\nc\n",
			up:    "a\nX\nc\n",
			want:  "a\nX\nc\n",
		},
		{
			name:  "disjoint changes both applied",
			base:  "a\nb\nc\nd\ne\n",
			local: "A\nb\nc\nd\ne\n",
			up:    "a\nb\nc\nd\nE\n",
			want:  "A\nb\nc\nd\nE\n",
		},
		{
			name:  "upstream adds a line, local edits elsewhere",
			base:  "a\nb\nc\n",
			local: "a\nb\nc-edited\n",
			up:    "a\nb\nc\nd\n",
			want:  "a\nb\nc-edited\nd\n",
		},
		{
			name:         "conflict: same line changed differently",
			base:         "a\nb\nc\n",
			local:        "a\nLOCAL\nc\n",
			up:           "a\nUPSTREAM\nc\n",
			want:         "a\n" + conflictStart + "\nLOCAL\n" + conflictMid + "\nUPSTREAM\n" + conflictEnd + "\nc\n",
			wantConflict: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, conflict := merge3(lines(tc.base), lines(tc.local), lines(tc.up))
			if gotStr := joinLines(got); gotStr != tc.want {
				t.Errorf("merge3 mismatch:\n got: %q\nwant: %q", gotStr, tc.want)
			}
			if conflict != tc.wantConflict {
				t.Errorf("conflict = %v, want %v", conflict, tc.wantConflict)
			}
		})
	}
}

func TestMerge3_PreservesUnrelatedUpstreamAndLocal(t *testing.T) {
	// A realistic config-file scenario: upstream adds a new key at the top;
	// the user changed a value lower down. Both must survive.
	base := "version: 1\nname: app\nport: 8080\n"
	local := "version: 1\nname: app\nport: 9090\n"
	up := "version: 2\nname: app\nport: 8080\n"
	got, conflict := merge3(lines(base), lines(local), lines(up))
	if conflict {
		t.Fatal("unexpected conflict on disjoint edits")
	}
	g := joinLines(got)
	if !strings.Contains(g, "version: 2") || !strings.Contains(g, "port: 9090") {
		t.Errorf("merge lost an edit:\n%s", g)
	}
}

func TestUnifiedDiff(t *testing.T) {
	a := lines("a\nb\nc\n")
	b := lines("a\nB\nc\n")
	d := unifiedDiff("x.txt", a, b)
	if !strings.Contains(d, "-b") || !strings.Contains(d, "+B") {
		t.Errorf("unified diff missing change:\n%s", d)
	}
	// No-change ⇒ empty.
	if unifiedDiff("x", a, a) != "" {
		t.Error("expected empty diff for identical inputs")
	}
}

func TestSplitJoinRoundTrip(t *testing.T) {
	for _, s := range []string{"", "a\n", "a\nb\n", "a\nb"} {
		got := joinLines(splitLines(s))
		want := s
		if want != "" && !strings.HasSuffix(want, "\n") {
			want += "\n" // joinLines normalizes a missing trailing newline
		}
		if got != want {
			t.Errorf("round-trip %q -> %q, want %q", s, got, want)
		}
	}
}
