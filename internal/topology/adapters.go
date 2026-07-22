package topology

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// PolicyType describes whether a policy applies directly to its target or is inherited.
type PolicyType string

const (
	// PolicyTypeDirect indicates a policy that applies directly to its target resource.
	PolicyTypeDirect PolicyType = "direct"
	// PolicyTypeInherited indicates a policy that is inherited by child resources of its target.
	PolicyTypeInherited PolicyType = "inherited"
)

// Policy holds a discovered unstructured Gateway API policy and its parsed metadata.
type Policy struct {
	// Object is the raw unstructured policy resource from the cluster.
	Object unstructured.Unstructured
	// TargetRefs are the resolved policy target references extracted from the spec.
	TargetRefs []PolicyTargetRef
	// Type classifies the policy as direct or inherited based on the CRD label.
	Type PolicyType
}

// PolicyTargetRef identifies a resource targeted by a policy.
type PolicyTargetRef struct {
	// Group is the API group of the target resource.
	Group string
	// Kind is the resource kind of the target.
	Kind string
	// Namespace is the target resource namespace; empty if omitted.
	Namespace string
	// Name is the target resource name.
	Name string
}

// GatewayAPIResources contains typed Gateway API resources for assembly.
type GatewayAPIResources struct {
	// GatewayClasses holds the watched GatewayClass resources.
	GatewayClasses     []gatewayv1.GatewayClass
	// Gateways holds the watched Gateway resources.
	Gateways           []gatewayv1.Gateway
	// HTTPRoutes holds the watched HTTPRoute resources.
	HTTPRoutes         []gatewayv1.HTTPRoute
	// GRPCRoutes holds the watched GRPCRoute resources.
	GRPCRoutes         []gatewayv1.GRPCRoute
	// TLSRoutes holds the watched TLSRoute resources.
	TLSRoutes          []gatewayv1.TLSRoute
	// TCPRoutes holds the watched TCPRoute resources.
	TCPRoutes          []gatewayv1.TCPRoute
	// UDPRoutes holds the watched UDPRoute resources.
	UDPRoutes          []gatewayv1.UDPRoute
	// ReferenceGrants holds the watched ReferenceGrant resources.
	ReferenceGrants    []gatewayv1.ReferenceGrant
	// BackendTLSPolicies holds the watched BackendTLSPolicy resources.
	BackendTLSPolicies []gatewayv1.BackendTLSPolicy
	// ListenerSets holds the watched ListenerSet resources.
	ListenerSets       []gatewayv1.ListenerSet
	// Policies holds dynamically discovered unstructured policy resources.
	Policies           []Policy
	// ExtensionRefs holds dynamically discovered ExtensionRef CRD instances.
	ExtensionRefs      []unstructured.Unstructured
	// ParametersRefs holds dynamically discovered ParametersRef CRD instances.
	ParametersRefs     []unstructured.Unstructured
}

// PolicyAdapter enriches a topology snapshot with implementor-specific metadata.
type PolicyAdapter interface {
	// Name returns the adapter's identifier used in annotation sources.
	Name() string
	// Annotate produces metadata annotations for the given snapshot and resources.
	Annotate(resources GatewayAPIResources, snapshot Snapshot) ([]NodeAnnotation, error)
}
