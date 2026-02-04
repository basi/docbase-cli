package client

import (
	"fmt"

	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/pkg/docbase"
	"github.com/spf13/cobra"
)

// CheckRequiredFlags checks if the required flags are set
func CheckRequiredFlags(cmd *cobra.Command, flagNames ...string) error {
	for _, flagName := range flagNames {
		flag := cmd.Flag(flagName)
		if flag == nil {
			return fmt.Errorf("flag %s not found", flagName)
		}
		if flag.Value.String() == "" {
			return fmt.Errorf("required flag %s not set", flagName)
		}
	}
	return nil
}

// Create creates a DocBase API client
func Create(cmd *cobra.Command) (*docbase.API, error) {
	teamDomain := config.GetTeamDomain(cmd.Flag("team").Value.String())
	accessToken := config.GetAccessToken(cmd.Flag("token").Value.String())

	if teamDomain == "" {
		return nil, fmt.Errorf("team domain is required")
	}

	if accessToken == "" {
		return nil, fmt.Errorf("access token is required")
	}

	return docbase.NewAPI(teamDomain, accessToken), nil
}
