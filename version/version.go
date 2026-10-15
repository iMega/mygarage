// Package version holds build metadata injected via -ldflags.
package version

var (
	// SHA is the full Git commit the binary was built from.
	SHA = "dev"
	// BuildTime is the UTC build timestamp in RFC 3339.
	BuildTime = ""
)
