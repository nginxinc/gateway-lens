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

// Package topology defines the Gateway API topology domain model.
package topology

import (
	"maps"
)

// ResourceRef identifies a Kubernetes resource in the graph.
type ResourceRef struct {
	// Group is the API group of the resource (e.g. "gateway.networking.k8s.io").
	Group string
	// Kind is the resource kind (e.g. "Gateway", "HTTPRoute").
	Kind string
	// Namespace is the resource namespace; empty for cluster-scoped resources.
	Namespace string
	// Name is the resource name.
	Name string
}

// IsNamespaced reports whether the referenced resource is namespace-scoped.
func (r ResourceRef) IsNamespaced() bool {
	return r.Namespace != ""
}

// IsValid reports whether the reference contains the minimum required identity.
func (r ResourceRef) IsValid() bool {
	if r.Kind == "" || r.Name == "" {
		return false
	}

	return true
}

// EdgeType identifies a relationship type between two resources.
type EdgeType string

const (
	// EdgeTypeParentRef represents a Route parentRef to Gateway relationship.
	EdgeTypeParentRef EdgeType = "parentRef"
	// EdgeTypeBackendRef represents a Route backendRef relationship.
	EdgeTypeBackendRef EdgeType = "backendRef"
	// EdgeTypeAttachment represents an attachment relationship.
	EdgeTypeAttachment EdgeType = "attachment"
	// EdgeTypePolicyTargetRef represents a policy targetRef relationship.
	EdgeTypePolicyTargetRef EdgeType = "policyTargetRef"
	// EdgeTypeExtensionRef represents a route extensionRef filter relationship.
	EdgeTypeExtensionRef EdgeType = "extensionRef"
	// EdgeTypeParametersRef represents a GatewayClass or Gateway parametersRef relationship.
	EdgeTypeParametersRef EdgeType = "parametersRef"
)

// Condition captures a normalized Kubernetes-style status condition.
type Condition struct {
	// Type is the condition type (e.g. "Accepted", "Programmed").
	Type    string
	// Status is the condition status ("True", "False", or "Unknown").
	Status  string
	// Reason is a machine-readable reason for the condition.
	Reason  string
	// Message is a human-readable description of the condition.
	Message string
}

// Node represents a resource in the topology graph.
type Node struct {
	// Ref is the identity of the Kubernetes resource.
	Ref        ResourceRef
	// Attributes are key-value metadata pairs for the node.
	Attributes map[string]string
	// Conditions are the normalized status conditions reported by the resource.
	Conditions []Condition
}

// IsValid reports whether the node has a valid resource identity.
func (n Node) IsValid() bool {
	return n.Ref.IsValid()
}

// Edge represents a directional relationship in the topology graph.
type Edge struct {
	// From is the source resource of the edge.
	From   ResourceRef
	// To is the destination resource of the edge.
	To     ResourceRef
	// Type categorizes the relationship.
	Type   EdgeType
	// Detail is an optional qualifier for the edge type.
	Detail string
}

// IsValid reports whether the edge endpoints are valid resource references.
func (e Edge) IsValid() bool {
	return e.From.IsValid() && e.To.IsValid()
}

// NodeAnnotation stores adapter-provided metadata for a resource node.
type NodeAnnotation struct {
	// Ref identifies the node this annotation belongs to.
	Ref    ResourceRef
	// Source is the adapter that produced this annotation.
	Source string
	// Key is the annotation key.
	Key    string
	// Value is the annotation value.
	Value  string
}

// Snapshot is the fully assembled topology for frontend consumption.
type Snapshot struct {
	// Nodes are the topology graph nodes.
	Nodes       []Node
	// Edges are the directional relationships between nodes.
	Edges       []Edge
	// Annotations are adapter-provided metadata entries attached to nodes.
	Annotations []NodeAnnotation
}

// Clone returns a deep copy of the snapshot.
func (s Snapshot) Clone() Snapshot {
	copiedNodes := make([]Node, len(s.Nodes))
	for idx, node := range s.Nodes {
		copiedNodes[idx] = cloneNode(node)
	}

	copiedEdges := make([]Edge, len(s.Edges))
	copy(copiedEdges, s.Edges)

	copiedAnnotations := make([]NodeAnnotation, len(s.Annotations))
	copy(copiedAnnotations, s.Annotations)

	return Snapshot{Nodes: copiedNodes, Edges: copiedEdges, Annotations: copiedAnnotations}
}

// HasNode reports whether a node with the given reference exists in the snapshot.
func (s Snapshot) HasNode(ref ResourceRef) bool {
	for _, node := range s.Nodes {
		if node.Ref == ref {
			return true
		}
	}

	return false
}

// NodeConditions returns the conditions for the node matching the given reference,
// or nil if the node is not found.
func (s Snapshot) NodeConditions(ref ResourceRef) []Condition {
	for _, node := range s.Nodes {
		if node.Ref == ref {
			return node.Conditions
		}
	}

	return nil
}

// cloneNode returns a deep copy of the given node.
func cloneNode(node Node) Node {
	copiedAttributes := make(map[string]string, len(node.Attributes))
	maps.Copy(copiedAttributes, node.Attributes)

	copiedConditions := make([]Condition, len(node.Conditions))
	copy(copiedConditions, node.Conditions)

	return Node{Ref: node.Ref, Attributes: copiedAttributes, Conditions: copiedConditions}
}
