// Package cmdinfo carries the build-time identity of the binary.
package cmdinfo

// Name is the command name.
const Name = "block"

// Version is injected with -ldflags "-X github.com/nao1215/block/internal/cmdinfo.Version=v1.2.3".
var Version = "dev" //nolint:gochecknoglobals // set by the linker

// UserAgent identifies block to upstream servers.
func UserAgent() string { return Name + "/" + Version }
