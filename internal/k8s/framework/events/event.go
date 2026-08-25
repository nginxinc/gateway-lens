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
