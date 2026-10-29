package client

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/basi/docbase-cli/internal/broker"
	"github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/pkg/docbase"
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

// Create creates a DocBase API client. While DOCBASE_BROKER_SOCKET is set, the
// client talks to the broker, which holds the access token: no token is looked up
// or sent, and there is no fallback to direct access, not even when the variable
// is set but empty.
func Create(cmd *cobra.Command) (*docbase.API, error) {
	socket, active, err := config.BrokerMode()
	if err != nil {
		return nil, err
	}
	if active {
		api, _, err := CreateForBroker(cmd, socket)
		return api, err
	}

	teamDomain := config.GetTeamDomain(cmd.Flag("team").Value.String())

	if teamDomain == "" {
		return nil, fmt.Errorf("team domain is required")
	}

	accessToken, err := config.ResolveAccessToken(teamDomain, cmd.Flag("token").Value.String())
	if err != nil {
		return nil, err
	}

	return docbase.NewAPI(teamDomain, accessToken), nil
}

// CreateForBroker creates a DocBase API client that talks to the broker on the
// socket, and returns the team the broker serves. It fails when a token was also
// given, when the broker cannot be reached, or when --team names another team.
// It takes the result of config.BrokerMode.
func CreateForBroker(cmd *cobra.Command, socket string) (*docbase.API, string, error) {
	if socket == "" {
		return nil, "", fmt.Errorf("%s is set but empty", config.EnvBrokerSocket)
	}
	if cmd.Flag("token").Value.String() != "" || os.Getenv(config.EnvAPIToken) != "" {
		return nil, "", fmt.Errorf("%s cannot be combined with --token or %s", config.EnvBrokerSocket, config.EnvAPIToken)
	}

	info, err := broker.Status(socket)
	if err != nil {
		return nil, "", err
	}
	if team := cmd.Flag("team").Value.String(); team != "" && !strings.EqualFold(team, info.Team) {
		return nil, "", fmt.Errorf("--team %q does not match the team of the DocBase broker (%q)", team, info.Team)
	}

	api := docbase.NewAPI(info.Team, "")
	api.SetTransport(broker.NewTransport(socket))
	api.SetBaseURL(broker.BaseURL)

	return api, info.Team, nil
}
