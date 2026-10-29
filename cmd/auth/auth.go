package auth

import (
	"errors"
	"fmt"
	"os"

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
			if accessToken == "" {
				// Lets users keep the token out of argv and shell history
				accessToken = os.Getenv(config.EnvAPIToken)
			}

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
			if err := config.StoreAccessToken(teamDomain, accessToken); err != nil {
				return fmt.Errorf("failed to save access token: %w", err)
			}

			cfg := &config.Config{
				TeamDomain:   teamDomain,
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
			teamDomain := config.GetTeamDomain(cmd.Flag("team").Value.String())
			if teamDomain == "" {
				fmt.Println(color.YellowString("Not logged in to DocBase"))
				return nil
			}

			accessToken, err := config.ResolveAccessToken(teamDomain, cmd.Flag("token").Value.String())
			if err != nil {
				// Not having a token is the normal logged-out state; anything else is worth showing.
				if !errors.Is(err, config.ErrNoToken) {
					fmt.Fprintln(os.Stderr, err)
				}
				fmt.Println(color.YellowString("Not logged in to DocBase"))
				return nil
			}

			// Test the credentials
			api := docbase.NewAPI(teamDomain, accessToken)
			_, err = api.Group.List(1, 1)
			if err != nil {
				fmt.Println(color.YellowString("Logged in as team: %s, but authentication failed: %v", teamDomain, err))
				return nil
			}

			fmt.Println(color.GreenString("Logged in to DocBase as team: %s", teamDomain))
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

			teamDomain := config.GetTeamDomain(cmd.Flag("team").Value.String())

			// Remove the stored token first; this also wipes a plaintext token that has no team
			if err := config.DeleteAccessToken(teamDomain); err != nil {
				return fmt.Errorf("failed to remove access token: %w", err)
			}

			if teamDomain == "" {
				fmt.Println(color.YellowString("Not logged in to DocBase"))
				return nil
			}

			// Clear the credentials
			cfg.TeamDomain = ""

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println(color.GreenString("Successfully logged out from DocBase"))
			if os.Getenv(config.EnvAPIToken) != "" {
				fmt.Fprintf(os.Stderr, "note: %s is still set in the environment and will continue to be used\n", config.EnvAPIToken)
			}
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
