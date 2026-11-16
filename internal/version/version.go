// Package version holds the build's version, stamped in at link time.
package version

// Version is the release this binary was built as. The release build sets it
// with -ldflags "-X github.com/primandproper/docent/internal/version.Version=v1.2.3";
// anything else is a development build.
var Version = "dev"
