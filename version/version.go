// Package version exposes build-time metadata injected via -ldflags.
//
// GoReleaser injects these at release time:
//
//	-X github.com/kmdn-ch/ledgeralps/version.version={{.Version}}
//	-X github.com/kmdn-ch/ledgeralps/version.commit={{.Commit}}
//	-X github.com/kmdn-ch/ledgeralps/version.date={{.CommitDate}}
//	-X github.com/kmdn-ch/ledgeralps/version.builtBy=goreleaser
package version

import "fmt"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
	builtBy = "local"
)

// Version returns the semantic version string (e.g. "v1.2.3").
func Version() string { return version }

// Commit returns the short Git commit SHA.
func Commit() string { return commit }

// Date returns the build timestamp in RFC3339 format.
func Date() string { return date }

// BuiltBy returns the build tool identifier (goreleaser, make, etc.).
func BuiltBy() string { return builtBy }

// Info returns a formatted version string suitable for User-Agent headers and logs.
func Info() string {
	return fmt.Sprintf(
		"ledgeralps/%s (commit=%s, built=%s by %s)",
		version, commit, date, builtBy,
	)
}
