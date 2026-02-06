package config

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/internal/formatter"
)

var (
	// ConfigCmd represents the config command
	ConfigCmd = &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `Manage configuration for DocBase CLI.`,
	}

	// SetCmd represents the config set command
	SetCmd = &cobra.Command{
		Use:   "set",
		Short: "Set configuration values",
		Long: `Set configuration values for DocBase CLI.

Example:
  docbase config set --team your-team
  docbase config set --output-format json
  docbase config set --default-group "全員"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			teamDomain, _ := cmd.Flags().GetString("team")
			accessToken, _ := cmd.Flags().GetString("token")
			outputFormat, _ := cmd.Flags().GetString("output-format")
			defaultGroup, _ := cmd.Flags().GetString("default-group")

			if teamDomain != "" {
				cfg.TeamDomain = teamDomain
			}

			if accessToken != "" {
				cfg.AccessToken = accessToken
			}

			if outputFormat != "" {
				cfg.OutputFormat = outputFormat
			}

			if defaultGroup != "" {
				cfg.DefaultGroup = defaultGroup
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println(color.GreenString("Configuration updated successfully"))
			return nil
		},
	}

	// GetCmd represents the config get command
	GetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get configuration values",
		Long: `Get configuration values for DocBase CLI.

Example:
  docbase config get --key team_domain
  docbase config get --key output_format`,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, _ := cmd.Flags().GetString("key")
			if key == "" {
				return fmt.Errorf("key is required")
			}

			value := viper.GetString(key)
			if value == "" {
				fmt.Printf("%s: not set\n", key)
			} else {
				fmt.Printf("%s: %s\n", key, value)
			}

			return nil
		},
	}

	// ListCmd represents the config list command
	ListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		Long:  `List all configuration values for DocBase CLI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			outputFormat, _ := cmd.Flags().GetString("format")
			f := formatter.NewFormatter(outputFormat, os.Stdout, true)

			// Mask the access token for security
			configMap := map[string]any{
				"team_domain":   cfg.TeamDomain,
				"access_token":  "********",
				"output_format": cfg.OutputFormat,
				"default_group": cfg.DefaultGroup,
				"config_file":   viper.ConfigFileUsed(),
			}

			return f.Print(configMap)
		},
	}
)

func init() {
	// Add config command to root command
	root.AddCommand(ConfigCmd)

	// Add subcommands to config command
	ConfigCmd.AddCommand(SetCmd)
	ConfigCmd.AddCommand(GetCmd)
	ConfigCmd.AddCommand(ListCmd)

	// Add flags to set command
	SetCmd.Flags().String("team", "", "DocBase team domain")
	SetCmd.Flags().String("token", "", "DocBase API access token")
	SetCmd.Flags().String("output-format", "", "Output format (text, json, yaml)")
	SetCmd.Flags().String("default-group", "", "Default group")

	// Add flags to get command
	GetCmd.Flags().String("key", "", "Configuration key")
	_ = GetCmd.MarkFlagRequired("key")
}
