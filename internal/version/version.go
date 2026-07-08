package version

// Version is injected at build time with ldflags. The default is "dev" for local builds.
var Version = "dev"

// Commit records the source revision when injected by the release pipeline.
var Commit = "unknown"
