package version

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/basi/docbase-cli/cmd/root"
)

var (
	// VersionCmd represents the version command
	VersionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Long:  `Print the version and build information of DocBase CLI.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("DocBase CLI version %s\n", root.Version)
			fmt.Printf("Built at %s\n", root.BuildTime)
		},
	}
)

func init() {
	// Add version command to root command
	root.AddCommand(VersionCmd)
}
