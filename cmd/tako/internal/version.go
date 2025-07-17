package internal

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of Tako",
		Long:  `All software has versions. This is Tako's`,
		Run: func(cmd *cobra.Command, args []string) {
			v, err := deriveVersion()
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), err)
				return
			}
			fmt.Fprintln(cmd.OutOrStdout(), v)
		},
	}
}

func deriveVersion() (string, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("could not read build info")
	}
	return deriveVersionFromInfo(info)
}

func deriveVersionFromInfo(info *debug.BuildInfo) (string, error) {
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version, nil
	}

	return derivePseudoVersionFromVCS(info)
}

// derivePseudoVersionFromVCS produces a pseudo version based on VCS tags,
// as described at https://go.dev/ref/mod#pseudo-versions
func derivePseudoVersionFromVCS(info *debug.BuildInfo) (string, error) {
	var revision, at string
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			revision = s.Value
		}
		if s.Key == "vcs.time" {
			at = s.Value
		}
	}

	if revision == "" && at == "" {
		return "", fmt.Errorf("version information is not available")
	}

	buf := strings.Builder{}
	buf.WriteString("v0.0.0-")
	if at != "" {
		// the commit time is of the form 2023-01-25T19:57:54Z
		p, err := time.Parse(time.RFC3339, at)
		if err == nil {
			buf.WriteString(p.Format("20060102150405"))
			buf.WriteString("-")
		}
	}
	if revision != "" {
		buf.WriteString(revision[:12])
	}
	return buf.String(), nil
}
