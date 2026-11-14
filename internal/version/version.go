// Package version holds the Esquadro version, overridable at build time with
// -ldflags "-X github.com/patrickluzdev/esquadro/internal/version.Version=v1.2.3".
package version

import "runtime/debug"

// Version is set at build time. When empty, the module version from the build
// info is used (e.g. when installed with go install).
var Version = ""

// String returns the best known version of this binary.
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
