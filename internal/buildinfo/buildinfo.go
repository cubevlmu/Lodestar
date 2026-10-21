package buildinfo

// Version is the Lodestar release version. Set at build time with
// -ldflags "-X lodestar/internal/buildinfo.Version=x.y.z".
var Version = "0.1.0"

// Commit is the git revision injected at build time.
var Commit = "dev"

// Author is the project author displayed in version output and UI footer.
var Author = "cubevlmu"
