package auth

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/pkg/docbase"
)

var (
	// AuthCmd represents the auth command
	AuthCmd = &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication",
		Long:  `Manage authentication with DocBase API.`,
	}

	// LoginCmd represents the auth login command
	LoginCmd = &cobra.Command{
		Use:   "login",
		Short: "Login to DocBase",
		Long: `Login to DocBase using an access token.

You can generate an access token from the DocBase settings page:
https://[your-team].docbase.io/settings/tokens

Example:
  docbase auth login --team your-team --token your-access-token`,
		RunE: func(cmd *cobra.Command, args []string) error {
			teamDomain, _ := cmd.Flags().GetString("team")
			accessToken, _ := cmd.Flags().GetString("token")

			if teamDomain == "" {
				return fmt.Errorf("team domain is required")
			}

			if accessToken == "" {
				return fmt.Errorf("access token is required")
			}

			// Test the credentials
			api := docbase.NewAPI(teamDomain, accessToken)
			_, err := api.Group.List(1, 1)
			if err != nil {
				return fmt.Errorf("failed to authenticate: %w", err)
			}

			// Save the credentials
			cfg := &config.Config{
				TeamDomain:   teamDomain,
				AccessToken:  accessToken,
				OutputFormat: "text",
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println(color.GreenString("Successfully logged in to DocBase as team: %s", teamDomain))
			return nil
		},
	}

	// StatusCmd represents the auth status command
	StatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long:  `Show authentication status with DocBase API.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.TeamDomain == "" || cfg.AccessToken == "" {
				fmt.Println(color.YellowString("Not logged in to DocBase"))
				return nil
			}

			// Test the credentials
			api := docbase.NewAPI(cfg.TeamDomain, cfg.AccessToken)
			_, err = api.Group.List(1, 1)
			if err != nil {
				fmt.Println(color.YellowString("Logged in as team: %s, but authentication failed: %v", cfg.TeamDomain, err))
				return nil
			}

			fmt.Println(color.GreenString("Logged in to DocBase as team: %s", cfg.TeamDomain))
			return nil
		},
	}

	// LogoutCmd represents the auth logout command
	LogoutCmd = &cobra.Command{
		Use:   "logout",
		Short: "Logout from DocBase",
		Long:  `Logout from DocBase by removing the saved credentials.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.TeamDomain == "" || cfg.AccessToken == "" {
				fmt.Println(color.YellowString("Not logged in to DocBase"))
				return nil
			}

			// Clear the credentials
			cfg.TeamDomain = ""
			cfg.AccessToken = ""

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println(color.GreenString("Successfully logged out from DocBase"))
			return nil
		},
	}
)

func init() {
	// Add auth command to root command
	root.AddCommand(AuthCmd)

	// Add subcommands to auth command
	AuthCmd.AddCommand(LoginCmd)
	AuthCmd.AddCommand(StatusCmd)
	AuthCmd.AddCommand(LogoutCmd)

	// Add flags to login command
	LoginCmd.Flags().String("team", "", "DocBase team domain")
	LoginCmd.Flags().String("token", "", "DocBase API access token")
}
