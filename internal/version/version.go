// Package version carries build-time identity for the binary.
package version

import (
	"runtime/debug"
	"strings"
)

const defaultVersion = "0.0.0-dev"

// Version is the current version. Override at release / local build time via
// -ldflags "-X github.com/aixgo-dev/sync/internal/version.Version=...".
// go install ...@vX.Y.Z also fills it from module build info.
var Version = defaultVersion

func init() {
	Version = resolveVersion(Version, moduleVersion())
}

func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

func resolveVersion(linkVersion, buildVersion string) string {
	linkVersion = normalizeVersion(linkVersion)
	if linkVersion != "" && linkVersion != defaultVersion {
		return linkVersion
	}

	buildVersion = normalizeVersion(buildVersion)
	if buildVersion != "" && buildVersion != "(devel)" {
		return buildVersion
	}
	if linkVersion != "" {
		return linkVersion
	}
	return defaultVersion
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	return strings.TrimPrefix(v, "v")
}
