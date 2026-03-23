// Package version provides build/version metadata helpers.
package version

import "runtime/debug"

// Version is the default version string, overridden at build time with -ldflags.
var Version = "dev"

// String returns the build version if available, otherwise the default Version.
func String() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}
	return Version
}
