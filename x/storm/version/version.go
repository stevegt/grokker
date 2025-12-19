package version

// Version is injected at build time via ldflags with the format:
// - "v0.1.5" if the current commit has a tag
// - "dev-abc1234" if no tag exists (where abc1234 is the short commit hash)
// Set via: -X github.com/stevegt/grokker/x/storm/version.Version=<version>
var Version = "dev-unknown"
