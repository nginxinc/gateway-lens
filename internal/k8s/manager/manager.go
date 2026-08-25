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

// Package manager contains runtime bootstrap helpers for Kubernetes-backed data sources.
package manager

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctlrconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	ctlrlog "sigs.k8s.io/controller-runtime/pkg/log"
	ctlrmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/app"
	"github.com/nginxinc/gateway-lens/internal/k8s/resources"
)

// Manager wraps a controller-runtime manager for this process.
type Manager struct {
	// manager is the underlying controller-runtime manager.
	manager ctlrmanager.Manager
	// logger is the structured logger for manager lifecycle events.
	logger logr.Logger
	// listenAddress is the TCP address for the dashboard HTTP server.
	listenAddress string
}

// New creates a manager configured for Gateway API cache/watch use,
// registers resource controllers, and wires the HTTP server runnable.
func New(listenAddress string, namespaces []string) (*Manager, error) {
	logger := ctlrlog.Log.WithName("manager")

	restConfig, err := ctlrconfig.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubernetes rest config: %w", err)
	}

	scheme, err := newGatewayAPIScheme()
	if err != nil {
		return nil, err
	}

	cacheOpts := cache.Options{
		DefaultTransform: cache.TransformStripManagedFields(),
	}

	if len(namespaces) > 0 {
		cacheOpts.DefaultNamespaces = buildNamespaceMap(namespaces)

		logger.Info("Namespace filtering enabled", "namespaces", namespaces)
	}

	mgr, err := ctlrmanager.New(restConfig, ctlrmanager.Options{
		Scheme:  scheme,
		Metrics: metricsserver.Options{BindAddress: "0"},
		Cache:   cacheOpts,
	})
	if err != nil {
		return nil, fmt.Errorf("creating controller-runtime manager: %w", err)
	}

	m := &Manager{manager: mgr, logger: logger, listenAddress: listenAddress}

	if err := m.registerRunnables(); err != nil {
		return nil, err
	}

	return m, nil
}

// RegisterController registers a controller for a specific object type and reconciler.
// The controller's internal log messages are raised to debug verbosity so they don't appear at the default info level.
func (m *Manager) RegisterController(name string, object client.Object, rec reconcile.Reconciler) error {
	bld := builder.ControllerManagedBy(m.manager).
		Named(name).
		For(object).
		WithLogConstructor(func(_ *reconcile.Request) logr.Logger {
			return m.manager.GetLogger().WithName(name).V(1)
		})

	if err := bld.Complete(rec); err != nil {
		return fmt.Errorf("registering controller %s: %w", name, err)
	}

	return nil
}

// Start starts the manager and blocks until shutdown.
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting manager")

	if err := m.manager.Start(ctx); err != nil {
		return fmt.Errorf("starting controller-runtime manager: %w", err)
	}

	m.logger.Info("Manager stopped")

	return nil
}

// registerRunnables wires resource controllers and the HTTP server into the manager.
func (m *Manager) registerRunnables() error {
	res := resources.New(m.manager.GetCache(), m.logger.WithName("resources"))
	if err := res.RegisterControllers(m); err != nil {
		return fmt.Errorf("registering resources controllers: %w", err)
	}

	serverRunnable := app.NewHTTPServerRunnable(res, m.listenAddress, m.logger.WithName("httpServer"))

	runnables := []ctlrmanager.Runnable{
		res,
		serverRunnable,
	}

	for _, runnable := range runnables {
		if err := m.manager.Add(runnable); err != nil {
			return fmt.Errorf("adding runnable: %w", err)
		}
	}

	return nil
}

// newGatewayAPIScheme builds a runtime scheme containing core v1, apiextensions v1, and Gateway API types.
func newGatewayAPIScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding core v1 scheme: %w", err)
	}

	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("adding apiextensions v1 scheme: %w", err)
	}

	if err := gatewayv1.Install(scheme); err != nil {
		return nil, fmt.Errorf("adding gateway v1 scheme: %w", err)
	}

	return scheme, nil
}

// buildNamespaceMap converts a namespace slice into the map format
// expected by cache.Options.DefaultNamespaces.
func buildNamespaceMap(namespaces []string) map[string]cache.Config {
	nsMap := make(map[string]cache.Config, len(namespaces))
	for _, ns := range namespaces {
		nsMap[ns] = cache.Config{}
	}

	return nsMap
}
