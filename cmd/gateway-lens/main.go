// Package main provides the gateway-lens executable entrypoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	ctlrconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	ctlrlog "sigs.k8s.io/controller-runtime/pkg/log"
	ctlrZap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/sjberman/gateway-lens/internal/k8s/manager"
)

const (
	versionCommandName = "version"
	defaultPort        = 8080
	defaultLogLevel    = "info"
)

var errInvalidLogLevel = errors.New("invalid log level")

// containerMarkerFiles are files whose existence indicates a container environment.
var containerMarkerFiles = []string{ //nolint:gochecknoglobals // static lookup table
	"/.dockerenv",
	"/run/.containerenv",
	"/var/run/secrets/kubernetes.io/serviceaccount",
}

// isContainer reports whether the process is running inside a container
// by checking for well-known container marker files.
func isContainer() bool {
	for _, f := range containerMarkerFiles {
		if _, err := os.Stat(f); err == nil {
			return true
		}
	}

	return false
}

//nolint:gochecknoglobals // These are set via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if err := newRootCommand(os.Stdout, os.Stderr).ExecuteContext(context.Background()); err != nil {
		log.Fatal(err)
	}
}

// newRootCommand creates the root cobra command with flags and subcommands.
func newRootCommand(out io.Writer, errOut io.Writer) *cobra.Command {
	var (
		port       int
		logLevel   string
		namespaces []string
	)

	rootCmd := &cobra.Command{
		Use:          "gateway-lens",
		Short:        "Visualize Gateway API resources and relationships",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd.Context(), port, logLevel, namespaces)
		},
	}

	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)

	rootCmd.Flags().IntVar(&port, "port", defaultPort, "port for the dashboard HTTP server")
	rootCmd.Flags().StringVar(
		&logLevel, "log-level", defaultLogLevel,
		"log level (one of 'debug', 'info', 'error', 'panic')",
	)
	rootCmd.Flags().StringSliceVar(
		&namespaces, "namespaces", nil,
		"list of namespaces to watch (default all namespaces)",
	)

	if kubeconfigFlag := flag.CommandLine.Lookup(ctlrconfig.KubeconfigFlagName); kubeconfigFlag != nil {
		rootCmd.PersistentFlags().AddGoFlag(kubeconfigFlag)
	}

	rootCmd.AddCommand(newVersionCommand(out, version, commit))

	return rootCmd
}

var logLevels = map[string]zapcore.Level{ //nolint:gochecknoglobals // static lookup table
	"debug": zap.DebugLevel,
	"info":  zap.InfoLevel,
	"error": zap.ErrorLevel,
	"panic": zap.PanicLevel,
}

// run initializes the logger, creates the manager, and starts the process.
func run(ctx context.Context, port int, logLevel string, namespaces []string) error {
	zapLevel, ok := logLevels[logLevel]
	if !ok {
		return fmt.Errorf("%w: %q (must be one of 'debug', 'info', 'error', 'panic')", errInvalidLogLevel, logLevel)
	}

	logger := ctlrZap.New(ctlrZap.Level(zap.NewAtomicLevelAt(zapLevel)))
	ctlrlog.SetLogger(logger)

	host := "127.0.0.1"
	if isContainer() {
		host = "0.0.0.0" //nolint:gosec // intentional bind to all interfaces in containers
	}

	listenAddr := fmt.Sprintf("%s:%d", host, port)

	logger.Info("Starting gateway-lens", "version", version, "commit", commit)

	mgr, err := manager.New(listenAddr, namespaces)
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("starting manager: %w", err)
	}

	return nil
}

// newVersionCommand creates the "version" subcommand.
func newVersionCommand(out io.Writer, buildVersion, buildCommit string) *cobra.Command {
	return &cobra.Command{
		Use:   versionCommandName,
		Short: "Print version information",
		RunE: func(_ *cobra.Command, _ []string) error {
			return printVersion(out, buildVersion, buildCommit)
		},
	}
}

// printVersion writes the build version and commit to the given writer.
func printVersion(out io.Writer, buildVersion, buildCommit string) error {
	if _, err := fmt.Fprintf(out, "version=%s commit=%s\n", buildVersion, buildCommit); err != nil {
		return fmt.Errorf("writing version output: %w", err)
	}

	return nil
}
