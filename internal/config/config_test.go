package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"

	"github.com/basi/docbase-cli/internal/credential"
)

const (
	// keyringService is the service name the credential package stores tokens under.
	keyringService = "docbase-cli"

	// Distinctive token values, so that a leak into an error or warning is easy to spot.
	legacyToken = "s3cret-legacy-token"
	newToken    = "s3cret-new-token"
)

// TestMain swaps in an in-memory keyring so that no test can reach the
// developer's real credential store. The in-memory keyring needs no D-Bus
// session bus, which Linux CI runners lack, so the session bus check is bypassed.
func TestMain(m *testing.M) {
	keyring.MockInit()
	credential.SetBusCheck(func() bool { return true })
	os.Exit(m.Run())
}

// setup resets the global state the tests share (viper, the keyring, the token
// related environment variables and the warning output). It returns the path of
// a config file registered with viper, which does not exist yet, and the buffer
// that receives warnings and notices.
func setup(t *testing.T) (string, *bytes.Buffer) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)
	configFile := filepath.Join(t.TempDir(), "config.json")
	viper.SetConfigFile(configFile)

	keyring.MockInit()
	for _, name := range []string{EnvAPIToken, EnvInsecureStorage, legacyEnvToken} {
		t.Setenv(name, "")
	}

	warnings := &bytes.Buffer{}
	original := warnOutput
	warnOutput = warnings
	t.Cleanup(func() { warnOutput = original })

	return configFile, warnings
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return raw
}

// checkMode0600 verifies the file mode; file modes are not meaningful on Windows.
func checkMode0600(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("file mode: got %o, want 600", got)
	}
}

// TestSaveWritesSnakeCaseKeys verifies that Save writes JSON keys in snake_case
// and never writes an access token.
func TestSaveWritesSnakeCaseKeys(t *testing.T) {
	configFile, _ := setup(t)

	if err := Save(&Config{TeamDomain: "example"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw := readJSON(t, configFile)
	for _, key := range []string{"team_domain", "output_format", "default_group"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("key %q not found (got: %v)", key, raw)
		}
	}
	if _, ok := raw["access_token"]; ok {
		t.Errorf("access_token must not be written (got: %v)", raw)
	}
}

// TestLoadReadsSnakeCaseKeys verifies that Load correctly reads snake_case keys
// from config.json into the Config struct, and tolerates a legacy access_token.
func TestLoadReadsSnakeCaseKeys(t *testing.T) {
	configFile, _ := setup(t)

	// Write a config.json with snake_case keys directly
	content := `{"team_domain":"example","access_token":"token123","output_format":"text","default_group":"mygroup"}`
	writeFile(t, configFile, content)

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
	if got.OutputFormat != "text" {
		t.Errorf("OutputFormat: got %q, want %q", got.OutputFormat, "text")
	}
	if got.DefaultGroup != "mygroup" {
		t.Errorf("DefaultGroup: got %q, want %q", got.DefaultGroup, "mygroup")
	}
}

func TestSave(t *testing.T) {
	t.Run("keeps a legacy token and unknown keys", func(t *testing.T) {
		configFile, _ := setup(t)
		writeFile(t, configFile, `{"team_domain":"old","access_token":"`+legacyToken+`","extra":{"a":[1,2]},"big":12345678901234567890}`)
		// Start wider than 0600 to show that Save tightens the mode.
		if err := os.Chmod(configFile, 0644); err != nil {
			t.Fatalf("Chmod: %v", err)
		}

		if err := Save(&Config{TeamDomain: "example", OutputFormat: "json", DefaultGroup: "g"}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		raw := readJSON(t, configFile)
		want := map[string]any{
			"team_domain":   "example",
			"output_format": "json",
			"default_group": "g",
			"access_token":  legacyToken,
		}
		for key, value := range want {
			if raw[key] != value {
				t.Errorf("%s: got %v, want %v", key, raw[key], value)
			}
		}
		if _, ok := raw["extra"]; !ok {
			t.Errorf("unknown key %q was dropped (got: %v)", "extra", raw)
		}
		if !strings.Contains(readFile(t, configFile), "12345678901234567890") {
			t.Errorf("number in an unknown key was altered:\n%s", readFile(t, configFile))
		}
		checkMode0600(t, configFile)
	})

	t.Run("drops an empty access_token", func(t *testing.T) {
		configFile, _ := setup(t)
		writeFile(t, configFile, `{"team_domain":"example","access_token":""}`)

		if err := Save(&Config{TeamDomain: "example"}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		if raw := readJSON(t, configFile); raw["access_token"] != nil {
			t.Errorf("empty access_token was kept (got: %v)", raw)
		}
	})

	t.Run("writes a fresh file and directory without a token", func(t *testing.T) {
		configFile, _ := setup(t)
		configFile = filepath.Join(filepath.Dir(configFile), "nested", "config.json")
		viper.SetConfigFile(configFile)

		if err := Save(&Config{TeamDomain: "example"}); err != nil {
			t.Fatalf("Save: %v", err)
		}

		raw := readJSON(t, configFile)
		for _, key := range []string{"team_domain", "output_format", "default_group"} {
			if _, ok := raw[key]; !ok {
				t.Errorf("key %q not found (got: %v)", key, raw)
			}
		}
		if _, ok := raw["access_token"]; ok {
			t.Errorf("access_token must not be written (got: %v)", raw)
		}
		checkMode0600(t, configFile)

		entries, err := os.ReadDir(filepath.Dir(configFile))
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("temp files left behind: %v", entries)
		}
	})

	t.Run("leaves a file it cannot parse untouched", func(t *testing.T) {
		configFile, _ := setup(t)
		content := `{"team_domain":"old","access_token":"` + legacyToken + `",`
		writeFile(t, configFile, content)

		if err := Save(&Config{TeamDomain: "example"}); err == nil {
			t.Fatal("Save: expected an error for invalid JSON")
		}

		if got := readFile(t, configFile); got != content {
			t.Errorf("file was modified:\n got: %s\nwant: %s", got, content)
		}
	})
}

func TestReadRawConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string // ignored when missing is set
		missing bool
		wantLen int
		wantErr bool
	}{
		{name: "missing file", missing: true},
		{name: "empty file", content: ""},
		{name: "blank file", content: " \n\t "},
		{name: "null", content: "null"},
		{name: "object", content: `{"a":1,"b":"x"}`, wantLen: 2},
		{name: "invalid JSON", content: `{"a":`, wantErr: true},
		{name: "not an object", content: `[1]`, wantErr: true},
		{name: "trailing data", content: `{"a":1} x`, wantErr: true},
		{name: "stray closing brace", content: `{"a":1}}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile, _ := setup(t)
			if !tt.missing {
				writeFile(t, configFile, tt.content)
			}

			raw, err := readRawConfig(configFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readRawConfig: error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			// Callers write into the map, so it must never be nil.
			if raw == nil {
				t.Fatal("readRawConfig returned a nil map")
			}
			if len(raw) != tt.wantLen {
				t.Errorf("readRawConfig: got %v, want %d keys", raw, tt.wantLen)
			}
		})
	}
}

func TestStoreAccessToken(t *testing.T) {
	t.Run("stores under the lowercased team and drops the legacy plaintext", func(t *testing.T) {
		configFile, warnings := setup(t)
		writeFile(t, configFile, `{"team_domain":"Example","access_token":"`+legacyToken+`","default_group":"g"}`)

		if err := StoreAccessToken(" Example ", newToken); err != nil {
			t.Fatalf("StoreAccessToken: %v", err)
		}

		got, err := keyring.Get(keyringService, "example")
		if err != nil || got != newToken {
			t.Errorf("keyring entry: got %q, %v; want %q", got, err, newToken)
		}
		raw := readJSON(t, configFile)
		if _, ok := raw["access_token"]; ok {
			t.Errorf("plaintext access_token is still in the file: %v", raw)
		}
		if raw["default_group"] != "g" {
			t.Errorf("other keys must survive (got: %v)", raw)
		}
		if warnings.Len() != 0 {
			t.Errorf("unexpected warnings: %q", warnings)
		}
	})

	t.Run("does not create the config file", func(t *testing.T) {
		configFile, _ := setup(t)

		if err := StoreAccessToken("example", newToken); err != nil {
			t.Fatalf("StoreAccessToken: %v", err)
		}

		if _, err := os.Stat(configFile); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("config file must not be created, Stat error = %v", err)
		}
	})

	t.Run("requires a team", func(t *testing.T) {
		setup(t)

		err := StoreAccessToken("  ", newToken)
		if err == nil || !strings.Contains(err.Error(), "--team") {
			t.Errorf("got %v, want an error asking for --team", err)
		}
	})
}

func TestStoreAccessTokenKeyringError(t *testing.T) {
	configFile, _ := setup(t)
	content := `{"team_domain":"example","access_token":"` + legacyToken + `"}`
	writeFile(t, configFile, content)
	keyring.MockInitWithError(errors.New("store unavailable"))

	err := StoreAccessToken("example", newToken)
	if err == nil {
		t.Fatal("StoreAccessToken: expected an error")
	}
	for _, want := range []string{"store unavailable", EnvAPIToken, EnvInsecureStorage} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), newToken) {
		t.Errorf("error leaks the token: %q", err)
	}
	if got := readFile(t, configFile); got != content {
		t.Errorf("file was modified:\n got: %s\nwant: %s", got, content)
	}
}

func TestInsecureStorage(t *testing.T) {
	t.Run("config file round trip without the keyring", func(t *testing.T) {
		configFile, warnings := setup(t)
		t.Setenv(EnvInsecureStorage, "1")
		keyring.MockInitWithError(errors.New("the keyring must not be used"))

		if err := StoreAccessToken("example", newToken); err != nil {
			t.Fatalf("StoreAccessToken: %v", err)
		}
		if raw := readJSON(t, configFile); raw["access_token"] != newToken {
			t.Errorf("access_token: got %v, want %q", raw["access_token"], newToken)
		}
		checkMode0600(t, configFile)
		if got := warnings.String(); !strings.Contains(got, EnvInsecureStorage) || !strings.Contains(got, configFile) || strings.Contains(got, newToken) {
			t.Errorf("unexpected warning: %q", got)
		}

		got, err := ResolveAccessToken("example", "")
		if err != nil || got != newToken {
			t.Errorf("ResolveAccessToken: got %q, %v; want %q", got, err, newToken)
		}

		// Saving other settings keeps the token.
		if err := Save(&Config{TeamDomain: "example"}); err != nil {
			t.Fatalf("Save: %v", err)
		}
		if raw := readJSON(t, configFile); raw["access_token"] != newToken {
			t.Errorf("Save dropped the token: %v", raw)
		}

		if err := DeleteAccessToken("example"); err != nil {
			t.Fatalf("DeleteAccessToken: %v", err)
		}
		if _, err := ResolveAccessToken("example", ""); !errors.Is(err, ErrNoToken) {
			t.Errorf("after delete: got %v, want ErrNoToken", err)
		}
	})

	t.Run("values of the environment variable", func(t *testing.T) {
		tests := []struct {
			value string
			want  bool
		}{
			{"1", true},
			{"true", true},
			{"TRUE", true},
			{"0", false},
			{"false", false},
			{"", false},
			{"yes", false},
			{"maybe", false},
		}
		for _, tt := range tests {
			t.Setenv(EnvInsecureStorage, tt.value)
			if got := InsecureStorage(); got != tt.want {
				t.Errorf("InsecureStorage() with %q = %v, want %v", tt.value, got, tt.want)
			}
		}
	})
}

func TestResolveMigratesLegacyToken(t *testing.T) {
	configFile, warnings := setup(t)
	writeFile(t, configFile, `{"team_domain":"Example","access_token":"`+legacyToken+`","default_group":"g"}`)

	got, err := ResolveAccessToken("example", "")
	if err != nil || got != legacyToken {
		t.Fatalf("ResolveAccessToken: got %q, %v; want %q", got, err, legacyToken)
	}

	stored, err := keyring.Get(keyringService, "example")
	if err != nil || stored != legacyToken {
		t.Errorf("keyring entry: got %q, %v; want %q", stored, err, legacyToken)
	}
	raw := readJSON(t, configFile)
	if _, ok := raw["access_token"]; ok {
		t.Errorf("plaintext access_token is still in the file: %v", raw)
	}
	if raw["default_group"] != "g" || raw["team_domain"] != "Example" {
		t.Errorf("other keys must survive (got: %v)", raw)
	}
	if got := warnings.String(); !strings.Contains(got, "moved the access token") || strings.Contains(got, legacyToken) {
		t.Errorf("unexpected notice: %q", got)
	}

	// The next run finds the token in the keyring and has nothing to report.
	warnings.Reset()
	got, err = ResolveAccessToken("example", "")
	if err != nil || got != legacyToken {
		t.Errorf("second ResolveAccessToken: got %q, %v; want %q", got, err, legacyToken)
	}
	if warnings.Len() != 0 {
		t.Errorf("unexpected warnings: %q", warnings)
	}
}

func TestResolveKeepsPlaintextWhenMigrationFails(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		team        string
		wantToken   string // empty: the plaintext token must not be returned
		wantWarning string
	}{
		{
			name:        "same team",
			content:     `{"team_domain":"Example","access_token":"` + legacyToken + `"}`,
			team:        "example",
			wantToken:   legacyToken,
			wantWarning: "could not move the access token",
		},
		{
			name:        "no team in the file",
			content:     `{"access_token":"` + legacyToken + `"}`,
			team:        "example",
			wantToken:   legacyToken,
			wantWarning: "no team_domain",
		},
		{
			name:        "another team",
			content:     `{"team_domain":"Example","access_token":"` + legacyToken + `"}`,
			team:        "other",
			wantWarning: "could not move the access token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configFile, warnings := setup(t)
			writeFile(t, configFile, tt.content)
			keyring.MockInitWithError(errors.New("store unavailable"))

			got, err := ResolveAccessToken(tt.team, "")
			if tt.wantToken != "" {
				if err != nil || got != tt.wantToken {
					t.Errorf("ResolveAccessToken: got %q, %v; want %q", got, err, tt.wantToken)
				}
			} else if err == nil || got != "" {
				t.Errorf("ResolveAccessToken: got %q, %v; want an error and no token", got, err)
			}

			if got := readFile(t, configFile); got != tt.content {
				t.Errorf("file was modified:\n got: %s\nwant: %s", got, tt.content)
			}
			if got := warnings.String(); !strings.Contains(got, tt.wantWarning) || strings.Contains(got, legacyToken) {
				t.Errorf("warning: got %q, want it to mention %q and not the token", got, tt.wantWarning)
			}
		})
	}
}

func TestResolveAccessTokenPriority(t *testing.T) {
	setup(t)
	if err := keyring.Set(keyringService, "example", "from-keyring"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}

	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{name: "keyring only", want: "from-keyring"},
		{name: "environment over keyring", env: "from-env", want: "from-env"},
		{name: "flag over environment", flag: "from-flag", env: "from-env", want: "from-flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvAPIToken, tt.env)

			got, err := ResolveAccessToken("example", tt.flag)
			if err != nil || got != tt.want {
				t.Errorf("ResolveAccessToken: got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestResolveAccessTokenErrors(t *testing.T) {
	t.Run("ACCESS_TOKEN is not a token", func(t *testing.T) {
		setup(t)
		viper.AutomaticEnv() // as the CLI does
		t.Setenv(legacyEnvToken, "generic-env-token")

		got, err := ResolveAccessToken("example", "")
		if got != "" || !errors.Is(err, ErrNoToken) {
			t.Fatalf("ResolveAccessToken: got %q, %v; want no token and ErrNoToken", got, err)
		}
		for _, want := range []string{EnvAPIToken, legacyEnvToken, "example"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "generic-env-token") {
			t.Errorf("error leaks the environment value: %q", err)
		}
	})

	t.Run("no token anywhere", func(t *testing.T) {
		setup(t)

		_, err := ResolveAccessToken("example", "")
		if !errors.Is(err, ErrNoToken) {
			t.Fatalf("got %v, want ErrNoToken", err)
		}
		if strings.Contains(err.Error(), "no longer read") {
			t.Errorf("error mentions ACCESS_TOKEN although it is not set: %q", err)
		}
	})

	t.Run("requires a team", func(t *testing.T) {
		setup(t)

		_, err := ResolveAccessToken(" ", "")
		if err == nil || errors.Is(err, ErrNoToken) || !strings.Contains(err.Error(), "team domain is required") {
			t.Errorf("got %v, want the team domain error", err)
		}
	})

	t.Run("credential store failure", func(t *testing.T) {
		setup(t)
		keyring.MockInitWithError(errors.New("store unavailable"))

		_, err := ResolveAccessToken("example", "")
		if err == nil || errors.Is(err, ErrNoToken) {
			t.Fatalf("got %v, want the store error", err)
		}
		for _, want := range []string{"store unavailable", EnvAPIToken, EnvInsecureStorage} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})
}

func TestDeleteAccessToken(t *testing.T) {
	t.Run("removes the plaintext and the keyring entry", func(t *testing.T) {
		configFile, _ := setup(t)
		writeFile(t, configFile, `{"team_domain":"example","access_token":"`+legacyToken+`","default_group":"g"}`)
		if err := keyring.Set(keyringService, "example", newToken); err != nil {
			t.Fatalf("keyring.Set: %v", err)
		}

		if err := DeleteAccessToken("Example"); err != nil {
			t.Fatalf("DeleteAccessToken: %v", err)
		}
		raw := readJSON(t, configFile)
		if _, ok := raw["access_token"]; ok {
			t.Errorf("plaintext access_token is still in the file: %v", raw)
		}
		if raw["default_group"] != "g" {
			t.Errorf("other keys must survive (got: %v)", raw)
		}
		if _, err := keyring.Get(keyringService, "example"); !errors.Is(err, keyring.ErrNotFound) {
			t.Errorf("keyring entry: Get error = %v, want ErrNotFound", err)
		}

		// Calling it again, with nothing left to remove, is fine.
		if err := DeleteAccessToken("Example"); err != nil {
			t.Errorf("second DeleteAccessToken: %v", err)
		}
	})

	t.Run("without a team only the plaintext goes", func(t *testing.T) {
		configFile, _ := setup(t)
		writeFile(t, configFile, `{"access_token":"`+legacyToken+`"}`)
		if err := keyring.Set(keyringService, "example", newToken); err != nil {
			t.Fatalf("keyring.Set: %v", err)
		}

		if err := DeleteAccessToken(""); err != nil {
			t.Fatalf("DeleteAccessToken: %v", err)
		}
		if raw := readJSON(t, configFile); len(raw) != 0 {
			t.Errorf("plaintext access_token is still in the file: %v", raw)
		}
		if got, err := keyring.Get(keyringService, "example"); err != nil || got != newToken {
			t.Errorf("keyring entry: got %q, %v; want it untouched", got, err)
		}
	})

	t.Run("does not create the config file", func(t *testing.T) {
		configFile, _ := setup(t)

		if err := DeleteAccessToken("example"); err != nil {
			t.Fatalf("DeleteAccessToken: %v", err)
		}
		if _, err := os.Stat(configFile); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("config file must not be created, Stat error = %v", err)
		}
	})
}

func TestDisplayValue(t *testing.T) {
	setup(t)
	viper.AutomaticEnv() // as the CLI does
	t.Setenv(legacyEnvToken, "generic-env-token")
	viper.Set("team_domain", "example")
	viper.Set("output_format", "json")
	viper.Set("default_group", "g")
	viper.Set("access_token", legacyToken)

	tests := []struct {
		key     string
		want    string
		wantErr bool
	}{
		{key: "team_domain", want: "example"},
		{key: "TEAM_DOMAIN", want: "example"},
		{key: "output_format", want: "json"},
		{key: "default_group", want: "g"},
		{key: "access_token", want: "********"},
		{key: "ACCESS_TOKEN", want: "********"},
		{key: "PATH", wantErr: true},
		{key: "verbose", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := DisplayValue(tt.key)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DisplayValue(%q): error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("DisplayValue(%q) = %q, want %q", tt.key, got, tt.want)
			}
			if err != nil && !strings.Contains(err.Error(), "valid keys") {
				t.Errorf("error %q does not list the valid keys", err)
			}
		})
	}
}

func TestBrokerMode(t *testing.T) {
	t.Run("not set", func(t *testing.T) {
		unsetEnv(t, EnvBrokerSocket)

		socket, active, err := BrokerMode()
		if socket != "" || active || err != nil {
			t.Errorf("BrokerMode() = %q, %v, %v; want no broker mode", socket, active, err)
		}
		if err := RejectInBrokerMode("auth login"); err != nil {
			t.Errorf("RejectInBrokerMode outside the broker mode = %v, want nil", err)
		}
	})

	t.Run("set", func(t *testing.T) {
		t.Setenv(EnvBrokerSocket, "/run/docbase/broker.sock")

		socket, active, err := BrokerMode()
		if socket != "/run/docbase/broker.sock" || !active || err != nil {
			t.Errorf("BrokerMode() = %q, %v, %v; want the socket", socket, active, err)
		}
		err = RejectInBrokerMode("auth login")
		if err == nil || !strings.Contains(err.Error(), "auth login") || !strings.Contains(err.Error(), EnvBrokerSocket) {
			t.Errorf("RejectInBrokerMode in the broker mode = %v, want an error naming the action and %s", err, EnvBrokerSocket)
		}
	})

	t.Run("set but empty is an error, not the absence of a broker", func(t *testing.T) {
		t.Setenv(EnvBrokerSocket, "")

		socket, active, err := BrokerMode()
		if socket != "" || !active || err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" {
			t.Errorf("BrokerMode() = %q, %v, %v; want the broker mode with the set-but-empty error", socket, active, err)
		}
		if err := RejectInBrokerMode("auth login"); err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" {
			t.Errorf("RejectInBrokerMode = %v, want the set-but-empty error", err)
		}
	})
}

// unsetEnv removes the variable for the test; the original value comes back afterwards.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
}
