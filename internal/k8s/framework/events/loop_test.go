package events_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nginxinc/gateway-lens/internal/k8s/framework/events"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
)

// fakeHandler is a test EventHandler that records batches and optionally blocks.
type fakeHandler struct {
	mu      sync.Mutex
	batches []events.EventBatch

	// When set, HandleEventBatch blocks until this channel is closed.
	blockCh chan struct{}
	// When set, HandleEventBatch panics with this value.
	panicVal any
	// Called (if non-nil) on each HandleEventBatch invocation before blocking.
	onHandle func(events.EventBatch)
}

func (h *fakeHandler) HandleEventBatch(_ context.Context, batch events.EventBatch) {
	if h.onHandle != nil {
		h.onHandle(batch)
	}

	if h.panicVal != nil {
		panic(h.panicVal)
	}

	if h.blockCh != nil {
		<-h.blockCh
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	cp := make(events.EventBatch, len(batch))
	copy(cp, batch)
	h.batches = append(h.batches, cp)
}

func (h *fakeHandler) getBatches() []events.EventBatch {
	h.mu.Lock()
	defer h.mu.Unlock()

	out := make([]events.EventBatch, len(h.batches))
	copy(out, h.batches)

	return out
}

func TestNewEventLoop(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any)
	handler := &fakeHandler{}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	g.Expect(loop).ToNot(BeNil())
}

func TestEventLoopSingleEvent(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any, 1)
	handler := &fakeHandler{}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	eventCh <- "event-1"

	g.Eventually(handler.getBatches, 2*time.Second, 10*time.Millisecond).
		Should(HaveLen(1))

	batches := handler.getBatches()
	g.Expect(batches[0]).To(HaveLen(1))
	g.Expect(batches[0][0]).To(Equal("event-1"))

	cancel()
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
}

func TestEventLoopBatchesEventsWhileHandling(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any, 10)
	blockCh := make(chan struct{})
	handling := make(chan struct{}, 1)
	handler := &fakeHandler{
		blockCh: blockCh,
		onHandle: func(_ events.EventBatch) {
			select {
			case handling <- struct{}{}:
			default:
			}
		},
	}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	// Send first event — will trigger immediate handling.
	eventCh <- "event-1"

	// Wait until handler is actively processing.
	g.Eventually(handling).Should(Receive())

	// Send more events while handler is blocked.
	eventCh <- "event-2"

	eventCh <- "event-3"

	// Unblock the handler.
	close(blockCh)

	// Wait until all three events have been handled.
	// The first batch is always [event-1] because it triggered handling.
	// Events 2 and 3 arrive while handling is blocked, but Go's select is non-deterministic,
	// so the loop may drain both into one batch ([event-2, event-3]) or handle them separately
	// ([event-2], [event-3]). We assert on the total event count and first-batch contents only.
	allEvents := func() []any {
		var all []any
		for _, b := range handler.getBatches() {
			all = append(all, b...)
		}

		return all
	}

	g.Eventually(allEvents, 2*time.Second, 10*time.Millisecond).
		Should(HaveLen(3))

	batches := handler.getBatches()
	g.Expect(batches[0]).To(Equal(events.EventBatch{"event-1"}))
	g.Expect(len(batches)).To(BeNumerically(">=", 2))
	g.Expect(len(batches)).To(BeNumerically("<=", 3))

	cancel()
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
}

func TestEventLoopCancelWhileNotHandling(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any)
	handler := &fakeHandler{}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	// Cancel immediately — no events, not handling.
	cancel()
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
	g.Expect(handler.getBatches()).To(BeEmpty())
}

func TestEventLoopCancelWhileHandling(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any, 1)
	blockCh := make(chan struct{})
	handling := make(chan struct{}, 1)
	handler := &fakeHandler{
		blockCh: blockCh,
		onHandle: func(_ events.EventBatch) {
			select {
			case handling <- struct{}{}:
			default:
			}
		},
	}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	// Start handling.
	eventCh <- "event-1"

	g.Eventually(handling).Should(Receive())

	// Cancel while handler is blocked.
	cancel()

	// Loop should not exit until handler finishes.
	g.Consistently(loopDone, 100*time.Millisecond).ShouldNot(Receive())

	// Now unblock handler.
	close(blockCh)
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
}

func TestEventLoopHandlerPanicRecovery(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any, 2)

	var callCount atomic.Int32

	handler := &fakeHandler{
		onHandle: func(_ events.EventBatch) {
			if callCount.Add(1) == 1 {
				panic("test panic")
			}
		},
		// Don't set panicVal here since we need to only panic on first call.
	}

	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	// First event triggers a panic in the handler.
	eventCh <- "event-1"

	// Wait for the panic to be handled and loop to recover.
	g.Eventually(callCount.Load, 2*time.Second, 10*time.Millisecond).Should(BeNumerically(">=", 1))

	// Second event should still be handled after panic recovery.
	eventCh <- "event-2"

	g.Eventually(handler.getBatches, 2*time.Second, 10*time.Millisecond).
		Should(HaveLen(1))

	batches := handler.getBatches()
	g.Expect(batches[0]).To(HaveLen(1))
	g.Expect(batches[0][0]).To(Equal("event-2"))

	cancel()
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
}

func TestEventLoopMultipleBatchCycles(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any, 10)
	handler := &fakeHandler{}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	// Send 3 events sequentially, waiting for each to be handled.
	for i := range 3 {
		eventCh <- i

		g.Eventually(func() int {
			return len(handler.getBatches())
		}, 2*time.Second, 10*time.Millisecond).Should(Equal(i + 1))
	}

	batches := handler.getBatches()
	g.Expect(batches).To(HaveLen(3))

	for i, batch := range batches {
		g.Expect(batch).To(HaveLen(1))
		g.Expect(batch[0]).To(Equal(i))
	}

	cancel()
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
}

func TestEventLoopHandlingDoneWithNoPendingEvents(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	eventCh := make(chan any, 1)
	blockCh := make(chan struct{})
	handling := make(chan struct{}, 1)
	handler := &fakeHandler{
		blockCh: blockCh,
		onHandle: func(_ events.EventBatch) {
			select {
			case handling <- struct{}{}:
			default:
			}
		},
	}
	loop := events.NewEventLoop(eventCh, handler, logr.Discard())

	ctx, cancel := context.WithCancel(t.Context())

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Start(ctx)
	}()

	// Send one event to start handling.
	eventCh <- "event-1"

	// Wait until handler is actively processing.
	g.Eventually(handling).Should(Receive())

	// Unblock handler with no pending events.
	close(blockCh)

	// Wait for first batch to complete.
	g.Eventually(handler.getBatches, 2*time.Second, 10*time.Millisecond).
		Should(HaveLen(1))

	// No extra batches should appear.
	g.Consistently(handler.getBatches, 100*time.Millisecond, 10*time.Millisecond).
		Should(HaveLen(1))

	cancel()
	g.Eventually(loopDone, 2*time.Second).Should(Receive(BeNil()))
}
