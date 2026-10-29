package broker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/basi/docbase-cli/cmd/root"
	"github.com/basi/docbase-cli/internal/broker"
	"github.com/basi/docbase-cli/internal/config"
)

// linuxOnly is the error of the commands that run a broker on other platforms.
const linuxOnly = "docbase broker is supported on Linux only"

// hostOS is the platform the commands run on. Tests replace it.
var hostOS = runtime.GOOS

// validateToken checks the access token against the DocBase API. It goes through
// the client of the broker, not the ordinary client of the CLI, so that no proxy
// from the environment and no redirect can take the token elsewhere. Tests replace it.
var validateToken = broker.ValidateToken

var (
	// BrokerCmd represents the broker command
	BrokerCmd = &cobra.Command{
		Use:   "broker",
		Short: "Let an AI agent use DocBase without being able to read the access token",
		Long: `Run a broker that holds the DocBase access token in memory and serves
read-only DocBase requests for one team on a Unix socket. A process that only
knows the socket path (DOCBASE_BROKER_SOCKET) can run read-only docbase commands
but cannot obtain the token.

The broker is meant to defend against a process of the same user that is not
root. It does not defend against root: drop sudo and container access before
the untrusted process runs.

Use a read-only access token for the broker.`,
	}

	// StartCmd represents the broker start command
	StartCmd = &cobra.Command{
		Use:   "start",
		Short: "Start a broker in the background",
		Long: `Start a broker in the background and print DOCBASE_BROKER_SOCKET=<path> on
stdout, so that the line can be appended to $GITHUB_ENV. Messages go to stderr.

The access token is taken from --token or DOCBASE_API_TOKEN (see 'docbase auth
login' for the stored token). It is handed to the broker process without
passing through its arguments, environment or any file.

Example:
  docbase broker start --team your-team --ttl 30m >> "$GITHUB_ENV"`,
		Args: cobra.NoArgs,
		RunE: runStart,
	}

	// StatusCmd represents the broker status command
	StatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Show the team and the expiry time of a running broker",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			socket, err := socketPath(cmd)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true

			info, err := broker.Status(socket)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "team: %s\nexpires_at: %s\n", info.Team, info.ExpiresAt.Format(time.RFC3339))

			return err
		},
	}

	// StopCmd represents the broker stop command
	StopCmd = &cobra.Command{
		Use:   "stop",
		Short: "Stop a running broker",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			socket, err := socketPath(cmd)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true

			if err := broker.Stop(socket); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Stopped the DocBase broker at %s\n", socket)

			return err
		},
	}

	// ServeCmd represents the internal broker serve command, which 'broker start' runs
	ServeCmd = &cobra.Command{
		Use:    "serve",
		Short:  "Run a broker in this process (internal; use 'broker start')",
		Hidden: true,
		Args:   cobra.NoArgs,
		// The process has no home directory and needs no configuration.
		Annotations: map[string]string{root.SkipConfigAnnotation: "true"},
		RunE:        runServe,
	}
)

func init() {
	broker.Version = root.Version

	root.AddCommand(BrokerCmd)
	BrokerCmd.AddCommand(StartCmd)
	BrokerCmd.AddCommand(StatusCmd)
	BrokerCmd.AddCommand(StopCmd)
	BrokerCmd.AddCommand(ServeCmd)

	StartCmd.Flags().String("team", "", "DocBase team domain (default: the configured team)")
	StartCmd.Flags().Duration("ttl", broker.DefaultTTL, fmt.Sprintf("How long the broker runs, from %s to %s", broker.MinTTL, broker.MaxTTL))

	for _, c := range []*cobra.Command{StatusCmd, StopCmd} {
		c.Flags().String("socket", "", "Socket of the broker (default: $"+config.EnvBrokerSocket+")")
	}

	ServeCmd.Flags().String("team", "", "DocBase team domain")
	ServeCmd.Flags().String("socket", "", "Path of the socket to listen on")
	ServeCmd.Flags().Duration("ttl", broker.DefaultTTL, "How long the broker runs")
}

func runStart(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true // the arguments are fine; what follows are not usage errors

	if hostOS != "linux" {
		return errors.New(linuxOnly)
	}
	if socket, active, err := config.BrokerMode(); active {
		if err != nil {
			return fmt.Errorf("%w: unset it before starting a broker", err)
		}
		return fmt.Errorf("%s is already set (%s): stop that broker or unset the variable first", config.EnvBrokerSocket, socket)
	}

	team, _ := cmd.Flags().GetString("team")
	if team == "" {
		team = config.GetTeamDomain("")
	}
	if team == "" {
		return errors.New("team domain is required (use --team)")
	}
	if err := broker.ValidTeam(team); err != nil {
		return err
	}
	ttl, _ := cmd.Flags().GetDuration("ttl")
	if err := broker.CheckTTL(ttl); err != nil {
		return err
	}

	token, err := config.ResolveAccessToken(team, cmd.Flag("token").Value.String())
	if err != nil {
		return err
	}
	if err := broker.ValidToken(token); err != nil {
		return err
	}
	if err := validateToken(team, token); err != nil {
		// The message comes from the network; keep the token out of it in any case.
		return fmt.Errorf("failed to authenticate with DocBase: %s", strings.ReplaceAll(err.Error(), token, "[redacted]"))
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find the docbase executable: %w", err)
	}
	dir, socket, err := broker.CreateSocketDir(broker.SocketBase())
	if err != nil {
		return err
	}

	pid, err := broker.Start(broker.StartOptions{Exe: exe, Team: team, Token: token, SocketPath: socket, TTL: ttl})
	if err != nil {
		_ = os.Remove(socket)
		_ = os.Remove(dir)
		return err
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "DocBase broker for team %s started (pid %d), it stops in %s\n", team, pid, ttl)
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", config.EnvBrokerSocket, socket)

	return err
}

func runServe(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	if hostOS != "linux" {
		return errors.New(linuxOnly)
	}

	team, _ := cmd.Flags().GetString("team")
	socket, _ := cmd.Flags().GetString("socket")
	ttl, _ := cmd.Flags().GetDuration("ttl")
	if err := broker.CheckTTL(ttl); err != nil {
		return err
	}

	// Started by hand, the process would write to whatever file descriptor 3 is.
	status, err := inheritedPipe(3, "status")
	if err != nil {
		return err
	}
	if info, err := os.Stdin.Stat(); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return errors.New("standard input is not a pipe: run 'docbase broker start' instead")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	return broker.RunServe(ctx, broker.ServeOptions{
		Team:       team,
		SocketPath: socket,
		TTL:        ttl,
		Version:    root.Version,
		Stdin:      os.Stdin,
		Status:     status,
	})
}

// inheritedPipe returns the pipe that start passed as file descriptor fd.
func inheritedPipe(fd uintptr, name string) (*os.File, error) {
	f := os.NewFile(fd, name)
	if f == nil {
		return nil, fmt.Errorf("file descriptor %d is not open: run 'docbase broker start' instead", fd)
	}
	if info, err := f.Stat(); err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		// Not closed: it is not ours.
		return nil, fmt.Errorf("file descriptor %d is not a pipe: run 'docbase broker start' instead", fd)
	}

	return f, nil
}

// socketPath returns the socket named by --socket or DOCBASE_BROKER_SOCKET.
func socketPath(cmd *cobra.Command) (string, error) {
	if socket, _ := cmd.Flags().GetString("socket"); socket != "" {
		return socket, nil
	}

	socket, active, err := config.BrokerMode()
	switch {
	case err != nil:
		return "", err
	case !active:
		return "", fmt.Errorf("no broker socket: set %s or use --socket", config.EnvBrokerSocket)
	}

	return socket, nil
}
