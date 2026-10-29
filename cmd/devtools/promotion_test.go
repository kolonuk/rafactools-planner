package main

import (
	"strings"
	"testing"
)

type fakeEnv struct {
	on      map[string]bool // "branch:sha"
	subject map[string]string
	parent  map[string]string
	ci      map[string]string
}

func (f fakeEnv) OnBranch(sha, branch string) (bool, error) { return f.on[branch+":"+sha], nil }
func (f fakeEnv) Subject(sha string) (string, error)        { return f.subject[sha], nil }
func (f fakeEnv) Parent(sha string) (string, error)         { return f.parent[sha], nil }
func (f fakeEnv) CIResult(sha string) (string, error)       { return f.ci[sha], nil }

func TestPromotionOrderIsDevThenTestThenMain(t *testing.T) {
	env := fakeEnv{
		on:      map[string]bool{"dev:mrg": true, "dev:aaa": true, "test:aaa": true, "dev:rel": true, "test:rel": true},
		subject: map[string]string{"mrg": "Merge release 1.10 from test into dev [skip ci]", "rel": "release 1.10: freeze the docs and changelog [skip ci]"},
		parent:  map[string]string{"rel": "aaa", "mrg": "aaa"},
		ci:      map[string]string{"aaa": "success", "bbb": "failure", "ccc": "running"},
	}
	cases := []struct {
		name, branch, sha, wantErr string
	}{
		{"anything may go to dev", "dev", "zzz", ""},
		{"a commit on dev with passing CI may go to test", "test", "aaa", ""},
		{"a commit not on dev may not go to test", "test", "nope", "not on dev yet"},
		{"a commit not on test may not go to main", "main", "nope", "not on test yet"},
		{"on test with passing CI may go to main", "main", "aaa", ""},
		{"the pipeline's release commit is judged by its parent's CI", "main", "rel", ""},
		{"so is the merge of a release back into dev", "test", "mrg", ""},
		{"failing CI blocks promotion", "test", "bbb", "CI did not pass"},
		{"CI still running blocks promotion", "test", "ccc", "still running"},
		{"no CI run blocks promotion", "test", "ddd", "no CI run"},
		{"other branches are not gated", "feature", "zzz", ""},
	}
	env.on["dev:bbb"], env.on["dev:ccc"], env.on["dev:ddd"] = true, true, true
	for _, tc := range cases {
		err := checkPromotion(env, tc.branch, tc.sha)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: got %v, want an error containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestSummariseCIPicksOneAnswer(t *testing.T) {
	for name, tc := range map[string]struct {
		runs []ciRun
		want string
	}{
		"none":                       {nil, ""},
		"a success wins over a fail": {[]ciRun{{"completed", "failure"}, {"completed", "success"}}, "success"},
		"running beats a failure":    {[]ciRun{{"completed", "failure"}, {"in_progress", ""}}, "running"},
		"a failure alone":            {[]ciRun{{"completed", "failure"}}, "failure"},
	} {
		if got := summariseCI(tc.runs); got != tc.want {
			t.Errorf("%s: %q, want %q", name, got, tc.want)
		}
	}
}
