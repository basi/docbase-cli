package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"

	internalconfig "github.com/basi/docbase-cli/internal/config"
	"github.com/basi/docbase-cli/internal/credential"
)

// TestMain swaps in an in-memory keyring so that no test can reach the
// developer's real credential store. The in-memory keyring needs no D-Bus
// session bus, which Linux CI runners lack, so the session bus check is bypassed.
func TestMain(m *testing.M) {
	keyring.MockInit()
	credential.SetBusCheck(func() bool { return true })
	os.Exit(m.Run())
}

// setup points the config at a file in a temporary directory and returns its path.
func setup(t *testing.T) string {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)
	configFile := filepath.Join(t.TempDir(), "config.json")
	viper.SetConfigFile(configFile)
	unsetEnv(t, internalconfig.EnvBrokerSocket)
	t.Setenv(internalconfig.EnvAPIToken, "")
	t.Setenv(internalconfig.EnvInsecureStorage, "")

	return configFile
}

// setFlag sets a flag of the set command for the test.
func setFlag(t *testing.T, name, value string) {
	t.Helper()

	if err := SetCmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Set(%s): %v", name, err)
	}
	t.Cleanup(func() {
		_ = SetCmd.Flags().Set(name, "")
		SetCmd.Flags().Lookup(name).Changed = false
	})
}

func TestSetTokenIsRefusedInBrokerMode(t *testing.T) {
	configFile := setup(t)
	t.Setenv(internalconfig.EnvBrokerSocket, "/run/docbase/broker.sock")
	keyring.MockInitWithError(errors.New("the credential store must not be used in broker mode"))
	t.Cleanup(keyring.MockInit)

	setFlag(t, "token", "s3cret-config-token")
	setFlag(t, "team", "example")

	err := SetCmd.RunE(SetCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "config set --token is not available while DOCBASE_BROKER_SOCKET is set") {
		t.Fatalf("got %v, want the broker mode error", err)
	}
	if strings.Contains(err.Error(), "s3cret-config-token") {
		t.Errorf("the error contains the token: %v", err)
	}
	// Nothing was saved, not even the other settings of the same command.
	if _, err := os.Stat(configFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the config file was written (Stat error: %v)", err)
	}
}

func TestSetTokenWithAnEmptyBrokerVariable(t *testing.T) {
	configFile := setup(t)
	t.Setenv(internalconfig.EnvBrokerSocket, "")
	keyring.MockInitWithError(errors.New("the credential store must not be used in broker mode"))
	t.Cleanup(keyring.MockInit)

	setFlag(t, "token", "s3cret-config-token")
	err := SetCmd.RunE(SetCmd, nil)
	if err == nil || err.Error() != "DOCBASE_BROKER_SOCKET is set but empty" {
		t.Fatalf("got %v, want the set-but-empty error", err)
	}
	if _, err := os.Stat(configFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the config file was written (Stat error: %v)", err)
	}
}

func TestSetOtherSettingsInBrokerMode(t *testing.T) {
	configFile := setup(t)
	t.Setenv(internalconfig.EnvBrokerSocket, "/run/docbase/broker.sock")
	keyring.MockInitWithError(errors.New("the credential store must not be used in broker mode"))
	t.Cleanup(keyring.MockInit)

	setFlag(t, "output-format", "json")
	if err := SetCmd.RunE(SetCmd, nil); err != nil {
		t.Fatalf("config set --output-format: %v", err)
	}
	data, err := os.ReadFile(configFile)
	if err != nil || !strings.Contains(string(data), `"output_format": "json"`) {
		t.Errorf("config file = %q, %v; want the output format saved", data, err)
	}
}

func TestSetTokenOutsideBrokerMode(t *testing.T) {
	setup(t)

	setFlag(t, "team", "example")
	setFlag(t, "token", "s3cret-config-token")
	if err := SetCmd.RunE(SetCmd, nil); err != nil {
		t.Fatalf("config set --token: %v", err)
	}

	// The token went to the (in-memory) credential store, as before.
	got, err := keyring.Get("docbase-cli", "example")
	if err != nil || got != "s3cret-config-token" {
		t.Errorf("credential store entry = %q, %v; want the token", got, err)
	}
}

// unsetEnv removes the variable for the test; the original value comes back afterwards.
func unsetEnv(t *testing.T, name string) {
	t.Helper()

	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
}
