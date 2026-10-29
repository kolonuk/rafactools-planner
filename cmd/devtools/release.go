package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var releaseDirRe = regexp.MustCompile(`^1\.([0-9]+)$`)

// newestRelease returns the number N of the newest docs snapshot 1.N under
// root, or 0 when there is none.
func newestRelease(root string) (int, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0, err
	}
	var nums []int
	for _, e := range entries {
		if m := releaseDirRe.FindStringSubmatch(e.Name()); e.IsDir() && m != nil {
			n, _ := strconv.Atoi(m[1])
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	if len(nums) == 0 {
		return 0, nil
	}
	return nums[len(nums)-1], nil
}

// pendingRelease reports whether there is anything to release: a docs page
// with unreleased front matter, or a non-empty "## Unreleased" section in the
// changelog.
func pendingRelease(root, changelog string) (bool, error) {
	entries, err := os.ReadDir(filepath.Join(root, "next"))
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(root, "next", e.Name())) // #nosec G304 -- entry of the fixed docs root
		if err != nil {
			return false, err
		}
		if relabelNext(string(raw), "1.0") != string(raw) {
			return true, nil
		}
	}
	raw, err := os.ReadFile(changelog) // #nosec G304 -- fixed path chosen by the caller
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	_, body, ok := unreleasedSection(string(raw))
	return ok && strings.TrimSpace(body) != "", nil
}

// unreleasedSection finds "## Unreleased" and returns its line index and body
// (up to the next "## " heading).
func unreleasedSection(text string) (int, string, bool) {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "## Unreleased" {
			continue
		}
		var body []string
		for _, n := range lines[i+1:] {
			if strings.HasPrefix(n, "## ") {
				break
			}
			body = append(body, n)
		}
		return i, strings.Join(body, "\n"), true
	}
	return 0, "", false
}

// releaseDocs cuts the next release when something is pending: it freezes the
// working docs as 1.N+1 and turns the changelog's "## Unreleased" heading into
// "## 1.N+1". It returns the version the build should carry (the new release,
// or the newest existing one when nothing is pending).
func releaseDocs(root, changelog string) (version string, released bool, err error) {
	n, err := newestRelease(root)
	if err != nil {
		return "", false, err
	}
	pending, err := pendingRelease(root, changelog)
	if err != nil {
		return "", false, err
	}
	if !pending {
		if n == 0 {
			return "1.0", false, nil
		}
		return fmt.Sprintf("1.%d", n), false, nil
	}
	version = fmt.Sprintf("1.%d", n+1)
	if err := snapshotDocs(root, version); err != nil {
		return "", false, err
	}
	if raw, rerr := os.ReadFile(changelog); rerr == nil { // #nosec G304 -- fixed path chosen by the caller
		if i, _, ok := unreleasedSection(string(raw)); ok {
			lines := strings.Split(string(raw), "\n")
			lines[i] = "## " + version
			if err := os.WriteFile(changelog, []byte(strings.Join(lines, "\n")), 0o644); err != nil { // #nosec G306 -- repo content
				return "", false, err
			}
		}
	}
	return version, true, nil
}

// runRelease is "devtools release": for the pipeline on the test branch. It
// prints version=... and released=... lines for $GITHUB_OUTPUT.
func runRelease() error {
	v, released, err := releaseDocs("internal/docs/content", "CHANGELOG.md")
	if err != nil {
		return err
	}
	fmt.Printf("version=%s\nreleased=%t\n", v, released)
	return nil
}
