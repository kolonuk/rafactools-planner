package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var versionArgRe = regexp.MustCompile(`^1\.[0-9]+$`)

// relabelNext rewrites the front-matter lines that belong to the release
// being cut. Only lines of exactly these shapes change, so page text that
// happens to mention "next" is untouched.
func relabelNext(page, ver string) string {
	lines := strings.Split(page, "\n")
	inFrontMatter := false
	for i, l := range lines {
		if l == "---" {
			if i == 0 {
				inFrontMatter = true
				continue
			}
			if inFrontMatter {
				break
			}
		}
		switch {
		case strings.HasPrefix(l, "  - next: "):
			lines[i] = "  - " + ver + ": " + strings.TrimPrefix(l, "  - next: ")
		case l == "since: next":
			lines[i] = "since: " + ver
		}
	}
	return strings.Join(lines, "\n")
}

// snapshotDocs freezes <root>/next as <root>/<ver>. "next" entries are
// relabelled to ver in both copies, because they now belong to that release.
func snapshotDocs(root, ver string) error {
	if !versionArgRe.MatchString(ver) {
		return fmt.Errorf("version must look like 1.<number> (got %q)", ver)
	}
	dst := filepath.Join(root, ver)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists: released snapshots are never overwritten", dst)
	}
	src := filepath.Join(root, "next")
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read %s: %w (run from the repository root)", src, err)
	}
	err = os.MkdirAll(dst, 0o755) // #nosec G301 -- checked-in repo content needs normal permissions
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			return fmt.Errorf("%s/%s: only .md pages are expected", src, e.Name())
		}
		path := filepath.Join(src, e.Name())
		raw, err := os.ReadFile(path) // #nosec G304 -- path is a directory entry under the fixed docs root
		if err != nil {
			return err
		}
		out := relabelNext(string(raw), ver)
		if err := os.WriteFile(path, []byte(out), fs.FileMode(0o644)); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), []byte(out), fs.FileMode(0o644)); err != nil {
			return err
		}
	}
	return nil
}

func runDocsSnapshot(ver string) error {
	if err := snapshotDocs("internal/docs/content", ver); err != nil {
		return err
	}
	cmd := exec.Command("go", "test", "./internal/docs", "-count=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docs tests failed after snapshot: %w", err)
	}
	fmt.Printf("Snapshot %s created. Review it in the app (Help > Version), then commit.\n", ver)
	return nil
}
