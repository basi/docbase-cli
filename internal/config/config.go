package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the configuration.
// The access token is not part of it: see StoreAccessToken and ResolveAccessToken.
type Config struct {
	TeamDomain   string `mapstructure:"team_domain"   json:"team_domain"`
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

// Save saves the configuration to the config file.
// Keys it does not manage are kept, including a legacy plaintext access_token,
// which only the token functions in token.go remove.
func Save(config *Config) error {
	viper.Set("team_domain", config.TeamDomain)
	viper.Set("output_format", config.OutputFormat)
	viper.Set("default_group", config.DefaultGroup)

	configFile, err := configFileForWrite()
	if err != nil {
		return err
	}

	raw, err := readRawConfig(configFile)
	if err != nil {
		return err
	}

	raw["team_domain"] = config.TeamDomain
	raw["output_format"] = config.OutputFormat
	raw["default_group"] = config.DefaultGroup
	if token, ok := raw["access_token"].(string); ok && token == "" {
		delete(raw, "access_token")
	}

	return writeRawConfig(configFile, raw)
}

// configFileForWrite returns the config file path, registering the default
// location with viper when no config file is set.
func configFileForWrite() (string, error) {
	if configFile := viper.ConfigFileUsed(); configFile != "" {
		return configFile, nil
	}

	// If no config file is set, create one in the default location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "docbase")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	configFile := filepath.Join(configDir, "config.json")
	viper.SetConfigFile(configFile)

	return configFile, nil
}

// readRawConfig reads the config file as a raw JSON object so that keys unknown
// to Config survive a rewrite. A missing or blank file yields an empty object;
// a file that cannot be parsed is an error, so callers never overwrite it.
func readRawConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}

	// UseNumber keeps numbers in unknown keys exactly as written.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("failed to parse config file %s: unexpected data after the top-level JSON value", path)
	}
	if raw == nil {
		raw = map[string]any{}
	}

	return raw, nil
}

// writeRawConfig atomically replaces the config file with the given JSON object.
func writeRawConfig(path string, raw map[string]any) error {
	// Ensure parent directory exists (supports custom --config paths)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config with restrictive permissions (it may hold the access token)
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp config file: %w", err)
	}

	// On Unix, CreateTemp is 0600 by default, but enforce it for safety.
	_ = os.Chmod(tmpName, 0600)

	// os.Rename replaces an existing destination on both Unix and Windows.
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	_ = os.Chmod(path, 0600)

	return nil
}

// GetTeamDomain returns the team domain from the config or flags
func GetTeamDomain(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return viper.GetString("team_domain")
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

// DisplayValue returns the value of a configuration key for display.
// Only known keys are accepted, and the access token is always masked.
func DisplayValue(key string) (string, error) {
	switch normalized := strings.ToLower(key); normalized {
	case "access_token":
		return "********", nil
	case "team_domain", "output_format", "default_group":
		return viper.GetString(normalized), nil
	default:
		return "", fmt.Errorf("unknown configuration key %q (valid keys: team_domain, access_token, output_format, default_group)", key)
	}
}
