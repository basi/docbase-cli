package root

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/basi/docbase-cli/internal/config"
)

// helperEnv turns the test binary into a helper process that runs initConfig.
const helperEnv = "DOCBASE_ROOT_TEST_HELPER"

// homeVariable is the environment variable that os.UserHomeDir reads.
func homeVariable() string {
	switch runtime.GOOS {
	case "windows":
		return "USERPROFILE"
	case "plan9":
		return "home"
	default:
		return "HOME"
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

// noHome makes the home directory of the process unknown.
func noHome(t *testing.T) {
	t.Helper()

	t.Setenv(homeVariable(), "")
	if _, err := os.UserHomeDir(); err == nil {
		t.Skip("this platform knows a home directory without the environment")
	}
}

func resetViper(t *testing.T) {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)
}

// TestInitConfigHelper is not a test: the tests below run it in a child process.
func TestInitConfigHelper(t *testing.T) {
	if os.Getenv(helperEnv) == "" {
		t.Skip("helper process")
	}
	initConfig()
	_, _ = os.Stdout.WriteString("initConfig returned\n")
}

// A run that only has the socket of a broker may have no home directory, and
// needs no config file: the command must get to the client.
func TestInitConfigInBrokerModeWithoutAHomeDirectory(t *testing.T) {
	for name, value := range map[string]string{"socket set": "/run/docbase/broker.sock", "socket set but empty": ""} {
		t.Run(name, func(t *testing.T) {
			resetViper(t)
			noHome(t)
			t.Setenv(config.EnvBrokerSocket, value)

			initConfig() // exits the test binary if it fails

			if file := viper.ConfigFileUsed(); file != "" {
				t.Errorf("config file = %q, want none", file)
			}
		})
	}
}

// Outside the broker mode, a missing home directory is still fatal, as it was.
func TestInitConfigWithoutAHomeDirectoryExitsOutsideBrokerMode(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestInitConfigHelper$")
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if name == homeVariable() || name == config.EnvBrokerSocket {
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, helperEnv+"=1", homeVariable()+"=")

	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("the helper process ended with %v, want exit status 1; output:\n%s", err, out)
	}
	if !strings.Contains(string(out), "HOME") && !strings.Contains(strings.ToLower(string(out)), "home") {
		t.Errorf("output = %q, want the message about the home directory", out)
	}
	if strings.Contains(string(out), "initConfig returned") {
		t.Errorf("initConfig returned: %q", out)
	}
}

// In broker mode a home directory that exists is used as before.
func TestInitConfigInBrokerModeWithAHomeDirectory(t *testing.T) {
	resetViper(t)
	home := t.TempDir()
	t.Setenv(homeVariable(), home)
	t.Setenv(config.EnvBrokerSocket, "/run/docbase/broker.sock")

	initConfig()

	if info, err := os.Stat(filepath.Join(home, ".config", "docbase")); err != nil || !info.IsDir() {
		t.Errorf("the config directory was not made: %v", err)
	}
}

// A command that opts out is not given the config; any other is.
func TestPersistentPreRunSkipsTheConfigOfCommandsThatOptOut(t *testing.T) {
	skipping := &cobra.Command{Use: "skipping", Annotations: map[string]string{SkipConfigAnnotation: "true"}}
	ordinary := &cobra.Command{Use: "ordinary"}

	t.Run("opted out", func(t *testing.T) {
		resetViper(t)
		home := t.TempDir()
		t.Setenv(homeVariable(), home)
		unsetEnv(t, config.EnvBrokerSocket)

		rootCmd.PersistentPreRun(skipping, nil)

		if _, err := os.Stat(filepath.Join(home, ".config")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the config directory was made for a command that opted out (Stat error: %v)", err)
		}
	})

	t.Run("opted out without a home directory", func(t *testing.T) {
		resetViper(t)
		noHome(t)
		unsetEnv(t, config.EnvBrokerSocket)

		rootCmd.PersistentPreRun(skipping, nil) // exits the test binary if it reads the config
	})

	t.Run("ordinary command", func(t *testing.T) {
		resetViper(t)
		home := t.TempDir()
		t.Setenv(homeVariable(), home)
		unsetEnv(t, config.EnvBrokerSocket)

		rootCmd.PersistentPreRun(ordinary, nil)

		if info, err := os.Stat(filepath.Join(home, ".config", "docbase")); err != nil || !info.IsDir() {
			t.Errorf("the config directory was not made for an ordinary command: %v", err)
		}
	})
}
