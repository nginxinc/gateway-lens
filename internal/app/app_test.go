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

package app_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/app"
	"github.com/nginxinc/gateway-lens/internal/topology"
)

const (
	serveTestTimeout    = 3 * time.Second
	shutdownWaitTimeout = 10 * time.Second
	pollInterval        = 25 * time.Millisecond
)

type fakeResourcesReader struct {
	resources topology.GatewayAPIResources

	mu          sync.Mutex
	subscribers []chan struct{}
}

func (f *fakeResourcesReader) Get() topology.GatewayAPIResources {
	return f.resources
}

func (f *fakeResourcesReader) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)

	f.mu.Lock()
	f.subscribers = append(f.subscribers, ch)
	f.mu.Unlock()

	return ch
}

func (f *fakeResourcesReader) Unsubscribe(ch <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, subscriber := range f.subscribers {
		if subscriber == ch {
			f.subscribers = append(f.subscribers[:i], f.subscribers[i+1:]...)

			close(subscriber)

			return
		}
	}
}

func (f *fakeResourcesReader) notify() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, ch := range f.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func newTestResourcesReader() *fakeResourcesReader {
	return &fakeResourcesReader{
		resources: topology.GatewayAPIResources{
			GatewayClasses: []gatewayv1.GatewayClass{
				{ObjectMeta: metav1.ObjectMeta{Name: "test-class"}},
			},
			Gateways: []gatewayv1.Gateway{
				{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-gateway"}},
			},
			HTTPRoutes: []gatewayv1.HTTPRoute{
				{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "test-route"}},
			},
		},
	}
}

func TestHTTPServerRunnableStartsDashboard(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", "", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)
	assertDashboardDataEndpoint(ctx, g, dashboardAddr, "")
	assertDashboardIssuesEndpoint(ctx, g, dashboardAddr, "")
	assertDashboardPageEndpoint(ctx, t, g, dashboardAddr, "")

	cancel()
	g.Eventually(errCh, shutdownWaitTimeout).Should(Receive(BeNil()))
}

func waitForDashboardReady(g Gomega, runnable *app.HTTPServerRunnable) string {
	var addr string

	g.Eventually(func() string {
		addr = runnable.Addr()

		return addr
	}).WithPolling(pollInterval).WithTimeout(serveTestTimeout).ShouldNot(BeEmpty())

	return "http://" + addr
}

func assertDashboardDataEndpoint(ctx context.Context, g Gomega, dashboardAddr, basePath string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+basePath+"/api/data", nil)
	g.Expect(err).ToNot(HaveOccurred())

	response, err := http.DefaultClient.Do(request)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response.StatusCode).To(Equal(http.StatusOK))
	g.Expect(string(body)).To(ContainSubstring("HTTPRoute"))
}

func assertDashboardIssuesEndpoint(ctx context.Context, g Gomega, dashboardAddr, basePath string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+basePath+"/api/issues", nil)
	g.Expect(err).ToNot(HaveOccurred())

	response, err := http.DefaultClient.Do(request)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response.StatusCode).To(Equal(http.StatusOK))
	g.Expect(string(body)).To(ContainSubstring("generatedAt"))
	g.Expect(string(body)).To(ContainSubstring("issues"))
}

func assertDashboardPageEndpoint(ctx context.Context, t *testing.T, g Gomega, dashboardAddr, basePath string) {
	t.Helper()

	pageRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+basePath+"/", nil)
	g.Expect(err).ToNot(HaveOccurred())

	pageResponse, err := http.DefaultClient.Do(pageRequest)
	g.Expect(err).ToNot(HaveOccurred())

	t.Cleanup(func() {
		_ = pageResponse.Body.Close()
	})

	pageBody, err := io.ReadAll(pageResponse.Body)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(pageBody)).To(ContainSubstring("Gateway Lens"))
}

func TestHTTPServerRunnableServesUnderBasePath(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	const basePath = "/gateway-lens"

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", basePath, logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)
	assertDashboardDataEndpoint(ctx, g, dashboardAddr, basePath)
	assertDashboardIssuesEndpoint(ctx, g, dashboardAddr, basePath)
	assertDashboardPageEndpoint(ctx, t, g, dashboardAddr, basePath)

	// A request to the bare base path (no trailing slash) should be
	// redirected to the base path with a trailing slash, so that relative
	// asset/API URLs in the served page resolve correctly.
	redirectReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+basePath, nil)
	g.Expect(err).ToNot(HaveOccurred())

	noRedirectClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	redirectResp, err := noRedirectClient.Do(redirectReq)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = redirectResp.Body.Close() }()

	g.Expect(redirectResp.StatusCode).To(Equal(http.StatusTemporaryRedirect))
	g.Expect(redirectResp.Header.Get("Location")).To(Equal(basePath + "/"))

	// Requests outside the base path should not be handled by the dashboard.
	unmatchedReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+"/data", nil)
	g.Expect(err).ToNot(HaveOccurred())

	unmatchedResp, err := http.DefaultClient.Do(unmatchedReq)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = unmatchedResp.Body.Close() }()

	g.Expect(unmatchedResp.StatusCode).To(Equal(http.StatusNotFound))

	cancel()
	g.Eventually(errCh, shutdownWaitTimeout).Should(Receive(BeNil()))
}

func TestHealthzEndpointIgnoresBasePath(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", "/gateway-lens", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+"/healthz", nil)
	g.Expect(err).ToNot(HaveOccurred())

	resp, err := http.DefaultClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	g.Expect(resp.StatusCode).To(Equal(http.StatusOK))

	cancel()
	g.Eventually(errCh, shutdownWaitTimeout).Should(Receive(BeNil()))
}

func TestSSEEndpointStreamsChangedEvents(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	reader := newTestResourcesReader()
	runnable := app.NewHTTPServerRunnable(reader, "127.0.0.1:0", "", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)

	sseReq, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+"/events", nil)
	g.Expect(err).ToNot(HaveOccurred())

	sseResp, err := http.DefaultClient.Do(sseReq)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = sseResp.Body.Close() }()

	g.Expect(sseResp.StatusCode).To(Equal(http.StatusOK))
	g.Expect(sseResp.Header.Get("Content-Type")).To(Equal("text/event-stream"))

	// Trigger a change notification.
	reader.notify()

	// Read the SSE event.
	scanner := bufio.NewScanner(sseResp.Body)

	var lines []string

	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)

		// SSE events end with an empty line.
		if line == "" && len(lines) > 1 {
			break
		}
	}

	g.Expect(scanner.Err()).ToNot(HaveOccurred())

	g.Expect(lines).To(ContainElement("event: changed"))
	g.Expect(lines).To(ContainElement("data: {}"))

	_ = sseResp.Body.Close()

	cancel()
	g.Eventually(errCh, shutdownWaitTimeout).Should(Receive(BeNil()))
}

func TestSSEEndpointRejectsBrowserNavigation(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", "", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+"/events", nil)
	g.Expect(err).ToNot(HaveOccurred())

	req.Header.Set("Accept", "text/html")

	resp, err := http.DefaultClient.Do(req)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = resp.Body.Close() }()

	g.Expect(resp.StatusCode).To(Equal(http.StatusNotAcceptable))

	cancel()
	g.Eventually(errCh, shutdownWaitTimeout).Should(Receive(BeNil()))
}

func TestEndpointsRejectNonGETMethods(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", "", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)

	endpoints := []string{"/api/data", "/api/issues", "/events", "/", "/healthz"}

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, dashboardAddr+endpoint, nil)
		g.Expect(err).ToNot(HaveOccurred())

		resp, err := http.DefaultClient.Do(req)
		g.Expect(err).ToNot(HaveOccurred())

		_, err = io.Copy(io.Discard, resp.Body)
		g.Expect(err).ToNot(HaveOccurred())

		_ = resp.Body.Close()

		g.Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed),
			"expected 405 for POST %s", endpoint)
	}

	cancel()
	g.Eventually(errCh, shutdownWaitTimeout).Should(Receive(BeNil()))
}
