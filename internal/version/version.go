// Package version holds the single build-time version string shown in the
// web footer and the PDF footer, so there's exactly one place either has to
// change to pick up a new scheme.
package version

// Version is set at build time via:
//
//	-ldflags "-X rafactools-planner/internal/version.Version=1.42"
//
// (see Dockerfile's VERSION build arg, set by .github/workflows/deploy.yml
// from the PR number associated with the commit being built). Left at
// "dev" for a local build with no version info supplied — e.g.
// `go build ./cmd/server` or `docker build .` with no --build-arg VERSION.
var Version = "dev"
