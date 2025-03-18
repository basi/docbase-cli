package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the configuration
type Config struct {
	TeamDomain   string `mapstructure:"team_domain"`
	AccessToken  string `mapstructure:"access_token"`
	OutputFormat string `mapstructure:"output_format"`
	DefaultGroup string `mapstructure:"default_group"`
}

// Load loads the configuration from the config file
func Load() (*Config, error) {
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// Save saves the configuration to the config file
func Save(config *Config) error {
	viper.Set("team_domain", config.TeamDomain)
	viper.Set("access_token", config.AccessToken)
	viper.Set("output_format", config.OutputFormat)
	viper.Set("default_group", config.DefaultGroup)

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		// If no config file is used, create one
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		configDir := filepath.Join(home, ".config", "docbase")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		configFile = filepath.Join(configDir, "config.json")
		viper.SetConfigFile(configFile)
	}

	if err := viper.WriteConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create it
			if err := viper.SafeWriteConfig(); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}
		} else {
			return fmt.Errorf("failed to write config file: %w", err)
		}
	}

	return nil
}

// GetTeamDomain returns the team domain from the config or flags
func GetTeamDomain(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return viper.GetString("team_domain")
}

// GetAccessToken returns the access token from the config or flags
func GetAccessToken(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return viper.GetString("access_token")
}

// GetOutputFormat returns the output format from the config or flags
func GetOutputFormat(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	format := viper.GetString("output_format")
	if format == "" {
		return "text"
	}
	return format
}

// GetDefaultGroup returns the default group from the config
func GetDefaultGroup() string {
	return viper.GetString("default_group")
}