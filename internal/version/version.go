// Package version holds the build identity, injected at link time.
package version

import "fmt"

// Set through -ldflags by the Makefile.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// String returns the one-line identity printed by `d8s version`.
func String() string {
	v := Version
	if v != "dev" {
		v = "v" + v
	}
	return fmt.Sprintf("d8s %s (%s, %s)", v, Commit, Date)
}
