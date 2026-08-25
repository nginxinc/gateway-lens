/*
Copyright 2026 F5, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package main provides the gateway-lens executable entrypoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/util/validation"
	ctlrconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	ctlrlog "sigs.k8s.io/controller-runtime/pkg/log"
	ctlrZap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/nginxinc/gateway-lens/internal/k8s/manager"
)

const (
	versionCommandName = "version"
	defaultPort        = 8080
	defaultLogLevel    = "info"
	minPort            = 1
	maxPort            = 65535
)

var (
	errInvalidLogLevel  = errors.New("invalid log level")
	errInvalidPort      = errors.New("invalid port")
	errInvalidNamespace = errors.New("invalid namespace")
)

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
		Use:           "gateway-lens",
		Short:         "Visualize Gateway API resources and relationships",
		SilenceUsage:  true,
		SilenceErrors: true,
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

	if port < minPort || port > maxPort {
		return fmt.Errorf("%w: %d (must be between %d and %d)", errInvalidPort, port, minPort, maxPort)
	}

	if err := validateNamespaces(namespaces); err != nil {
		return err
	}

	logger := ctlrZap.New(ctlrZap.Level(zap.NewAtomicLevelAt(zapLevel)))
	ctlrlog.SetLogger(logger)

	host := "127.0.0.1"
	if isContainer() {
		host = "0.0.0.0"
	}

	listenAddr := net.JoinHostPort(host, strconv.Itoa(port))

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

// validateNamespaces reports an error if any namespace is not a valid DNS-1123 subdomain.
func validateNamespaces(namespaces []string) error {
	for _, ns := range namespaces {
		if errs := validation.IsDNS1123Subdomain(ns); len(errs) > 0 {
			return fmt.Errorf("%w: %q (%s)", errInvalidNamespace, ns, strings.Join(errs, "; "))
		}
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
