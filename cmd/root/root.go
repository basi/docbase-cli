package root

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Version information
var (
	Version   = "0.0.12"
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

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/docbase/config.json)")
	rootCmd.PersistentFlags().StringVar(&teamDomain, "team", "", "DocBase team domain")
	rootCmd.PersistentFlags().StringVar(&accessToken, "token", "", "DocBase API access token")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", "text", "Output format (text, json, yaml)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose output")

	// Bind flags to viper
	_ = viper.BindPFlag("team_domain", rootCmd.PersistentFlags().Lookup("team"))
	_ = viper.BindPFlag("access_token", rootCmd.PersistentFlags().Lookup("token"))
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
