package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"

	"github.com/basi/docbase-cli/internal/credential"
)

const (
	// EnvAPIToken supplies the access token for a run without saving it anywhere.
	EnvAPIToken = "DOCBASE_API_TOKEN"
	// EnvInsecureStorage, when true, keeps the access token in the config file
	// instead of the OS credential store.
	EnvInsecureStorage = "DOCBASE_INSECURE_STORAGE"
	// legacyEnvToken was picked up through viper.AutomaticEnv() by older
	// versions; it is no longer read as a token.
	legacyEnvToken = "ACCESS_TOKEN"
)

// warnOutput receives warnings and notices (never the token). Tests replace it.
var warnOutput io.Writer = os.Stderr

// ErrNoToken is returned when no access token is available.
var ErrNoToken = errors.New("access token is required")

// InsecureStorage reports whether the access token is kept in the config file
// instead of the OS credential store (DOCBASE_INSECURE_STORAGE is a true value).
func InsecureStorage() bool {
	insecure, err := strconv.ParseBool(os.Getenv(EnvInsecureStorage))
	return err == nil && insecure
}

// ResolveAccessToken returns the access token to use, in this order: the --token
// flag value, the DOCBASE_API_TOKEN environment variable, then the stored
// credential (the OS credential store, or the config file when
// DOCBASE_INSECURE_STORAGE is set). A plaintext token left in the config file by
// an older version is moved to the credential store on the way.
func ResolveAccessToken(team, flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if token := os.Getenv(EnvAPIToken); token != "" {
		return token, nil
	}
	if strings.TrimSpace(team) == "" {
		return "", errors.New("team domain is required")
	}

	if InsecureStorage() {
		var token string
		if path := viper.ConfigFileUsed(); path != "" {
			raw, err := readRawConfig(path)
			if err != nil {
				return "", err
			}
			token = rawString(raw, "access_token")
		}
		if token == "" {
			return "", noTokenError(team)
		}
		return token, nil
	}

	// A plaintext token that could not be moved keeps being used. The credential
	// store is not asked again then, so that its timeouts do not pile up.
	if token, fileTeam := migrateLegacyToken(); token != "" && (fileTeam == "" || strings.EqualFold(fileTeam, team)) {
		return token, nil
	}

	token, err := credential.Get(team)
	switch {
	case err == nil:
		return token, nil
	case errors.Is(err, credential.ErrNotFound):
		return "", noTokenError(team)
	default:
		return "", fmt.Errorf("failed to read the access token from the OS credential store: %w (set %s, or set %s=1 to keep the token in the config file)", err, EnvAPIToken, EnvInsecureStorage)
	}
}

// StoreAccessToken saves the access token for the team: in the OS credential
// store, or in the config file when DOCBASE_INSECURE_STORAGE is set. A plaintext
// token left in the config file by an older version is removed once the
// credential store holds the new one.
func StoreAccessToken(team, token string) error {
	if InsecureStorage() {
		path, err := configFileForWrite()
		if err != nil {
			return err
		}
		raw, err := readRawConfig(path)
		if err != nil {
			return err
		}
		raw["access_token"] = token
		if err := writeRawConfig(path, raw); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(warnOutput, "warning: %s is set; the access token is stored in plaintext in %s\n", EnvInsecureStorage, path)
		return nil
	}

	if strings.TrimSpace(team) == "" {
		return errors.New("team domain is required to store the access token (set it with --team)")
	}
	if err := credential.Set(team, token); err != nil {
		return fmt.Errorf("failed to save the access token to the OS credential store: %w (use %s for non-persistent use, or set %s=1 to store it in the config file)", err, EnvAPIToken, EnvInsecureStorage)
	}

	// A plaintext copy left in the file would be migrated over the new token later.
	if err := removeFileToken(); err != nil {
		return fmt.Errorf("the access token was saved to the OS credential store but the plaintext copy in the config file could not be removed: %w", err)
	}

	return nil
}

// DeleteAccessToken removes the stored access token from the config file and,
// unless the config file is the storage or the team is empty, from the OS
// credential store. Nothing being stored is not an error.
func DeleteAccessToken(team string) error {
	// The file goes first: if the credential store entry were deleted first, a
	// plaintext token left in the file would be migrated back on the next run.
	if err := removeFileToken(); err != nil {
		return err
	}
	if InsecureStorage() || strings.TrimSpace(team) == "" {
		return nil
	}

	return credential.Delete(team)
}

// migrateLegacyToken moves a plaintext access_token left in the config file by
// an older version into the OS credential store and removes it from the file.
// It returns the token and the file's team_domain only when the plaintext token
// remains in the file and so is still the one to use; otherwise both are empty.
// Whatever fails before the removal leaves the file untouched.
func migrateLegacyToken() (token, team string) {
	path := viper.ConfigFileUsed()
	if path == "" {
		return "", ""
	}
	raw, err := readRawConfig(path)
	if err != nil {
		return "", ""
	}
	token = rawString(raw, "access_token")
	if token == "" {
		return "", ""
	}

	// The team recorded in the file names the credential store entry, not --team.
	team = rawString(raw, "team_domain")
	if strings.TrimSpace(team) == "" {
		_, _ = fmt.Fprintf(warnOutput, "warning: %s contains a plaintext access token but no team_domain; run 'docbase auth login' to move it to the OS credential store\n", path)
		return token, ""
	}

	if err := credential.Set(team, token); err != nil {
		_, _ = fmt.Fprintf(warnOutput, "warning: could not move the access token in %s to the OS credential store: %v; the plaintext token is still being used (set %s=1 to keep it in the config file intentionally)\n", path, err, EnvInsecureStorage)
		return token, team
	}

	if err := removeFileToken(); err != nil {
		// The credential store has the token now; the next run retries the removal.
		_, _ = fmt.Fprintf(warnOutput, "warning: the access token was saved to the OS credential store but could not be removed from %s: %v\n", path, err)
		return "", ""
	}

	_, _ = fmt.Fprintf(warnOutput, "docbase: moved the access token from %s to the OS credential store\n", path)
	return "", ""
}

// removeFileToken deletes access_token from the config file and keeps every
// other key. It does nothing, and never creates the file, when the file or the
// key does not exist.
func removeFileToken() error {
	path := viper.ConfigFileUsed()
	if path == "" {
		return nil
	}
	raw, err := readRawConfig(path)
	if err != nil {
		return err
	}
	if _, ok := raw["access_token"]; !ok {
		return nil
	}
	delete(raw, "access_token")

	return writeRawConfig(path, raw)
}

// rawString returns the string value of key in a raw config object, or "" when
// the key is missing or not a string.
func rawString(raw map[string]any, key string) string {
	value, _ := raw[key].(string)
	return value
}

// noTokenError reports that no access token is available for the team.
func noTokenError(team string) error {
	hint := ""
	if os.Getenv(legacyEnvToken) != "" {
		hint = fmt.Sprintf("; the %s environment variable is no longer read, use %s instead", legacyEnvToken, EnvAPIToken)
	}

	return fmt.Errorf("%w for team %q: run 'docbase auth login --team %s --token <token>' or set %s%s", ErrNoToken, team, team, EnvAPIToken, hint)
}
