package version

var (
	// These are populated via -ldflags at build time.
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
