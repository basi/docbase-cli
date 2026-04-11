package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the configuration
type Config struct {
	TeamDomain   string `mapstructure:"team_domain"   json:"team_domain"`
	AccessToken  string `mapstructure:"access_token"  json:"access_token"`
	OutputFormat string `mapstructure:"output_format" json:"output_format"`
	DefaultGroup string `mapstructure:"default_group" json:"default_group"`
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
		// If no config file is set, create one in the default location
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		configDir := filepath.Join(home, ".config", "docbase")
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		configFile = filepath.Join(configDir, "config.json")
		viper.SetConfigFile(configFile)
	}

	// Ensure parent directory exists (supports custom --config paths)
	if err := os.MkdirAll(filepath.Dir(configFile), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config with restrictive permissions (access token is included)
	bytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	bytes = append(bytes, '\n')

	tmpFile, err := os.CreateTemp(filepath.Dir(configFile), filepath.Base(configFile)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmpFile.Write(bytes); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file: %w", err)
	}

	// On Unix, CreateTemp is 0600 by default, but enforce it for safety.
	_ = os.Chmod(tmpName, 0600)

	// Atomic replace on Unix; on Windows, rename fails if the destination exists.
	if err := os.Rename(tmpName, configFile); err != nil {
		_ = os.Remove(configFile)
		if err2 := os.Rename(tmpName, configFile); err2 != nil {
			return fmt.Errorf("failed to replace config file: %w", err2)
		}
	}

	_ = os.Chmod(configFile, 0600)

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
