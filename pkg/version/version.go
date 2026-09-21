package version

// Version is set at build time from git tags (see Makefile).
// Falls back to dev for plain `go build`.
var Version = "dev"
