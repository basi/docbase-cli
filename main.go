package main

import (
	"fmt"
	"os"

	// Import all commands
	_ "github.com/basi/docbase-cli/cmd/api"
	_ "github.com/basi/docbase-cli/cmd/auth"
	_ "github.com/basi/docbase-cli/cmd/comment"
	_ "github.com/basi/docbase-cli/cmd/completion"
	_ "github.com/basi/docbase-cli/cmd/config"
	_ "github.com/basi/docbase-cli/cmd/export"
	_ "github.com/basi/docbase-cli/cmd/group"
	// _ "github.com/basi/docbase-cli/cmd/import" // Temporarily disabled due to compilation issues
	_ "github.com/basi/docbase-cli/cmd/memo"
	"github.com/basi/docbase-cli/cmd/root"
	_ "github.com/basi/docbase-cli/cmd/tag"
	_ "github.com/basi/docbase-cli/cmd/version"
)

func main() {
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}