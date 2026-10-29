package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseFixture(t *testing.T, nextPage, changelog string) (root, cl string) {
	t.Helper()
	dir := t.TempDir()
	root = filepath.Join(dir, "content")
	for _, d := range []string{"next", "1.3", "1.9"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "next", "a.md"), []byte(nextPage), 0o644); err != nil {
		t.Fatal(err)
	}
	cl = filepath.Join(dir, "CHANGELOG.md")
	if err := os.WriteFile(cl, []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, cl
}

const releasedPage = "---\ntitle: A\nsince: 1.3\nchanged:\n  - 1.9: old\n---\nbody\n"

func TestReleaseDocsCutsTheNextNumberWhenSomethingIsPending(t *testing.T) {
	root, cl := releaseFixture(t, "---\ntitle: A\nsince: 1.3\nchanged:\n  - next: new\n---\nbody\n", "# V\n\n## Unreleased\n\n- **feat**: x\n\n## 1.9\n\n- old\n")
	v, released, err := releaseDocs(root, cl)
	if err != nil || !released || v != "1.10" {
		t.Fatalf("%q %v %v", v, released, err)
	}
	// 1.10 is newer than 1.9 (numeric, not text, order).
	if n, _ := newestRelease(root); n != 10 {
		t.Errorf("newest = %d", n)
	}
	snap, _ := os.ReadFile(filepath.Join(root, "1.10", "a.md"))
	work, _ := os.ReadFile(filepath.Join(root, "next", "a.md"))
	if !strings.Contains(string(snap), "  - 1.10: new") || string(snap) != string(work) {
		t.Errorf("snapshot/working copy not relabelled alike:\n%s\n%s", snap, work)
	}
	log, _ := os.ReadFile(cl)
	if !strings.Contains(string(log), "## 1.10\n") || strings.Contains(string(log), "Unreleased") {
		t.Errorf("changelog:\n%s", log)
	}
	// Running it again finds nothing pending and keeps the version.
	v, released, err = releaseDocs(root, cl)
	if err != nil || released || v != "1.10" {
		t.Errorf("second run: %q %v %v", v, released, err)
	}
}

func TestReleaseDocsDoesNothingWhenNothingIsPending(t *testing.T) {
	root, cl := releaseFixture(t, releasedPage, "# V\n\n## Unreleased\n\n## 1.9\n\n- old\n")
	v, released, err := releaseDocs(root, cl)
	if err != nil || released || v != "1.9" {
		t.Fatalf("%q %v %v", v, released, err)
	}
	if _, err := os.Stat(filepath.Join(root, "1.10")); err == nil {
		t.Error("a snapshot was made with nothing to release")
	}
}

func TestReleaseDocsChangelogAloneCountsAsPending(t *testing.T) {
	root, cl := releaseFixture(t, releasedPage, "# V\n\n## Unreleased\n\n- **fix**: y\n\n## 1.9\n")
	if v, released, err := releaseDocs(root, cl); err != nil || !released || v != "1.10" {
		t.Fatalf("%q %v %v", v, released, err)
	}
}
