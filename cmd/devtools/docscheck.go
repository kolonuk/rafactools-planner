package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

var (
	// User-visible code: a change here must come with a docs change.
	visibleRe = regexp.MustCompile(`^(internal/(web|config|auth|mail|pdfgen)/|cmd/server/|Dockerfile$|docker-compose\.yml$|\.github/workflows/)`)
	// A reason of at least 15 characters, on a line of its own.
	noImpactRe = regexp.MustCompile(`(?m)^No docs impact: .{15,}`)
)

// checkDocs decides whether a set of changed files needs a docs change.
// It returns ok=false and a message when the change must be refused.
func checkDocs(changed []string, prBody string) (ok bool, msg string) {
	var visible []string
	docsTouched := false
	for _, f := range changed {
		if strings.HasPrefix(f, "internal/docs/content/") {
			docsTouched = true
		}
		if visibleRe.MatchString(f) && !strings.HasSuffix(f, "_test.go") {
			visible = append(visible, f)
		}
	}
	switch {
	case len(visible) == 0:
		return true, "No user-visible files changed; docs not required."
	case docsTouched:
		return true, "User-visible files changed and the docs were updated."
	case noImpactRe.MatchString(prBody):
		return true, "No docs impact declared in the PR description."
	}
	return false, "User-visible files changed but internal/docs/content/ did not:\n  " +
		strings.Join(visible, "\n  ") +
		"\nUpdate the matching pages in internal/docs/content/next/ (see .claude/skills/docs/SKILL.md),\n" +
		"or add a line to the PR description:  No docs impact: <specific reason>"
}

// runDocsCheck reads BASE and HEAD (commit SHAs) and PR_BODY from the
// environment. The body is read here as data and never passed to a shell.
func runDocsCheck() error {
	base, head := os.Getenv("BASE"), os.Getenv("HEAD")
	// They become arguments to git, so insist on plain commit hashes.
	if !shaRe.MatchString(base) || !shaRe.MatchString(head) {
		return fmt.Errorf("BASE and HEAD must be commit hashes")
	}
	out, err := exec.Command("git", "diff", "--name-only", base+"..."+head).Output() // #nosec G204 -- both values validated as hex above
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}
	var changed []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			changed = append(changed, l)
		}
	}
	ok, msg := checkDocs(changed, os.Getenv("PR_BODY"))
	fmt.Println(msg)
	if !ok {
		return fmt.Errorf("docs check failed")
	}
	return nil
}
