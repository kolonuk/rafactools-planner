package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// previousStage is the branch a commit must already be on, and have passed CI
// on, before it may be pushed to the key: dev, then test, then main.
var previousStage = map[string]string{"test": "dev", "main": "test"}

// promotionEnv is what checkPromotion needs from the outside world, so the rule
// can be tested without git or GitHub.
type promotionEnv interface {
	// OnBranch reports whether sha is already part of origin/<branch>.
	OnBranch(sha, branch string) (bool, error)
	// Subject and Parent describe a commit (the first parent of a merge).
	Subject(sha string) (string, error)
	Parent(sha string) (string, error)
	// CIResult is the conclusion of the finished CI run for sha ("success",
	// "failure", ...), "running" when one is still going, or "" when there is none.
	CIResult(sha string) (string, error)
}

// checkPromotion enforces dev -> test -> main. A push to dev is always fine. A
// push to test or main must already be on the previous branch, and CI must have
// passed on it there. A release commit made by the pipeline ("release 1.N ...",
// made without its own CI run) has none, so its parent is checked.
func checkPromotion(env promotionEnv, branch, sha string) error {
	prev, gated := previousStage[branch]
	if !gated {
		return nil
	}
	on, err := env.OnBranch(sha, prev)
	if err != nil {
		return err
	}
	if !on {
		return fmt.Errorf("%s is not on %s yet: push it to %s first, let CI pass there, then promote it to %s", short(sha), prev, prev, branch)
	}
	target := sha
	if subject, err := env.Subject(sha); err == nil && strings.HasPrefix(subject, "release ") || strings.HasPrefix(subject, "Merge release ") {
		if parent, err := env.Parent(sha); err == nil && parent != "" {
			target = parent
		}
	}
	switch res, err := env.CIResult(target); {
	case err != nil:
		return err
	case res == "success":
		return nil
	case res == "running":
		return fmt.Errorf("CI is still running for %s: wait for it to pass on %s, then promote it to %s", short(target), prev, branch)
	case res == "":
		return fmt.Errorf("no CI run found for %s: it must pass CI on %s before it goes to %s", short(target), prev, branch)
	default:
		return fmt.Errorf("CI did not pass for %s (%s): fix it on %s before promoting it to %s", short(target), res, prev, branch)
	}
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// liveEnv is promotionEnv backed by the checked-out repository and GitHub's API.
type liveEnv struct {
	repo, token, api string
	hc               *http.Client
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput() // #nosec G204 -- fixed program, arguments built from validated hashes and branch names
	return strings.TrimSpace(string(out)), err
}

func (l liveEnv) OnBranch(sha, branch string) (bool, error) {
	if out, err := git("fetch", "--quiet", "origin", branch); err != nil {
		return false, fmt.Errorf("fetch %s: %v: %s", branch, err, out)
	}
	cmd := exec.Command("git", "merge-base", "--is-ancestor", sha, "origin/"+branch) // #nosec G204 -- fixed program; sha is a hash from the workflow, branch from a fixed map
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	default:
		return false, fmt.Errorf("compare with %s: %w", branch, err)
	}
}

func (l liveEnv) Subject(sha string) (string, error) { return git("log", "-1", "--format=%s", sha) }
func (l liveEnv) Parent(sha string) (string, error)  { return git("rev-parse", sha+"^1") }

func (l liveEnv) CIResult(sha string) (string, error) {
	q := url.Values{"head_sha": {sha}, "per_page": {"20"}}
	req, err := http.NewRequest(http.MethodGet, l.api+"/repos/"+l.repo+"/actions/workflows/ci.yml/runs?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := l.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub answered %d looking for the CI run of %s", resp.StatusCode, short(sha))
	}
	var body struct {
		Runs []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", err
	}
	return summariseCI(func() (out []ciRun) {
		for _, r := range body.Runs {
			out = append(out, ciRun{r.Status, r.Conclusion})
		}
		return out
	}()), nil
}

type ciRun struct{ status, conclusion string }

// summariseCI picks one answer from every CI run of a commit (it can run on
// dev, then again after promotion): any success counts, else a run in
// progress, else the latest failure.
func summariseCI(runs []ciRun) string {
	result := ""
	for _, r := range runs {
		switch {
		case r.status == "completed" && r.conclusion == "success":
			return "success"
		case r.status != "completed":
			result = "running"
		case result != "running":
			result = r.conclusion
		}
	}
	return result
}

// runPromotionGate is "devtools promotion-gate" for the deploy workflow. It
// reads GITHUB_REF_NAME, GITHUB_SHA, GITHUB_REPOSITORY and GITHUB_TOKEN.
func runPromotionGate() error {
	branch, sha := os.Getenv("GITHUB_REF_NAME"), os.Getenv("GITHUB_SHA")
	if _, gated := previousStage[branch]; !gated {
		fmt.Printf("%s is not gated: nothing to check\n", branch)
		return nil
	}
	api := os.Getenv("GITHUB_API_URL")
	if api == "" {
		api = "https://api.github.com"
	}
	env := liveEnv{repo: os.Getenv("GITHUB_REPOSITORY"), token: os.Getenv("GITHUB_TOKEN"), api: api, hc: &http.Client{Timeout: 30 * time.Second}}
	if env.repo == "" || env.token == "" || sha == "" {
		return errors.New("GITHUB_REPOSITORY, GITHUB_TOKEN and GITHUB_SHA must be set")
	}
	if err := checkPromotion(env, branch, sha); err != nil {
		return err
	}
	fmt.Printf("%s may go to %s: it is on %s and passed CI there\n", short(sha), branch, previousStage[branch])
	return nil
}
