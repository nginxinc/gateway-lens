package app

import (
	"github.com/sjberman/gateway-lens/internal/topology"
)

// liveResourcesReader provides read access to the latest synchronized Gateway API resources.
type liveResourcesReader interface {
	// Get returns a snapshot of the current Gateway API resources.
	Get() topology.GatewayAPIResources
	// Subscribe returns a channel that receives a notification each time the resource store changes.
	Subscribe() <-chan struct{}
	// Unsubscribe removes a previously subscribed channel and closes it.
	Unsubscribe(ch <-chan struct{})
}
