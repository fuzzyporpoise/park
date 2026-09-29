// Command park is a standalone parking-lot for markdown notes, organized
// as IPAA: Inbox / Projects / Areas / Archive, a PARA variant.

package main

import (
	_ "embed"
	"strings"

	"go.fuzzyporpoise.dev/park/cmd/park"
)

//go:embed VERSION
var versionFile string

// version and buildTime are set by GoReleaser via ldflags at build time.
var (
	version   string
	buildTime string
)

func main() {
	if version == "" {
		version = strings.TrimSpace(versionFile)
	}
	cmd.SetVersion(version)
	cmd.SetBuildTime(buildTime)
	cmd.Main()
}
