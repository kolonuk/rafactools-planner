package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckDocs(t *testing.T) {
	const reason = "No docs impact: internal refactor, no visible change"
	for name, tc := range map[string]struct {
		changed []string
		body    string
		want    bool
	}{
		"nothing user-visible":        {[]string{"README.md", "TODO.md", ".claude/skills/x"}, "", true},
		"code without docs":           {[]string{"internal/web/handlers_auth.go"}, "", false},
		"template without docs":       {[]string{"internal/web/templates/login.html"}, "", false},
		"config without docs":         {[]string{"internal/config/config.go"}, "", false},
		"workflow without docs":       {[]string{".github/workflows/ci.yml"}, "", false},
		"dockerfile without docs":     {[]string{"Dockerfile"}, "", false},
		"code with docs":              {[]string{"internal/web/web.go", "internal/docs/content/next/signing-in.md"}, "", true},
		"tests only":                  {[]string{"internal/web/security_test.go", "cmd/server/operational_test.go"}, "", true},
		"docs package code is exempt": {[]string{"internal/docs/render.go"}, "", true},
		"valid excuse":                {[]string{"internal/auth/auth.go"}, "Title\n\n" + reason + "\n", true},
		"excuse too short":            {[]string{"internal/auth/auth.go"}, "No docs impact: no", false},
		"excuse not at line start":    {[]string{"internal/auth/auth.go"}, "I say " + reason, false},
		"excuse missing the reason":   {[]string{"internal/auth/auth.go"}, "No docs impact:", false},
	} {
		ok, msg := checkDocs(tc.changed, tc.body)
		if ok != tc.want {
			t.Errorf("%s: ok=%v, want %v (%s)", name, ok, tc.want, msg)
		}
	}
	if _, msg := checkDocs([]string{"internal/web/a.go", "cmd/server/main.go"}, ""); !strings.Contains(msg, "internal/web/a.go") || !strings.Contains(msg, "cmd/server/main.go") {
		t.Errorf("failure message does not list the offending files: %s", msg)
	}
}

func TestRelabelNext(t *testing.T) {
	in := "---\ntitle: T\nsince: next\nchanged:\n  - next: Did a thing.\n  - 1.2: Older.\n---\nBody says since: next\n  - next: not front matter\n"
	got := relabelNext(in, "1.7")
	want := "---\ntitle: T\nsince: 1.7\nchanged:\n  - 1.7: Did a thing.\n  - 1.2: Older.\n---\nBody says since: next\n  - next: not front matter\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if relabelNext("---\nsince: 1.0\n---\nx", "1.7") != "---\nsince: 1.0\n---\nx" {
		t.Error("changed a page with nothing to relabel")
	}
}

func TestSnapshotDocs(t *testing.T) {
	root := t.TempDir()
	next := filepath.Join(root, "next")
	os.Mkdir(next, 0o755)
	page := "---\ntitle: T\norder: 0\nsummary: S\nsince: next\nchanged:\n  - next: New.\n---\nBody\n"
	os.WriteFile(filepath.Join(next, "index.md"), []byte(page), 0o644)

	if err := snapshotDocs(root, "1.7"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(next, "index.md"), filepath.Join(root, "1.7", "index.md")} {
		b, _ := os.ReadFile(p)
		if !strings.Contains(string(b), "since: 1.7") || !strings.Contains(string(b), "  - 1.7: New.") {
			t.Errorf("%s not relabelled:\n%s", p, b)
		}
	}
	if err := snapshotDocs(root, "1.7"); err == nil {
		t.Error("existing snapshot overwritten")
	}
	for _, bad := range []string{"", "next", "2.0", "1.x", "../1.8", "1.7/../x"} {
		if err := snapshotDocs(root, bad); err == nil {
			t.Errorf("version %q accepted", bad)
		}
	}
	if entries, _ := os.ReadDir(root); len(entries) != 2 {
		t.Errorf("rejected versions left directories behind: %v", entries)
	}
}

func TestDocsCheckRejectsNonHashRevisions(t *testing.T) {
	// BASE/HEAD become git arguments; anything that is not a commit hash,
	// notably something starting with "-", must be refused before git runs.
	for _, bad := range []string{"", "--output=/tmp/x", "-p", "main; rm -rf /", "HEAD~1", "xyz"} {
		t.Setenv("BASE", bad)
		t.Setenv("HEAD", "0123456789abcdef0123456789abcdef01234567")
		if err := runDocsCheck(); err == nil || !strings.Contains(err.Error(), "commit hashes") {
			t.Errorf("BASE=%q: err = %v", bad, err)
		}
	}
}

func TestGithubSetupSettings(t *testing.T) {
	all := masterAdminSettings("RAFACTOOLS_")
	byName := map[string]setting{}
	for _, x := range all {
		byName[x.Name] = x
	}
	for name, kind := range map[string]string{
		"RAFACTOOLS_MASTER_ADMIN_EMAIL": "variable", "RAFACTOOLS_MASTER_ADMIN_PASSWORD": "secret",
		"RAFACTOOLS_SITE_DOMAIN": "variable", "RAFACTOOLS_ISSUE_MASKING": "variable",
	} {
		if byName[name].Kind != kind {
			t.Errorf("%s is %q, want %q", name, byName[name].Kind, kind)
		}
	}
	if len(all) != 4 {
		t.Errorf("%d settings, want 4", len(all))
	}
	for _, x := range all {
		if x.Kind == "variable" && x.Placeholder == "" {
			t.Errorf("variable %s has no placeholder", x.Name)
		}
	}
	s := []setting{byName["RAFACTOOLS_MASTER_ADMIN_EMAIL"], byName["RAFACTOOLS_MASTER_ADMIN_PASSWORD"]}
	if got := masterAdminSettings(""); got[len(got)-1].Name != "MASTER_ADMIN_PASSWORD" {
		t.Errorf("no-prefix names = %+v", got)
	}
	// The password must never be on a command line, where it would show in
	// process listings and shell history.
	cmd := strings.Join(setCommand("o/r", s[1]), " ")
	if strings.Contains(cmd, "--body") || strings.Contains(cmd, "--env-file") || s[1].Placeholder != "" {
		t.Errorf("secret command carries a value: %s", cmd)
	}
	if got := strings.Join(setCommand("o/r", s[0]), " "); got != "gh variable set RAFACTOOLS_MASTER_ADMIN_EMAIL --repo o/r --body CHANGE_ME" {
		t.Errorf("variable command = %s", got)
	}
}

func TestGithubSetupRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-repo", "not a repo"}, {"-repo", "a/b/c"}, {"-repo", "--foo/bar"}, {"-repo", "owner/name; rm -rf /"},
		{"-prefix", "lower_"}, {"-prefix", "A B"}, {"-prefix", "X;"},
	} {
		if err := runGithubSetup(args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if err := runGithubSetup(nil); err != nil { // prints the plan, runs nothing
		t.Errorf("default run: %v", err)
	}
}
