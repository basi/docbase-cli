package root

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/basi/docbase-cli/internal/config"
)

// Version information
var (
	Version   = "0.0.14"
	BuildTime = "unknown"
)

var (
	cfgFile      string
	teamDomain   string
	accessToken  string
	outputFormat string
	verbose      bool

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "docbase",
		Short: "DocBase CLI - Command line interface for DocBase",
		Long: `DocBase CLI is a command line interface for DocBase.
It provides various commands to interact with DocBase API.

Examples:
  # List memos
  docbase memo list

  # Search memos
  docbase memo search "keyword"

  # Create a memo
  docbase memo create --title "Test Memo" --body "This is a test memo" --group "Everyone"`,
		Version: Version,
	}
)

// SkipConfigAnnotation, set to "true" in the Annotations of a command, keeps the
// command from reading the config file (and from needing a home directory).
const SkipConfigAnnotation = "docbase/skip-config"

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Read the config before the command runs, unless the command opts out.
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if cmd.Annotations[SkipConfigAnnotation] != "true" {
			initConfig()
		}
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/docbase/config.json)")
	rootCmd.PersistentFlags().StringVar(&teamDomain, "team", "", "DocBase team domain")
	rootCmd.PersistentFlags().StringVar(&accessToken, "token", "", "DocBase API access token (prefer the DOCBASE_API_TOKEN environment variable)")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", "text", "Output format (text, json, yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")

	// Bind flags to viper
	_ = viper.BindPFlag("team_domain", rootCmd.PersistentFlags().Lookup("team"))
	_ = viper.BindPFlag("output_format", rootCmd.PersistentFlags().Lookup("format"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		if err != nil {
			if _, brokerMode, _ := config.BrokerMode(); brokerMode {
				// A run that only has the socket of a broker has no use for a config
				// file, and may well have no home directory: do not look for one.
				viper.AutomaticEnv()
				return
			}
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".config/docbase" (without extension).
		configDir := filepath.Join(home, ".config", "docbase")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			fmt.Println("Error creating config directory:", err)
			os.Exit(1)
		}

		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("json")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		// No config file is fine; parse/permission errors should be surfaced.
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintln(os.Stderr, "Error reading config file:", err)
			os.Exit(1)
		}
	} else if verbose {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

// AddCommand adds a command to the root command
func AddCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}
