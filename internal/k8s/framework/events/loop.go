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

package events

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
)

var errHandlerPanicked = errors.New("handler panicked")

// EventLoop reads events, batches them, and invokes a handler.
//
// When a new event arrives, there are two cases:
//   - If no batch is currently being handled, a new batch is started immediately.
//   - Otherwise, the event is saved for later. All saved events are handled together
//     after the current batch finishes.
//
// The loop uses double buffering: incoming events always land in nextBatch while the
// handler goroutine reads from currentBatch. On each cycle the two are swapped so
// their backing arrays are reused without allocation.
type EventLoop struct {
	// handler processes completed event batches.
	handler EventHandler
	// eventCh is the channel from which raw events are read.
	eventCh <-chan any
	// logger is the structured logger for loop lifecycle events.
	logger  logr.Logger

	// currentBatch holds the batch being processed by the handler.
	currentBatch EventBatch
	// nextBatch accumulates events while the current batch is being handled.
	nextBatch    EventBatch

	// currentBatchID is a monotonically increasing batch counter for logging.
	currentBatchID int
}

// NewEventLoop creates an event loop that forwards events to the provided handler.
func NewEventLoop(eventCh <-chan any, handler EventHandler, logger logr.Logger) *EventLoop {
	return &EventLoop{
		handler:      handler,
		eventCh:      eventCh,
		logger:       logger.WithName("eventLoop"),
		currentBatch: make(EventBatch, 0),
		nextBatch:    make(EventBatch, 0),
	}
}

// Start runs the event loop until context cancellation.
func (l *EventLoop) Start(ctx context.Context) error {
	l.logger.Info("Starting event loop")

	var handling bool

	handlingDone := make(chan struct{})

	swapAndHandle := func() {
		l.swapBatches()
		l.handleBatch(ctx, handlingDone)

		handling = true
	}

	for {
		select {
		case <-ctx.Done():
			if handling {
				<-handlingDone
			}

			l.logger.Info("Stopping event loop")

			return nil
		case event := <-l.eventCh:
			l.nextBatch = append(l.nextBatch, event)

			l.logger.V(1).Info("Added event to next batch",
				"type", fmt.Sprintf("%T", event),
				"total", len(l.nextBatch),
			)

			if !handling {
				swapAndHandle()
			}
		case <-handlingDone:
			handling = false

			if len(l.nextBatch) > 0 {
				swapAndHandle()
			}
		}
	}
}

// handleBatch dispatches the current batch to the handler in a goroutine.
func (l *EventLoop) handleBatch(ctx context.Context, done chan<- struct{}) {
	l.currentBatchID++

	batchLogger := l.logger.WithValues("batchID", l.currentBatchID)

	go func(batch EventBatch) {
		defer func() {
			if r := recover(); r != nil {
				batchLogger.Error(
					fmt.Errorf("%w: %v", errHandlerPanicked, r),
					"Recovered from panic while handling event batch",
				)
			}

			done <- struct{}{}
		}()

		batchLogger.V(1).Info("Handling events", "total", len(batch))

		l.handler.HandleEventBatch(ctx, batch)

		batchLogger.V(1).Info("Finished handling event batch")
	}(l.currentBatch)
}

// swapBatches exchanges current and next batch slices, reusing backing arrays.
func (l *EventLoop) swapBatches() {
	l.currentBatch, l.nextBatch = l.nextBatch, l.currentBatch
	l.nextBatch = l.nextBatch[:0]
}
