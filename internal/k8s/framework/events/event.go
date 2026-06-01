// Package events provides generic event types for controller-driven state updates.
package events

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EventBatch groups related events for atomic handler processing.
type EventBatch []any

// UpsertEvent indicates a resource was created or updated.
type UpsertEvent struct {
	// Resource is the created or updated Kubernetes object.
	Resource client.Object
}

// DeleteEvent indicates a resource was deleted.
type DeleteEvent struct {
	// Type is a zero-value object identifying the resource kind.
	Type           client.Object
	// NamespacedName is the identity of the deleted resource.
	NamespacedName types.NamespacedName
}
