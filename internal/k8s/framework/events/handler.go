package events

import "context"

// EventHandler processes event batches produced by the event loop.
type EventHandler interface {
	HandleEventBatch(ctx context.Context, batch EventBatch)
}
