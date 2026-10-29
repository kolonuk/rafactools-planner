// Command devtools holds the repository's own tooling, so none of it needs
// a shell or another language:
//
//	go run ./cmd/devtools docs-check            PR gate: docs must change with user-visible code
//	go run ./cmd/devtools docs-snapshot 1.42    freeze the working docs as release 1.42
//	go run ./cmd/devtools release              pipeline (test branch): cut the next release if anything is pending
//	go run ./cmd/devtools promotion-gate      pipeline (test, main): refuse a commit that skipped the previous stage
//	go run ./cmd/devtools validate-html         validate rendered pages with html-validate
//	go run ./cmd/devtools github-setup [-apply] create the master admin variable and secret on GitHub
//
// See .claude/skills/docs/SKILL.md for when each is used.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "docs-check":
		err = runDocsCheck()
	case "docs-snapshot":
		if len(os.Args) != 3 {
			usage()
		}
		err = runDocsSnapshot(os.Args[2])
	case "promotion-gate":
		err = runPromotionGate()
	case "release":
		err = runRelease()
	case "validate-html":
		err = runValidateHTML()
	case "github-setup":
		err = runGithubSetup(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "devtools:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: devtools docs-check | docs-snapshot 1.<number> | release | promotion-gate | validate-html | github-setup [-repo owner/name] [-prefix X_] [-apply]")
	os.Exit(2)
}
