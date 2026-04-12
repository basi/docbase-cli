package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// TestSaveWritesSnakeCaseKeys verifies that Save writes JSON keys in snake_case.
func TestSaveWritesSnakeCaseKeys(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")

	viper.Reset()
	viper.SetConfigFile(configFile)
	if err := Save(&Config{TeamDomain: "example", AccessToken: "token123"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	for _, key := range []string{"team_domain", "access_token", "output_format", "default_group"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("key %q not found (got: %v)", key, raw)
		}
	}
}

// TestLoadReadsSnakeCaseKeys verifies that Load correctly reads snake_case keys
// from config.json into the Config struct.
func TestLoadReadsSnakeCaseKeys(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.json")

	// Write a config.json with snake_case keys directly
	content := `{"team_domain":"example","access_token":"token123","output_format":"text","default_group":"mygroup"}`
	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	viper.Reset()
	viper.SetConfigFile(configFile)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("ReadInConfig: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.TeamDomain != "example" {
		t.Errorf("TeamDomain: got %q, want %q", got.TeamDomain, "example")
	}
	if got.AccessToken != "token123" {
		t.Errorf("AccessToken: got %q, want %q", got.AccessToken, "token123")
	}
	if got.OutputFormat != "text" {
		t.Errorf("OutputFormat: got %q, want %q", got.OutputFormat, "text")
	}
	if got.DefaultGroup != "mygroup" {
		t.Errorf("DefaultGroup: got %q, want %q", got.DefaultGroup, "mygroup")
	}
}
