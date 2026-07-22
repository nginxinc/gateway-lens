package topology

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const gatewayAPIGroup = gatewayv1.GroupName

const (
	defaultGatewayKind       = "Gateway"
	defaultServiceKind       = "Service"
	wildcardRefName          = "*"
	kindReferenceGrant       = "ReferenceGrant"
	kindBackendTLSPolicy     = "BackendTLSPolicy"
	detailGatewayClass       = "gatewayClass"
	detailParentRef          = "parentRef"
	detailBackendRef         = "backendRef"
	detailReferenceGrantFrom = "referenceGrantFrom"
	detailReferenceGrantTo   = "referenceGrantTo"
	detailTargetRef          = "targetRef"
	detailExtensionRef       = "extensionRef"
	detailParametersRef      = "parametersRef"
	attributePolicyType      = "policyType"
)

// TranslateGatewayAPI builds a base topology snapshot from typed Gateway API resources.
func TranslateGatewayAPI(resources GatewayAPIResources) Snapshot {
	builder := newBaseSnapshotBuilder()

	builder.addGatewayClassNodes(resources.GatewayClasses)
	builder.addGatewayNodes(resources.Gateways)
	builder.addHTTPRouteNodes(resources.HTTPRoutes)
	builder.addGRPCRouteNodes(resources.GRPCRoutes)
	builder.addTLSRouteNodes(resources.TLSRoutes)
	builder.addTCPRouteNodes(resources.TCPRoutes)
	builder.addUDPRouteNodes(resources.UDPRoutes)
	builder.addReferenceGrantNodes(resources.ReferenceGrants)
	builder.addBackendTLSPolicyNodes(resources.BackendTLSPolicies)
	builder.addPolicyNodes(resources.Policies)

	return Snapshot{Nodes: builder.nodes, Edges: builder.edges}
}

// baseSnapshotBuilder accumulates nodes and edges while building the base topology.
type baseSnapshotBuilder struct {
	// nodes is the ordered list of topology nodes.
	nodes []Node
	// edges is the ordered list of topology edges.
	edges []Edge
	// nodeSet maps each resource ref to its index in nodes for deduplication.
	nodeSet map[ResourceRef]int
}

// newBaseSnapshotBuilder creates an empty builder.
func newBaseSnapshotBuilder() *baseSnapshotBuilder {
	return &baseSnapshotBuilder{
		nodes:   make([]Node, 0),
		edges:   make([]Edge, 0),
		nodeSet: make(map[ResourceRef]int),
	}
}

// addNode adds a node if the reference is valid and not already present.
func (b *baseSnapshotBuilder) addNode(ref ResourceRef) {
	if !ref.IsValid() {
		return
	}

	if _, exists := b.nodeSet[ref]; exists {
		return
	}

	b.nodeSet[ref] = len(b.nodes)
	b.nodes = append(b.nodes, Node{Ref: ref})
}

// setNodeConditions replaces the conditions on an existing node.
func (b *baseSnapshotBuilder) setNodeConditions(ref ResourceRef, conditions []metav1.Condition) {
	index, exists := b.nodeSet[ref]
	if !exists {
		return
	}

	b.nodes[index].Conditions = normalizeConditions(conditions)
}

// appendNodeConditions appends additional conditions to an existing node.
func (b *baseSnapshotBuilder) appendNodeConditions(ref ResourceRef, conditions []Condition) {
	index, exists := b.nodeSet[ref]
	if !exists {
		return
	}

	b.nodes[index].Conditions = append(b.nodes[index].Conditions, conditions...)
}

// normalizeConditions converts Kubernetes status conditions to the topology domain type.
func normalizeConditions(conditions []metav1.Condition) []Condition {
	normalized := make([]Condition, len(conditions))
	for idx, condition := range conditions {
		normalized[idx] = Condition{
			Type:    condition.Type,
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		}
	}

	return normalized
}

// flattenPolicyAncestorConditions collects conditions from all PolicyAncestorStatus entries.
func flattenPolicyAncestorConditions(ancestors []gatewayv1.PolicyAncestorStatus) []metav1.Condition {
	var conditions []metav1.Condition
	for _, ancestor := range ancestors {
		conditions = append(conditions, ancestor.Conditions...)
	}

	return conditions
}

// extractUnstructuredConditions extracts conditions from an unstructured object.
// It checks both the standard .status.conditions path and the Gateway API policy
// .status.ancestors[].conditions path.
func extractUnstructuredConditions(obj unstructured.Unstructured) []Condition {
	statusRaw, ok := obj.Object["status"]
	if !ok {
		return nil
	}

	statusMap, ok := statusRaw.(map[string]any)
	if !ok {
		return nil
	}

	// Try .status.conditions first (standard Kubernetes pattern).
	if conditions := parseConditionsList(statusMap["conditions"]); len(conditions) > 0 {
		return conditions
	}

	// Fall back to .status.ancestors[].conditions (Gateway API policy pattern).
	ancestorsRaw, ok := statusMap["ancestors"]
	if !ok {
		return nil
	}

	ancestorsList, ok := ancestorsRaw.([]any)
	if !ok {
		return nil
	}

	var conditions []Condition

	for _, ancestorRaw := range ancestorsList {
		ancestorMap, ok := ancestorRaw.(map[string]any)
		if !ok {
			continue
		}

		conditions = append(conditions, parseConditionsList(ancestorMap["conditions"])...)
	}

	return conditions
}

// parseConditionsList converts a raw []any of condition maps into typed Conditions.
func parseConditionsList(raw any) []Condition {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}

	conditions := make([]Condition, 0, len(list))

	for _, item := range list {
		condMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		conditions = append(conditions, Condition{
			Type:    stringField(condMap, "type"),
			Status:  stringField(condMap, "status"),
			Reason:  stringField(condMap, "reason"),
			Message: stringField(condMap, "message"),
		})
	}

	return conditions
}

func stringField(m map[string]any, key string) string {
	val, ok := m[key].(string)
	if !ok {
		return ""
	}

	return val
}

// addEdge adds a directed edge, implicitly creating endpoint nodes if needed.
func (b *baseSnapshotBuilder) addEdge(from, to ResourceRef, edgeType EdgeType, detail string) {
	edge := Edge{From: from, To: to, Type: edgeType, Detail: detail}
	if !edge.IsValid() {
		return
	}

	b.addNode(from)
	b.addNode(to)
	b.edges = append(b.edges, edge)
}

// addGatewayClassNodes adds GatewayClass nodes with their status conditions.
func (b *baseSnapshotBuilder) addGatewayClassNodes(gatewayClasses []gatewayv1.GatewayClass) {
	for _, gatewayClass := range gatewayClasses {
		ref := ResourceRef{Group: gatewayAPIGroup, Kind: "GatewayClass", Name: gatewayClass.Name}
		b.addNode(ref)
		b.setNodeConditions(ref, gatewayClass.Status.Conditions)

		if gatewayClass.Spec.ParametersRef != nil {
			paramsRef := resolveParametersRef(gatewayClass.Spec.ParametersRef)
			b.addEdge(ref, paramsRef, EdgeTypeParametersRef, detailParametersRef)
		}
	}
}

// addGatewayNodes adds Gateway nodes with conditions, listener conditions, and gatewayClass edges.
func (b *baseSnapshotBuilder) addGatewayNodes(gateways []gatewayv1.Gateway) {
	for _, gateway := range gateways {
		gatewayRef := ResourceRef{
			Group:     gatewayAPIGroup,
			Kind:      "Gateway",
			Namespace: gateway.Namespace,
			Name:      gateway.Name,
		}
		b.addNode(gatewayRef)
		b.setNodeConditions(gatewayRef, gateway.Status.Conditions)
		b.appendListenerConditions(gatewayRef, gateway.Status.Listeners)

		if gateway.Spec.GatewayClassName == "" {
			continue
		}

		gatewayClassRef := ResourceRef{
			Group: gatewayAPIGroup,
			Kind:  "GatewayClass",
			Name:  string(gateway.Spec.GatewayClassName),
		}
		b.addEdge(gatewayRef, gatewayClassRef, EdgeTypeAttachment, detailGatewayClass)

		if gateway.Spec.Infrastructure != nil && gateway.Spec.Infrastructure.ParametersRef != nil {
			paramsRef := resolveLocalParametersRef(
				gateway.Namespace,
				gateway.Spec.Infrastructure.ParametersRef,
			)
			b.addEdge(gatewayRef, paramsRef, EdgeTypeParametersRef, detailParametersRef)
		}
	}
}

// appendListenerConditions appends per-listener status conditions to a Gateway node,
// prefixing each condition type with "listener/<name>/".
func (b *baseSnapshotBuilder) appendListenerConditions(
	gatewayRef ResourceRef,
	listeners []gatewayv1.ListenerStatus,
) {
	for _, listener := range listeners {
		prefix := "listener/" + string(listener.Name) + "/"
		normalized := normalizeConditions(listener.Conditions)

		for idx := range normalized {
			normalized[idx].Type = prefix + normalized[idx].Type
		}

		b.appendNodeConditions(gatewayRef, normalized)
	}
}

// addHTTPRouteNodes adds HTTPRoute nodes with parent/backend/extensionRef edges and conditions.
//
//nolint:dupl // route methods share structure but differ in type-specific callbacks
func (b *baseSnapshotBuilder) addHTTPRouteNodes(httpRoutes []gatewayv1.HTTPRoute) {
	addRoutes(
		b,
		httpRoutes,
		"HTTPRoute",
		func(route gatewayv1.HTTPRoute) string { return route.Namespace },
		func(route gatewayv1.HTTPRoute) string { return route.Name },
		func(route gatewayv1.HTTPRoute, onParent func(ResourceRef)) {
			for _, parentRef := range route.Spec.ParentRefs {
				onParent(resolveParentRef(route.Namespace, parentRef))
			}
		},
		func(route gatewayv1.HTTPRoute, onBackend func(ResourceRef)) {
			for _, rule := range route.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					onBackend(resolveHTTPBackendRef(route.Namespace, backendRef))
				}
			}
		},
		func(route gatewayv1.HTTPRoute, onExtRef func(ResourceRef)) {
			iterHTTPRouteExtensionRefs(route, onExtRef)
		},
		func(route gatewayv1.HTTPRoute) []metav1.Condition {
			return flattenRouteParentConditions(route.Status.Parents)
		},
	)
}

// addGRPCRouteNodes adds GRPCRoute nodes with parent/backend/extensionRef edges and conditions.
//
//nolint:dupl // route methods share structure but differ in type-specific callbacks
func (b *baseSnapshotBuilder) addGRPCRouteNodes(grpcRoutes []gatewayv1.GRPCRoute) {
	addRoutes(
		b,
		grpcRoutes,
		"GRPCRoute",
		func(route gatewayv1.GRPCRoute) string { return route.Namespace },
		func(route gatewayv1.GRPCRoute) string { return route.Name },
		func(route gatewayv1.GRPCRoute, onParent func(ResourceRef)) {
			for _, parentRef := range route.Spec.ParentRefs {
				onParent(resolveParentRef(route.Namespace, parentRef))
			}
		},
		func(route gatewayv1.GRPCRoute, onBackend func(ResourceRef)) {
			for _, rule := range route.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					onBackend(resolveGRPCBackendRef(route.Namespace, backendRef))
				}
			}
		},
		func(route gatewayv1.GRPCRoute, onExtRef func(ResourceRef)) {
			iterGRPCRouteExtensionRefs(route, onExtRef)
		},
		func(route gatewayv1.GRPCRoute) []metav1.Condition {
			return flattenRouteParentConditions(route.Status.Parents)
		},
	)
}

// addTLSRouteNodes adds TLSRoute nodes with parent/backend edges and conditions.
//
//nolint:dupl // route methods share structure but differ in type-specific callbacks
func (b *baseSnapshotBuilder) addTLSRouteNodes(tlsRoutes []gatewayv1.TLSRoute) {
	addRoutes(
		b,
		tlsRoutes,
		"TLSRoute",
		func(route gatewayv1.TLSRoute) string { return route.Namespace },
		func(route gatewayv1.TLSRoute) string { return route.Name },
		func(route gatewayv1.TLSRoute, onParent func(ResourceRef)) {
			for _, parentRef := range route.Spec.ParentRefs {
				onParent(resolveParentRef(route.Namespace, parentRef))
			}
		},
		func(route gatewayv1.TLSRoute, onBackend func(ResourceRef)) {
			for _, rule := range route.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					onBackend(resolveBackendRef(route.Namespace, backendRef))
				}
			}
		},
		nil, // TLS routes do not have filters
		func(route gatewayv1.TLSRoute) []metav1.Condition {
			return flattenRouteParentConditions(route.Status.Parents)
		},
	)
}

// addTCPRouteNodes adds TCPRoute nodes with parent/backend edges and conditions.
//
//nolint:dupl // route methods share structure but differ in type-specific callbacks
func (b *baseSnapshotBuilder) addTCPRouteNodes(tcpRoutes []gatewayv1.TCPRoute) {
	addRoutes(
		b,
		tcpRoutes,
		"TCPRoute",
		func(route gatewayv1.TCPRoute) string { return route.Namespace },
		func(route gatewayv1.TCPRoute) string { return route.Name },
		func(route gatewayv1.TCPRoute, onParent func(ResourceRef)) {
			for _, parentRef := range route.Spec.ParentRefs {
				onParent(resolveParentRef(route.Namespace, parentRef))
			}
		},
		func(route gatewayv1.TCPRoute, onBackend func(ResourceRef)) {
			for _, rule := range route.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					onBackend(resolveBackendRef(route.Namespace, backendRef))
				}
			}
		},
		nil, // TCP routes do not have filters
		func(route gatewayv1.TCPRoute) []metav1.Condition {
			return flattenRouteParentConditions(route.Status.Parents)
		},
	)
}

// addUDPRouteNodes adds UDPRoute nodes with parent/backend edges and conditions.
//
//nolint:dupl // route methods share structure but differ in type-specific callbacks
func (b *baseSnapshotBuilder) addUDPRouteNodes(udpRoutes []gatewayv1.UDPRoute) {
	addRoutes(
		b,
		udpRoutes,
		"UDPRoute",
		func(route gatewayv1.UDPRoute) string { return route.Namespace },
		func(route gatewayv1.UDPRoute) string { return route.Name },
		func(route gatewayv1.UDPRoute, onParent func(ResourceRef)) {
			for _, parentRef := range route.Spec.ParentRefs {
				onParent(resolveParentRef(route.Namespace, parentRef))
			}
		},
		func(route gatewayv1.UDPRoute, onBackend func(ResourceRef)) {
			for _, rule := range route.Spec.Rules {
				for _, backendRef := range rule.BackendRefs {
					onBackend(resolveBackendRef(route.Namespace, backendRef))
				}
			}
		},
		nil, // UDP routes do not have filters
		func(route gatewayv1.UDPRoute) []metav1.Condition {
			return flattenRouteParentConditions(route.Status.Parents)
		},
	)
}

// addReferenceGrantNodes adds ReferenceGrant nodes with from/to attachment edges.
func (b *baseSnapshotBuilder) addReferenceGrantNodes(referenceGrants []gatewayv1.ReferenceGrant) {
	for _, referenceGrant := range referenceGrants {
		grantRef := ResourceRef{
			Group:     gatewayAPIGroup,
			Kind:      kindReferenceGrant,
			Namespace: referenceGrant.Namespace,
			Name:      referenceGrant.Name,
		}
		b.addNode(grantRef)

		for _, from := range referenceGrant.Spec.From {
			fromRef := resolveReferenceGrantFromRef(from)
			b.addEdge(fromRef, grantRef, EdgeTypeAttachment, detailReferenceGrantFrom)
		}

		for _, to := range referenceGrant.Spec.To {
			toRef := resolveReferenceGrantToRef(referenceGrant.Namespace, to)
			b.addEdge(grantRef, toRef, EdgeTypeAttachment, detailReferenceGrantTo)
		}
	}
}

// addBackendTLSPolicyNodes adds BackendTLSPolicy nodes with targetRef edges.
func (b *baseSnapshotBuilder) addBackendTLSPolicyNodes(policies []gatewayv1.BackendTLSPolicy) {
	for _, policy := range policies {
		policyRef := ResourceRef{
			Group:     gatewayAPIGroup,
			Kind:      kindBackendTLSPolicy,
			Namespace: policy.Namespace,
			Name:      policy.Name,
		}
		b.addNode(policyRef)
		b.setNodeConditions(policyRef, flattenPolicyAncestorConditions(policy.Status.Ancestors))

		for _, targetRef := range policy.Spec.TargetRefs {
			targetNode := ResourceRef{
				Group:     string(targetRef.Group),
				Kind:      string(targetRef.Kind),
				Namespace: policy.Namespace,
				Name:      string(targetRef.Name),
			}
			b.addEdge(policyRef, targetNode, EdgeTypePolicyTargetRef, detailTargetRef)
		}
	}
}

// addPolicyNodes adds dynamically discovered policy nodes with targetRef edges and policyType attributes.
func (b *baseSnapshotBuilder) addPolicyNodes(policies []Policy) {
	for _, policy := range policies {
		policyRef := ResourceRef{
			Group:     policy.Object.GetObjectKind().GroupVersionKind().Group,
			Kind:      policy.Object.GetObjectKind().GroupVersionKind().Kind,
			Namespace: policy.Object.GetNamespace(),
			Name:      policy.Object.GetName(),
		}
		b.addNode(policyRef)
		b.setNodeAttributes(policyRef, map[string]string{attributePolicyType: string(policy.Type)})
		b.appendNodeConditions(policyRef, extractUnstructuredConditions(policy.Object))

		for _, targetRef := range policy.TargetRefs {
			targetNode := ResourceRef(targetRef)
			b.addEdge(policyRef, targetNode, EdgeTypePolicyTargetRef, detailTargetRef)
		}
	}
}

// setNodeAttributes sets the attributes on an existing node.
func (b *baseSnapshotBuilder) setNodeAttributes(ref ResourceRef, attributes map[string]string) {
	index, exists := b.nodeSet[ref]
	if !exists {
		return
	}

	b.nodes[index].Attributes = attributes
}

// addRoutes is a generic helper that adds route nodes, parent/backend/extensionRef edges, and conditions.
func addRoutes[T any](
	b *baseSnapshotBuilder,
	routes []T,
	routeKind string,
	routeNamespace func(T) string,
	routeName func(T) string,
	iterParentRefs func(T, func(ResourceRef)),
	iterBackendRefs func(T, func(ResourceRef)),
	iterExtensionRefs func(T, func(ResourceRef)),
	routeConditions func(T) []metav1.Condition,
) {
	for _, route := range routes {
		routeRef := ResourceRef{
			Group:     gatewayAPIGroup,
			Kind:      routeKind,
			Namespace: routeNamespace(route),
			Name:      routeName(route),
		}
		b.addNode(routeRef)
		b.setNodeConditions(routeRef, routeConditions(route))

		iterParentRefs(route, func(parentRef ResourceRef) {
			b.addEdge(routeRef, parentRef, EdgeTypeParentRef, detailParentRef)
		})

		iterBackendRefs(route, func(backendRef ResourceRef) {
			b.addEdge(routeRef, backendRef, EdgeTypeBackendRef, detailBackendRef)
		})

		if iterExtensionRefs != nil {
			iterExtensionRefs(route, func(extRef ResourceRef) {
				b.addEdge(routeRef, extRef, EdgeTypeExtensionRef, detailExtensionRef)
			})
		}
	}
}

// flattenRouteParentConditions collects conditions from all parent statuses into a single slice.
func flattenRouteParentConditions(parents []gatewayv1.RouteParentStatus) []metav1.Condition {
	var conditions []metav1.Condition
	for _, parent := range parents {
		conditions = append(conditions, parent.Conditions...)
	}

	return conditions
}

// resolveParentRef resolves a v1 ParentReference to a ResourceRef, applying Gateway API defaults.
func resolveParentRef(routeNamespace string, parentRef gatewayv1.ParentReference) ResourceRef {
	parentNamespace := routeNamespace
	if parentRef.Namespace != nil {
		parentNamespace = string(*parentRef.Namespace)
	}

	parentKind := defaultGatewayKind
	if parentRef.Kind != nil {
		parentKind = string(*parentRef.Kind)
	}

	parentGroup := gatewayAPIGroup
	if parentRef.Group != nil {
		parentGroup = string(*parentRef.Group)
	}

	return ResourceRef{
		Group:     parentGroup,
		Kind:      parentKind,
		Namespace: parentNamespace,
		Name:      string(parentRef.Name),
	}
}

// resolveHTTPBackendRef resolves an HTTPBackendRef to a ResourceRef.
func resolveHTTPBackendRef(routeNamespace string, backendRef gatewayv1.HTTPBackendRef) ResourceRef {
	return resolveBackendRef(routeNamespace, backendRef.BackendRef)
}

// resolveGRPCBackendRef resolves a GRPCBackendRef to a ResourceRef.
func resolveGRPCBackendRef(routeNamespace string, backendRef gatewayv1.GRPCBackendRef) ResourceRef {
	return resolveBackendRef(routeNamespace, backendRef.BackendRef)
}

// resolveBackendRef resolves a v1 BackendRef to a ResourceRef, defaulting to Service.
func resolveBackendRef(routeNamespace string, backendRef gatewayv1.BackendRef) ResourceRef {
	backendNamespace := routeNamespace
	if backendRef.Namespace != nil {
		backendNamespace = string(*backendRef.Namespace)
	}

	backendKind := defaultServiceKind
	if backendRef.Kind != nil {
		backendKind = string(*backendRef.Kind)
	}

	backendGroup := ""
	if backendRef.Group != nil {
		backendGroup = string(*backendRef.Group)
	}

	return ResourceRef{
		Group:     backendGroup,
		Kind:      backendKind,
		Namespace: backendNamespace,
		Name:      string(backendRef.Name),
	}
}

// iterHTTPRouteExtensionRefs iterates over ExtensionRef filters in an HTTPRoute,
// scanning both rule-level and backend-ref-level filters.
func iterHTTPRouteExtensionRefs(route gatewayv1.HTTPRoute, onExtRef func(ResourceRef)) {
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if filter.Type == gatewayv1.HTTPRouteFilterExtensionRef && filter.ExtensionRef != nil {
				onExtRef(resolveExtensionRef(route.Namespace, *filter.ExtensionRef))
			}
		}

		for _, backendRef := range rule.BackendRefs {
			for _, filter := range backendRef.Filters {
				if filter.Type == gatewayv1.HTTPRouteFilterExtensionRef && filter.ExtensionRef != nil {
					onExtRef(resolveExtensionRef(route.Namespace, *filter.ExtensionRef))
				}
			}
		}
	}
}

// iterGRPCRouteExtensionRefs iterates over ExtensionRef filters in a GRPCRoute,
// scanning both rule-level and backend-ref-level filters.
func iterGRPCRouteExtensionRefs(route gatewayv1.GRPCRoute, onExtRef func(ResourceRef)) {
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if filter.Type == gatewayv1.GRPCRouteFilterExtensionRef && filter.ExtensionRef != nil {
				onExtRef(resolveExtensionRef(route.Namespace, *filter.ExtensionRef))
			}
		}

		for _, backendRef := range rule.BackendRefs {
			for _, filter := range backendRef.Filters {
				if filter.Type == gatewayv1.GRPCRouteFilterExtensionRef && filter.ExtensionRef != nil {
					onExtRef(resolveExtensionRef(route.Namespace, *filter.ExtensionRef))
				}
			}
		}
	}
}

// resolveExtensionRef resolves a LocalObjectReference to a ResourceRef.
// The namespace is inherited from the route since LocalObjectReference is namespace-local.
func resolveExtensionRef(routeNamespace string, ref gatewayv1.LocalObjectReference) ResourceRef {
	return ResourceRef{
		Group:     string(ref.Group),
		Kind:      string(ref.Kind),
		Namespace: routeNamespace,
		Name:      string(ref.Name),
	}
}

// resolveParametersRef resolves a GatewayClass ParametersReference to a ResourceRef.
// Namespace is optional; when nil, the target is cluster-scoped.
func resolveParametersRef(ref *gatewayv1.ParametersReference) ResourceRef {
	namespace := ""
	if ref.Namespace != nil {
		namespace = string(*ref.Namespace)
	}

	return ResourceRef{
		Group:     string(ref.Group),
		Kind:      string(ref.Kind),
		Namespace: namespace,
		Name:      ref.Name,
	}
}

// resolveLocalParametersRef resolves a Gateway LocalParametersReference to a ResourceRef.
// The namespace is inherited from the Gateway since LocalParametersReference is namespace-local.
func resolveLocalParametersRef(
	gatewayNamespace string,
	ref *gatewayv1.LocalParametersReference,
) ResourceRef {
	return ResourceRef{
		Group:     string(ref.Group),
		Kind:      string(ref.Kind),
		Namespace: gatewayNamespace,
		Name:      ref.Name,
	}
}

// resolveReferenceGrantFromRef converts a ReferenceGrant "from" clause to a wildcard ResourceRef.
func resolveReferenceGrantFromRef(from gatewayv1.ReferenceGrantFrom) ResourceRef {
	return ResourceRef{
		Group:     string(from.Group),
		Kind:      string(from.Kind),
		Namespace: string(from.Namespace),
		Name:      wildcardRefName,
	}
}

// resolveReferenceGrantToRef converts a ReferenceGrant "to" clause to a ResourceRef.
func resolveReferenceGrantToRef(namespace string, to gatewayv1.ReferenceGrantTo) ResourceRef {
	resourceName := wildcardRefName
	if to.Name != nil {
		resourceName = string(*to.Name)
	}

	return ResourceRef{
		Group:     string(to.Group),
		Kind:      string(to.Kind),
		Namespace: namespace,
		Name:      resourceName,
	}
}
