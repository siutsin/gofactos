// This file surfaces build metadata injected at link time via -ldflags.
package app

import (
	"fmt"
	"runtime"
)

// Build metadata, overridden at link time with -ldflags -X. The defaults keep
// unstamped builds such as `go run` and `go install` self-describing.
var (
	version   = "dev"
	buildTime = "unknown"
)

// versionString combines build metadata with the Go toolchain version and
// target platform for --version output.
func versionString() string {
	return fmt.Sprintf("%s (built %s, %s %s/%s)",
		version, buildTime, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
