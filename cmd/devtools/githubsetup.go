package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// setting is one GitHub Actions variable or secret the deployment needs.
type setting struct {
	Kind        string // "variable" or "secret"
	Name        string
	Placeholder string // variables only: a harmless value to fill in later
	Purpose     string
}

var (
	// Owner and name must not start with "-": the value is passed to gh, which
	// would read it as a flag.
	repoRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9_.][A-Za-z0-9._-]*$`)
	prefixRe = regexp.MustCompile(`^[A-Z0-9_]*$`)
)

// masterAdminSettings are the settings the master admin and problem
// reporting need. The password is a SECRET: GitHub variables are plain text.
func masterAdminSettings(prefix string) []setting {
	return []setting{
		{"variable", prefix + "SITE_DOMAIN", "CHANGE_ME", "Domain of the site (for example cadets.example.org): the default for the no-reply@ sender and the issues@ address that problem reports go to. Optional: it defaults to the host of BASE_URL."},
		{"variable", prefix + "ISSUE_MASKING", "true", "true lets people black out parts of a screenshot before sending; set false only on a site you run for others."},
		{"variable", prefix + "MASTER_ADMIN_EMAIL", "CHANGE_ME", "Email address of the site-wide master admin (not secret)."},
		{"secret", prefix + "MASTER_ADMIN_PASSWORD", "", "Password of the master admin, 12 to 72 characters. Must be a secret."},
	}
}

// setCommand is the gh invocation that creates a setting. A secret's value is
// never put on the command line: gh prompts for it without echoing.
func setCommand(repo string, s setting) []string {
	if s.Kind == "secret" {
		return []string{"gh", "secret", "set", s.Name, "--repo", repo}
	}
	return []string{"gh", "variable", "set", s.Name, "--repo", repo, "--body", s.Placeholder}
}

func runGithubSetup(args []string) error {
	fs := flag.NewFlagSet("github-setup", flag.ContinueOnError)
	repo := fs.String("repo", "kolonuk/rafactools-planner", "GitHub repository (owner/name) that holds the settings")
	prefix := fs.String("prefix", "", "optional prefix for the names, for example MYAPP_")
	apply := fs.Bool("apply", false, "create them now with the gh CLI (you must be signed in with `gh auth login`)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !repoRe.MatchString(*repo) || strings.HasSuffix(*repo, "/.") || strings.HasSuffix(*repo, "/..") {
		return fmt.Errorf("-repo must look like owner/name")
	}
	if !prefixRe.MatchString(*prefix) {
		return fmt.Errorf("-prefix may only contain A-Z, 0-9 and _")
	}

	settings := masterAdminSettings(*prefix)
	fmt.Printf("Master admin and problem-report settings for %s\n\n", *repo)
	for _, s := range settings {
		fmt.Printf("  %-8s %-34s %s\n", s.Kind, s.Name, s.Purpose)
	}
	fmt.Println("\nCommands (the secret is typed when gh asks, never on the command line):")
	for _, s := range settings {
		fmt.Println("  " + strings.Join(setCommand(*repo, s), " "))
	}
	if !*apply {
		fmt.Println("\nRun again with -apply to run them. Then set the real email values, and give the")
		fmt.Println("deployment stack each setting without the prefix (MASTER_ADMIN_EMAIL, SITE_DOMAIN, ...).")
		return nil
	}
	for _, s := range settings {
		argv := setCommand(*repo, s)
		cmd := exec.Command(argv[0], argv[1:]...) // #nosec G204 -- fixed tool; repo and names validated above
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	fmt.Println("\nDone. Edit the variable to the real email in GitHub > Settings > Secrets and variables > Actions.")
	return nil
}
