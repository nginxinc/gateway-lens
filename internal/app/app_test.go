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

	"github.com/sjberman/gateway-lens/internal/app"
	"github.com/sjberman/gateway-lens/internal/topology"
)

const (
	serveTestTimeout = 3 * time.Second
	pollInterval     = 25 * time.Millisecond
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

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)
	assertDashboardDataEndpoint(ctx, g, dashboardAddr)
	assertDashboardPageEndpoint(ctx, t, g, dashboardAddr)

	cancel()
	g.Eventually(errCh, serveTestTimeout).Should(Receive(BeNil()))
}

func waitForDashboardReady(g Gomega, runnable *app.HTTPServerRunnable) string {
	var addr string

	g.Eventually(func() string {
		addr = runnable.Addr()

		return addr
	}).WithPolling(pollInterval).WithTimeout(serveTestTimeout).ShouldNot(BeEmpty())

	return "http://" + addr
}

func assertDashboardDataEndpoint(ctx context.Context, g Gomega, dashboardAddr string) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr+"/data", nil)
	g.Expect(err).ToNot(HaveOccurred())

	response, err := http.DefaultClient.Do(request)
	g.Expect(err).ToNot(HaveOccurred())

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(response.StatusCode).To(Equal(http.StatusOK))
	g.Expect(string(body)).To(ContainSubstring("HTTPRoute"))
}

func assertDashboardPageEndpoint(ctx context.Context, t *testing.T, g Gomega, dashboardAddr string) {
	t.Helper()

	pageRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, dashboardAddr, nil)
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

func TestSSEEndpointStreamsChangedEvents(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	reader := newTestResourcesReader()
	runnable := app.NewHTTPServerRunnable(reader, "127.0.0.1:0", logr.Discard())

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

	g.Expect(lines).To(ContainElement("event: changed"))
	g.Expect(lines).To(ContainElement("data: {}"))

	cancel()
	g.Eventually(errCh, serveTestTimeout).Should(Receive(BeNil()))
}

func TestSSEEndpointRejectsBrowserNavigation(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", logr.Discard())

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
	g.Eventually(errCh, serveTestTimeout).Should(Receive(BeNil()))
}

func TestEndpointsRejectNonGETMethods(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(t.Context(), serveTestTimeout)
	defer cancel()

	runnable := app.NewHTTPServerRunnable(newTestResourcesReader(), "127.0.0.1:0", logr.Discard())

	errCh := make(chan error, 1)

	go func() {
		errCh <- runnable.Start(ctx)
	}()

	dashboardAddr := waitForDashboardReady(g, runnable)

	endpoints := []string{"/data", "/events", "/"}

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, dashboardAddr+endpoint, nil)
		g.Expect(err).ToNot(HaveOccurred())

		resp, err := http.DefaultClient.Do(req)
		g.Expect(err).ToNot(HaveOccurred())

		_ = resp.Body.Close()

		g.Expect(resp.StatusCode).To(Equal(http.StatusMethodNotAllowed),
			"expected 405 for POST %s", endpoint)
	}

	cancel()
	g.Eventually(errCh, serveTestTimeout).Should(Receive(BeNil()))
}
