package topology_test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/sjberman/gateway-lens/internal/topology"
)

const gatewayClassName = "edge-class"

const (
	kindGRPCRoute        = "GRPCRoute"
	kindGatewayClass     = "GatewayClass"
	kindTLSRoute         = "TLSRoute"
	kindTCPRoute         = "TCPRoute"
	kindUDPRoute         = "UDPRoute"
	kindReferenceGrant   = "ReferenceGrant"
	kindBackendTLSPolicy = "BackendTLSPolicy"
	detailParent         = "parentRef"
	detailBackend        = "backendRef"
	detailTargetRef      = "targetRef"
	wildcardRefName      = "*"

	conditionTrue           = "True"
	conditionTypeAccepted   = "Accepted"
	conditionTypeProgrammed = "Programmed"
	conditionTypeResolved   = "ResolvedRefs"
	msgAccepted             = "Resource accepted"
	msgProgrammed           = "Resource programmed"
	msgResolved             = "Refs resolved"

	groupExampleIO         = "example.io"
	groupSecurityExIO      = "security.example.io"
	kindRateLimitPolicy    = "RateLimitPolicy"
	kindAuthPolicy         = "AuthPolicy"
	controllerNameExample  = "example.com/controller"
)

type translateGatewayAPITestCase struct {
	name          string
	resources     topology.GatewayAPIResources
	expectedNodes []topology.ResourceRef
	expectedEdges []topology.Edge
}

func TestTranslateGatewayAPI(t *testing.T) {
	t.Parallel()

	for _, testCase := range translateGatewayAPITestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runTranslateGatewayAPITest(t, testCase)
		})
	}
}

func runTranslateGatewayAPITest(t *testing.T, testCase translateGatewayAPITestCase) {
	t.Helper()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(testCase.resources)

	for _, node := range testCase.expectedNodes {
		g.Expect(snapshot.HasNode(node)).To(BeTrue())
	}

	g.Expect(snapshot.Edges).To(ConsistOf(testCase.expectedEdges))
}

func translateGatewayAPITestCases() []translateGatewayAPITestCase {
	return []translateGatewayAPITestCase{
		baseTranslationCase(),
		overridesCase(),
		grpcRouteCase(),
		tlsRouteCase(),
		tcpRouteCase(),
		udpRouteCase(),
		referenceGrantCase(),
		backendTLSPolicyCase(),
		genericPolicyCase(),
		genericPolicyMultipleTargetRefsCase(),
		httpRouteExtensionRefCase(),
		httpRouteBackendExtensionRefCase(),
		grpcRouteExtensionRefCase(),
		gatewayClassParametersRefCase(),
		gatewayClassParametersRefClusterScopedCase(),
		gatewayParametersRefCase(),
	}
}

func baseTranslationCase() translateGatewayAPITestCase {
	return translateGatewayAPITestCase{
		name: "translates gateway class gateway and route relationships",
		resources: topology.GatewayAPIResources{
			GatewayClasses: []gatewayv1.GatewayClass{
				{ObjectMeta: metav1.ObjectMeta{Name: gatewayClassName}},
			},
			Gateways: []gatewayv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameEdge},
					Spec: gatewayv1.GatewaySpec{
						GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
					},
				},
			},
			HTTPRoutes: []gatewayv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{
								{Name: gatewayv1.ObjectName(nameEdge)},
							},
						},
						Rules: []gatewayv1.HTTPRouteRule{
							{
								BackendRefs: []gatewayv1.HTTPBackendRef{serviceBackendRef(nameBackend)},
							},
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{
			{Group: gatewayv1.GroupName, Kind: kindGatewayClass, Name: gatewayClassName},
			{
				Group:     gatewayv1.GroupName,
				Kind:      kindGateway,
				Namespace: namespaceDefault,
				Name:      nameEdge,
			},
			{
				Group:     gatewayv1.GroupName,
				Kind:      kindHTTPRoute,
				Namespace: namespaceDefault,
				Name:      nameRoute,
			},
			{Group: "", Kind: kindService, Namespace: namespaceDefault, Name: nameBackend},
		},
		expectedEdges: []topology.Edge{
			gatewayClassEdge(),
			defaultParentRefEdge(),
			defaultBackendRefEdge(),
		},
	}
}

func overridesCase() translateGatewayAPITestCase {
	return translateGatewayAPITestCase{
		name: "honors explicit namespace kind and group overrides",
		resources: topology.GatewayAPIResources{
			HTTPRoutes: []gatewayv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{customParentRef()},
						},
						Rules: []gatewayv1.HTTPRouteRule{
							{
								BackendRefs: []gatewayv1.HTTPBackendRef{customBackendRef()},
							},
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{
			customParentRefEdge(),
			customBackendRefEdge(),
		},
	}
}

func grpcRouteCase() translateGatewayAPITestCase {
	const (
		grpcRouteName   = "grpc-route"
		grpcBackendName = "grpc-backend"
	)

	return translateGatewayAPITestCase{
		name: "translates grpc route parent and backend references",
		resources: topology.GatewayAPIResources{
			GRPCRoutes: []gatewayv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: grpcRouteName},
					Spec: gatewayv1.GRPCRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(nameEdge)}},
						},
						Rules: []gatewayv1.GRPCRouteRule{
							{BackendRefs: []gatewayv1.GRPCBackendRef{grpcBackendRef(grpcBackendName)}},
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{
			routeParentRefEdge(kindGRPCRoute, grpcRouteName),
			routeBackendRefEdge(kindGRPCRoute, grpcRouteName, grpcBackendName),
		},
	}
}

func tlsRouteCase() translateGatewayAPITestCase {
	const (
		tlsRouteName   = "tls-route"
		tlsBackendName = "tls-backend"
	)

	return translateGatewayAPITestCase{
		name: "translates tls route parent and backend references",
		resources: topology.GatewayAPIResources{
			TLSRoutes: []gatewayv1.TLSRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: tlsRouteName},
					Spec: gatewayv1.TLSRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(nameEdge)}},
						},
						Rules: []gatewayv1.TLSRouteRule{
							{BackendRefs: []gatewayv1.BackendRef{serviceBackendObjectRef(tlsBackendName)}},
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{
			routeParentRefEdge(kindTLSRoute, tlsRouteName),
			routeBackendRefEdge(kindTLSRoute, tlsRouteName, tlsBackendName),
		},
	}
}

func tcpRouteCase() translateGatewayAPITestCase {
	const (
		tcpRouteName   = "tcp-route"
		tcpBackendName = "tcp-backend"
	)

	return translateGatewayAPITestCase{
		name: "translates tcp route parent and backend references",
		resources: topology.GatewayAPIResources{
			TCPRoutes: []gatewayv1.TCPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: tcpRouteName},
					Spec: gatewayv1.TCPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(nameEdge)}},
						},
						Rules: []gatewayv1.TCPRouteRule{
							{BackendRefs: []gatewayv1.BackendRef{serviceBackendObjectRefV1(tcpBackendName)}},
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{
			routeParentRefEdge(kindTCPRoute, tcpRouteName),
			routeBackendRefEdge(kindTCPRoute, tcpRouteName, tcpBackendName),
		},
	}
}

func udpRouteCase() translateGatewayAPITestCase {
	const (
		udpRouteName   = "udp-route"
		udpBackendName = "udp-backend"
	)

	return translateGatewayAPITestCase{
		name: "translates udp route parent and backend references",
		resources: topology.GatewayAPIResources{
			UDPRoutes: []gatewayv1.UDPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: udpRouteName},
					Spec: gatewayv1.UDPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName(nameEdge)}},
						},
						Rules: []gatewayv1.UDPRouteRule{
							{BackendRefs: []gatewayv1.BackendRef{serviceBackendObjectRefV1(udpBackendName)}},
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{
			routeParentRefEdge(kindUDPRoute, udpRouteName),
			routeBackendRefEdge(kindUDPRoute, udpRouteName, udpBackendName),
		},
	}
}

func referenceGrantCase() translateGatewayAPITestCase {
	const (
		grantNamespace = "security"
		grantName      = "allow-backend"
	)

	return translateGatewayAPITestCase{
		name: "translates reference grant from and to relationships",
		resources: topology.GatewayAPIResources{
			ReferenceGrants: []gatewayv1.ReferenceGrant{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: grantNamespace, Name: grantName},
					Spec: gatewayv1.ReferenceGrantSpec{
						From: []gatewayv1.ReferenceGrantFrom{
							{
								Group:     gatewayv1.Group(gatewayv1.GroupName),
								Kind:      gatewayv1.Kind(kindHTTPRoute),
								Namespace: gatewayv1.Namespace(namespaceDefault),
							},
						},
						To: []gatewayv1.ReferenceGrantTo{
							{
								Group: gatewayv1.Group(""),
								Kind:  gatewayv1.Kind(kindService),
								Name:  ptrToObjectName(nameBackend),
							},
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{
			referenceGrantFromEdge(grantNamespace, grantName),
			referenceGrantToEdge(grantNamespace, grantName),
		},
	}
}

func backendTLSPolicyCase() translateGatewayAPITestCase {
	const policyName = "backend-mtls"

	return translateGatewayAPITestCase{
		name: "translates backend tls policy target reference",
		resources: topology.GatewayAPIResources{
			BackendTLSPolicies: []gatewayv1.BackendTLSPolicy{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: policyName},
					Spec: gatewayv1.BackendTLSPolicySpec{
						TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
							{
								LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
									Group: gatewayv1.Group(""),
									Kind:  gatewayv1.Kind(kindService),
									Name:  gatewayv1.ObjectName(nameBackend),
								},
							},
						},
						Validation: gatewayv1.BackendTLSPolicyValidation{
							Hostname: gatewayv1.PreciseHostname("backend.internal"),
						},
					},
				},
			},
		},
		expectedEdges: []topology.Edge{backendTLSPolicyTargetEdge(policyName)},
	}
}

func serviceBackendRef(name string) gatewayv1.HTTPBackendRef {
	return gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(name),
			},
		},
	}
}

func grpcBackendRef(name string) gatewayv1.GRPCBackendRef {
	return gatewayv1.GRPCBackendRef{BackendRef: serviceBackendObjectRef(name)}
}

func serviceBackendObjectRef(name string) gatewayv1.BackendRef {
	return gatewayv1.BackendRef{
		BackendObjectReference: gatewayv1.BackendObjectReference{Name: gatewayv1.ObjectName(name)},
	}
}

func serviceBackendObjectRefV1(name string) gatewayv1.BackendRef {
	return gatewayv1.BackendRef{
		BackendObjectReference: gatewayv1.BackendObjectReference{Name: gatewayv1.ObjectName(name)},
	}
}

func customParentRef() gatewayv1.ParentReference {
	return gatewayv1.ParentReference{
		Name:      gatewayv1.ObjectName("ext-parent"),
		Namespace: ptrToNamespace("other"),
		Kind:      ptrToKind("CustomGateway"),
		Group:     ptrToGroup(groupExampleIO),
	}
}

func customBackendRef() gatewayv1.HTTPBackendRef {
	return gatewayv1.HTTPBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name:      gatewayv1.ObjectName("custom-backend"),
				Namespace: ptrToNamespace("backends"),
				Kind:      ptrToKind("Backend"),
				Group:     ptrToGroup("infra.example.io"),
			},
		},
	}
}

func gatewayClassEdge() topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindGateway,
			Namespace: namespaceDefault,
			Name:      nameEdge,
		},
		To:     topology.ResourceRef{Group: gatewayv1.GroupName, Kind: kindGatewayClass, Name: gatewayClassName},
		Type:   topology.EdgeTypeAttachment,
		Detail: "gatewayClass",
	}
}

func defaultParentRefEdge() topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindHTTPRoute,
			Namespace: namespaceDefault,
			Name:      nameRoute,
		},
		To: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindGateway,
			Namespace: namespaceDefault,
			Name:      nameEdge,
		},
		Type:   topology.EdgeTypeParentRef,
		Detail: detailParent,
	}
}

func defaultBackendRefEdge() topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindHTTPRoute,
			Namespace: namespaceDefault,
			Name:      nameRoute,
		},
		To:     topology.ResourceRef{Group: "", Kind: kindService, Namespace: namespaceDefault, Name: nameBackend},
		Type:   topology.EdgeTypeBackendRef,
		Detail: detailBackend,
	}
}

func customParentRefEdge() topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindHTTPRoute,
			Namespace: namespaceDefault,
			Name:      nameRoute,
		},
		To: topology.ResourceRef{
			Group:     groupExampleIO,
			Kind:      "CustomGateway",
			Namespace: "other",
			Name:      "ext-parent",
		},
		Type:   topology.EdgeTypeParentRef,
		Detail: detailParent,
	}
}

func customBackendRefEdge() topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindHTTPRoute,
			Namespace: namespaceDefault,
			Name:      nameRoute,
		},
		To: topology.ResourceRef{
			Group:     "infra.example.io",
			Kind:      "Backend",
			Namespace: "backends",
			Name:      "custom-backend",
		},
		Type:   topology.EdgeTypeBackendRef,
		Detail: detailBackend,
	}
}

func routeParentRefEdge(routeKind, routeName string) topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      routeKind,
			Namespace: namespaceDefault,
			Name:      routeName,
		},
		To: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindGateway,
			Namespace: namespaceDefault,
			Name:      nameEdge,
		},
		Type:   topology.EdgeTypeParentRef,
		Detail: detailParent,
	}
}

func routeBackendRefEdge(routeKind, routeName, backendName string) topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      routeKind,
			Namespace: namespaceDefault,
			Name:      routeName,
		},
		To: topology.ResourceRef{
			Group:     "",
			Kind:      kindService,
			Namespace: namespaceDefault,
			Name:      backendName,
		},
		Type:   topology.EdgeTypeBackendRef,
		Detail: detailBackend,
	}
}

func referenceGrantFromEdge(grantNamespace, grantName string) topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindHTTPRoute,
			Namespace: namespaceDefault,
			Name:      wildcardRefName,
		},
		To: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindReferenceGrant,
			Namespace: grantNamespace,
			Name:      grantName,
		},
		Type:   topology.EdgeTypeAttachment,
		Detail: "referenceGrantFrom",
	}
}

func referenceGrantToEdge(grantNamespace, grantName string) topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindReferenceGrant,
			Namespace: grantNamespace,
			Name:      grantName,
		},
		To: topology.ResourceRef{
			Group:     "",
			Kind:      kindService,
			Namespace: grantNamespace,
			Name:      nameBackend,
		},
		Type:   topology.EdgeTypeAttachment,
		Detail: "referenceGrantTo",
	}
}

func backendTLSPolicyTargetEdge(policyName string) topology.Edge {
	return topology.Edge{
		From: topology.ResourceRef{
			Group:     gatewayv1.GroupName,
			Kind:      kindBackendTLSPolicy,
			Namespace: namespaceDefault,
			Name:      policyName,
		},
		To: topology.ResourceRef{
			Group:     "",
			Kind:      kindService,
			Namespace: namespaceDefault,
			Name:      nameBackend,
		},
		Type:   topology.EdgeTypePolicyTargetRef,
		Detail: detailTargetRef,
	}
}

func ptrToNamespace(value string) *gatewayv1.Namespace {
	ns := gatewayv1.Namespace(value)

	return &ns
}

func ptrToKind(value string) *gatewayv1.Kind {
	kind := gatewayv1.Kind(value)

	return &kind
}

func ptrToGroup(value string) *gatewayv1.Group {
	group := gatewayv1.Group(value)

	return &group
}

func ptrToObjectName(value string) *gatewayv1.ObjectName {
	name := gatewayv1.ObjectName(value)

	return &name
}

func TestTranslateGatewayAPIConditions(t *testing.T) {
	t.Parallel()

	t.Run("gateway class conditions", testGatewayClassConditions)
	t.Run("gateway conditions with listener conditions", testGatewayListenerConditions)
	t.Run("gateway with no listener status", testGatewayNoListenerConditions)
	t.Run("http route conditions from parent status", testRouteParentConditions)
	t.Run("route conditions flattened from multiple parents", testRouteMultipleParentConditions)
	t.Run("route with no parent status conditions", testRouteNoConditions)
	t.Run("backend tls policy conditions from ancestors", testBackendTLSPolicyConditions)
	t.Run("dynamic policy conditions from unstructured status", testDynamicPolicyConditions)
	t.Run("dynamic policy conditions from ancestors path", testDynamicPolicyAncestorConditions)
}

func testGatewayClassConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		GatewayClasses: []gatewayv1.GatewayClass{
			{
				ObjectMeta: metav1.ObjectMeta{Name: gatewayClassName},
				Status: gatewayv1.GatewayClassStatus{
					Conditions: []metav1.Condition{acceptedCondition()},
				},
			},
		},
	})
	ref := topology.ResourceRef{Group: gatewayv1.GroupName, Kind: kindGatewayClass, Name: gatewayClassName}

	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{
		normalizedAccepted(),
	}))
}

func testGatewayListenerConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		Gateways: []gatewayv1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameEdge},
				Status: gatewayv1.GatewayStatus{
					Conditions: []metav1.Condition{acceptedCondition(), programmedCondition()},
					Listeners: []gatewayv1.ListenerStatus{
						{
							Name:       "http",
							Conditions: []metav1.Condition{acceptedCondition(), resolvedRefsCondition()},
						},
						{
							Name:       "https",
							Conditions: []metav1.Condition{programmedCondition()},
						},
					},
				},
			},
		},
	})
	ref := gatewayRef()

	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{
		normalizedAccepted(),
		normalizedProgrammed(),
		{
			Type: "listener/http/" + conditionTypeAccepted, Status: conditionTrue,
			Reason: conditionTypeAccepted, Message: msgAccepted,
		},
		{
			Type: "listener/http/" + conditionTypeResolved, Status: conditionTrue,
			Reason: conditionTypeResolved, Message: msgResolved,
		},
		{
			Type: "listener/https/" + conditionTypeProgrammed, Status: conditionTrue,
			Reason: conditionTypeProgrammed, Message: msgProgrammed,
		},
	}))
}

func testGatewayNoListenerConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		Gateways: []gatewayv1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameEdge},
				Status: gatewayv1.GatewayStatus{
					Conditions: []metav1.Condition{acceptedCondition()},
				},
			},
		},
	})
	ref := gatewayRef()

	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{normalizedAccepted()}))
}

func testRouteParentConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		HTTPRoutes: []gatewayv1.HTTPRoute{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{Name: gatewayv1.ObjectName(nameEdge)},
						},
					},
				},
				Status: gatewayv1.HTTPRouteStatus{
					RouteStatus: gatewayv1.RouteStatus{
						Parents: []gatewayv1.RouteParentStatus{
							{
								ParentRef:      gatewayv1.ParentReference{Name: gatewayv1.ObjectName(nameEdge)},
								ControllerName: "example.com/controller",
								Conditions:     []metav1.Condition{acceptedCondition(), resolvedRefsCondition()},
							},
						},
					},
				},
			},
		},
	})
	ref := httpRouteRef()

	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{
		normalizedAccepted(),
		normalizedResolvedRefs(),
	}))
}

func testRouteMultipleParentConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		HTTPRoutes: []gatewayv1.HTTPRoute{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{Name: gatewayv1.ObjectName(nameEdge)},
						},
					},
				},
				Status: gatewayv1.HTTPRouteStatus{
					RouteStatus: gatewayv1.RouteStatus{
						Parents: []gatewayv1.RouteParentStatus{
							{
								ParentRef:      gatewayv1.ParentReference{Name: gatewayv1.ObjectName(nameEdge)},
								ControllerName: "example.com/controller-a",
								Conditions:     []metav1.Condition{acceptedCondition()},
							},
							{
								ParentRef:      gatewayv1.ParentReference{Name: gatewayv1.ObjectName("other-gw")},
								ControllerName: "example.com/controller-b",
								Conditions:     []metav1.Condition{programmedCondition()},
							},
						},
					},
				},
			},
		},
	})
	ref := httpRouteRef()

	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{
		normalizedAccepted(),
		normalizedProgrammed(),
	}))
}

func testRouteNoConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		HTTPRoutes: []gatewayv1.HTTPRoute{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{Name: gatewayv1.ObjectName(nameEdge)},
						},
					},
				},
			},
		},
	})
	ref := httpRouteRef()

	g.Expect(snapshot.NodeConditions(ref)).To(BeEmpty())
}

func testBackendTLSPolicyConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	const policyName = "backend-mtls"

	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		BackendTLSPolicies: []gatewayv1.BackendTLSPolicy{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: policyName},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{
						{
							LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
								Group: gatewayv1.Group(""),
								Kind:  gatewayv1.Kind(kindService),
								Name:  gatewayv1.ObjectName(nameBackend),
							},
						},
					},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname: gatewayv1.PreciseHostname("backend.internal"),
					},
				},
				Status: gatewayv1.PolicyStatus{
					Ancestors: []gatewayv1.PolicyAncestorStatus{
						{
							AncestorRef:    gatewayv1.ParentReference{Name: gatewayv1.ObjectName(nameEdge)},
							ControllerName: "example.com/controller",
							Conditions:     []metav1.Condition{acceptedCondition()},
						},
					},
				},
			},
		},
	})

	ref := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindBackendTLSPolicy, Namespace: namespaceDefault, Name: policyName,
	}
	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{normalizedAccepted()}))
}

func testDynamicPolicyConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	const policyName = "rate-limit"

	obj := newTestPolicyUnstructured(groupExampleIO, "v1", kindRateLimitPolicy, policyName)
	obj.Object["status"] = map[string]any{
		"conditions": []any{
			map[string]any{
				"type": conditionTypeAccepted, "status": conditionTrue,
				"reason": conditionTypeAccepted, "message": msgAccepted,
			},
		},
	}

	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		Policies: []topology.Policy{
			{Object: obj, Type: topology.PolicyTypeDirect},
		},
	})

	ref := topology.ResourceRef{
		Group: groupExampleIO, Kind: kindRateLimitPolicy, Namespace: namespaceDefault, Name: policyName,
	}
	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{normalizedAccepted()}))
}

func testDynamicPolicyAncestorConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	const policyName = "auth-policy"

	obj := newTestPolicyUnstructured(groupSecurityExIO, "v1", kindAuthPolicy, policyName)
	obj.Object["status"] = map[string]any{
		"ancestors": []any{
			map[string]any{
				"ancestorRef":    map[string]any{"name": nameEdge},
				"controllerName": controllerNameExample,
				"conditions": []any{
					map[string]any{
						"type": conditionTypeAccepted, "status": conditionTrue,
						"reason": conditionTypeAccepted, "message": msgAccepted,
					},
				},
			},
		},
	}

	snapshot := topology.TranslateGatewayAPI(topology.GatewayAPIResources{
		Policies: []topology.Policy{
			{Object: obj, Type: topology.PolicyTypeDirect},
		},
	})

	ref := topology.ResourceRef{
		Group: groupSecurityExIO, Kind: kindAuthPolicy, Namespace: namespaceDefault, Name: policyName,
	}
	g.Expect(snapshot.NodeConditions(ref)).To(Equal([]topology.Condition{normalizedAccepted()}))
}

func acceptedCondition() metav1.Condition {
	return metav1.Condition{
		Type:    conditionTypeAccepted,
		Status:  metav1.ConditionTrue,
		Reason:  conditionTypeAccepted,
		Message: msgAccepted,
	}
}

func programmedCondition() metav1.Condition {
	return metav1.Condition{
		Type:    conditionTypeProgrammed,
		Status:  metav1.ConditionTrue,
		Reason:  conditionTypeProgrammed,
		Message: msgProgrammed,
	}
}

func resolvedRefsCondition() metav1.Condition {
	return metav1.Condition{
		Type:    conditionTypeResolved,
		Status:  metav1.ConditionTrue,
		Reason:  conditionTypeResolved,
		Message: msgResolved,
	}
}

func normalizedAccepted() topology.Condition {
	return topology.Condition{
		Type: conditionTypeAccepted, Status: conditionTrue, Reason: conditionTypeAccepted, Message: msgAccepted,
	}
}

func normalizedProgrammed() topology.Condition {
	return topology.Condition{
		Type: conditionTypeProgrammed, Status: conditionTrue, Reason: conditionTypeProgrammed, Message: msgProgrammed,
	}
}

func normalizedResolvedRefs() topology.Condition {
	return topology.Condition{
		Type: conditionTypeResolved, Status: conditionTrue, Reason: conditionTypeResolved, Message: msgResolved,
	}
}

func gatewayRef() topology.ResourceRef {
	return topology.ResourceRef{
		Group:     gatewayv1.GroupName,
		Kind:      kindGateway,
		Namespace: namespaceDefault,
		Name:      nameEdge,
	}
}

func httpRouteRef() topology.ResourceRef {
	return topology.ResourceRef{
		Group:     gatewayv1.GroupName,
		Kind:      kindHTTPRoute,
		Namespace: namespaceDefault,
		Name:      nameRoute,
	}
}

func genericPolicyCase() translateGatewayAPITestCase {
	const policyName = "rate-limit"

	return translateGatewayAPITestCase{
		name: "translates generic policy with targetRef",
		resources: topology.GatewayAPIResources{
			Policies: []topology.Policy{
				{
					Object: newTestPolicyUnstructured(
						groupExampleIO, "v1", kindRateLimitPolicy,
						policyName,
					),
					TargetRefs: []topology.PolicyTargetRef{
						{
							Group:     gatewayv1.GroupName,
							Kind:      kindGateway,
							Namespace: namespaceDefault,
							Name:      nameEdge,
						},
					},
					Type: topology.PolicyTypeDirect,
				},
			},
		},
		expectedNodes: []topology.ResourceRef{
			{
				Group:     groupExampleIO,
				Kind:      kindRateLimitPolicy,
				Namespace: namespaceDefault,
				Name:      policyName,
			},
			{
				Group:     gatewayv1.GroupName,
				Kind:      kindGateway,
				Namespace: namespaceDefault,
				Name:      nameEdge,
			},
		},
		expectedEdges: []topology.Edge{
			{
				From: topology.ResourceRef{
					Group:     groupExampleIO,
					Kind:      kindRateLimitPolicy,
					Namespace: namespaceDefault,
					Name:      policyName,
				},
				To: topology.ResourceRef{
					Group:     gatewayv1.GroupName,
					Kind:      kindGateway,
					Namespace: namespaceDefault,
					Name:      nameEdge,
				},
				Type:   topology.EdgeTypePolicyTargetRef,
				Detail: detailTargetRef,
			},
		},
	}
}

func genericPolicyMultipleTargetRefsCase() translateGatewayAPITestCase {
	const policyName = "auth-policy"

	policyRef := topology.ResourceRef{
		Group: groupSecurityExIO, Kind: kindAuthPolicy, Namespace: namespaceDefault, Name: policyName,
	}
	gwRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge,
	}
	routeRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute,
	}

	return translateGatewayAPITestCase{
		name: "translates generic policy with multiple targetRefs",
		resources: topology.GatewayAPIResources{
			Policies: []topology.Policy{
				{
					Object: newTestPolicyUnstructured(
						groupSecurityExIO, "v1beta1", kindAuthPolicy, policyName,
					),
					TargetRefs: []topology.PolicyTargetRef{
						{Group: gwRef.Group, Kind: gwRef.Kind, Namespace: gwRef.Namespace, Name: gwRef.Name},
						{Group: routeRef.Group, Kind: routeRef.Kind, Namespace: routeRef.Namespace, Name: routeRef.Name},
					},
					Type: topology.PolicyTypeInherited,
				},
			},
		},
		expectedNodes: []topology.ResourceRef{policyRef, gwRef, routeRef},
		expectedEdges: []topology.Edge{
			policyTargetRefEdge(policyRef, gwRef),
			policyTargetRefEdge(policyRef, routeRef),
		},
	}
}

func policyTargetRefEdge(from, to topology.ResourceRef) topology.Edge {
	return topology.Edge{
		From:   from,
		To:     to,
		Type:   topology.EdgeTypePolicyTargetRef,
		Detail: detailTargetRef,
	}
}

func newTestPolicyUnstructured(group, version, kind, name string) unstructured.Unstructured {
	obj := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": group + "/" + version,
			"kind":       kind,
			"metadata": map[string]any{
				"namespace": namespaceDefault,
				"name":      name,
			},
		},
	}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})

	return obj
}

func TestTranslateGatewayAPIPolicyAttributes(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	resources := topology.GatewayAPIResources{
		Policies: []topology.Policy{
			{
				Object: newTestPolicyUnstructured(
					groupExampleIO, "v1", kindRateLimitPolicy,
					"my-policy",
				),
				TargetRefs: []topology.PolicyTargetRef{
					{
						Group:     gatewayv1.GroupName,
						Kind:      kindGateway,
						Namespace: namespaceDefault,
						Name:      nameEdge,
					},
				},
				Type: topology.PolicyTypeDirect,
			},
		},
	}

	snapshot := topology.TranslateGatewayAPI(resources)

	policyRef := topology.ResourceRef{
		Group:     groupExampleIO,
		Kind:      kindRateLimitPolicy,
		Namespace: namespaceDefault,
		Name:      "my-policy",
	}

	var policyNode topology.Node

	for _, node := range snapshot.Nodes {
		if node.Ref == policyRef {
			policyNode = node

			break
		}
	}

	g.Expect(policyNode.Attributes).To(HaveKeyWithValue("policyType", "direct"))
}

func httpRouteExtensionRefCase() translateGatewayAPITestCase {
	const (
		filterGroup = "filters.example.io"
		filterKind  = "RateLimitFilter"
		filterName  = "my-rate-limit"
	)

	routeRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute,
	}
	filterRef := topology.ResourceRef{
		Group: filterGroup, Kind: filterKind, Namespace: namespaceDefault, Name: filterName,
	}

	return translateGatewayAPITestCase{
		name: "translates HTTPRoute with rule-level ExtensionRef filter",
		resources: topology.GatewayAPIResources{
			HTTPRoutes: []gatewayv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
					Spec: gatewayv1.HTTPRouteSpec{
						Rules: []gatewayv1.HTTPRouteRule{
							{
								Filters: []gatewayv1.HTTPRouteFilter{
									{
										Type: gatewayv1.HTTPRouteFilterExtensionRef,
										ExtensionRef: &gatewayv1.LocalObjectReference{
											Group: gatewayv1.Group(filterGroup),
											Kind:  gatewayv1.Kind(filterKind),
											Name:  gatewayv1.ObjectName(filterName),
										},
									},
								},
								BackendRefs: []gatewayv1.HTTPBackendRef{serviceBackendRef(nameBackend)},
							},
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{routeRef, filterRef,
			{Kind: kindService, Namespace: namespaceDefault, Name: nameBackend},
		},
		expectedEdges: []topology.Edge{
			defaultBackendRefEdge(),
			extensionRefEdge(routeRef, filterRef),
		},
	}
}

func httpRouteBackendExtensionRefCase() translateGatewayAPITestCase {
	const (
		filterGroup = "filters.example.io"
		filterKind  = "AuthFilter"
		filterName  = "my-auth"
	)

	routeRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute,
	}
	filterRef := topology.ResourceRef{
		Group: filterGroup, Kind: filterKind, Namespace: namespaceDefault, Name: filterName,
	}

	return translateGatewayAPITestCase{
		name: "translates HTTPRoute with backend-level ExtensionRef filter",
		resources: topology.GatewayAPIResources{
			HTTPRoutes: []gatewayv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameRoute},
					Spec: gatewayv1.HTTPRouteSpec{
						Rules: []gatewayv1.HTTPRouteRule{
							{
								BackendRefs: []gatewayv1.HTTPBackendRef{
									{
										BackendRef: gatewayv1.BackendRef{
											BackendObjectReference: gatewayv1.BackendObjectReference{
												Name: gatewayv1.ObjectName(nameBackend),
											},
										},
										Filters: []gatewayv1.HTTPRouteFilter{
											{
												Type: gatewayv1.HTTPRouteFilterExtensionRef,
												ExtensionRef: &gatewayv1.LocalObjectReference{
													Group: gatewayv1.Group(filterGroup),
													Kind:  gatewayv1.Kind(filterKind),
													Name:  gatewayv1.ObjectName(filterName),
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{routeRef, filterRef,
			{Kind: kindService, Namespace: namespaceDefault, Name: nameBackend},
		},
		expectedEdges: []topology.Edge{
			defaultBackendRefEdge(),
			extensionRefEdge(routeRef, filterRef),
		},
	}
}

func grpcRouteExtensionRefCase() translateGatewayAPITestCase {
	const (
		grpcRouteName = "grpc-route"
		filterGroup   = "filters.example.io"
		filterKind    = "RateLimitFilter"
		filterName    = "grpc-rate-limit"
		backendName   = "grpc-backend"
	)

	routeRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindGRPCRoute, Namespace: namespaceDefault, Name: grpcRouteName,
	}
	filterRef := topology.ResourceRef{
		Group: filterGroup, Kind: filterKind, Namespace: namespaceDefault, Name: filterName,
	}

	return translateGatewayAPITestCase{
		name: "translates GRPCRoute with rule-level ExtensionRef filter",
		resources: topology.GatewayAPIResources{
			GRPCRoutes: []gatewayv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: grpcRouteName},
					Spec: gatewayv1.GRPCRouteSpec{
						Rules: []gatewayv1.GRPCRouteRule{
							{
								Filters: []gatewayv1.GRPCRouteFilter{
									{
										Type: gatewayv1.GRPCRouteFilterExtensionRef,
										ExtensionRef: &gatewayv1.LocalObjectReference{
											Group: gatewayv1.Group(filterGroup),
											Kind:  gatewayv1.Kind(filterKind),
											Name:  gatewayv1.ObjectName(filterName),
										},
									},
								},
								BackendRefs: []gatewayv1.GRPCBackendRef{grpcBackendRef(backendName)},
							},
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{routeRef, filterRef,
			{Kind: kindService, Namespace: namespaceDefault, Name: backendName},
		},
		expectedEdges: []topology.Edge{
			routeBackendRefEdge(kindGRPCRoute, grpcRouteName, backendName),
			extensionRefEdge(routeRef, filterRef),
		},
	}
}

func extensionRefEdge(from, to topology.ResourceRef) topology.Edge {
	return topology.Edge{
		From:   from,
		To:     to,
		Type:   topology.EdgeTypeExtensionRef,
		Detail: "extensionRef",
	}
}

func gatewayClassParametersRefCase() translateGatewayAPITestCase {
	const (
		paramsGroup = "config.example.io"
		paramsKind  = "GatewayClassConfig"
		paramsName  = "my-config"
		paramsNS    = "config-ns"
	)

	gcRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindGatewayClass, Name: gatewayClassName,
	}
	configRef := topology.ResourceRef{
		Group: paramsGroup, Kind: paramsKind, Namespace: paramsNS, Name: paramsName,
	}

	ns := gatewayv1.Namespace(paramsNS)

	return translateGatewayAPITestCase{
		name: "translates GatewayClass with namespace-scoped parametersRef",
		resources: topology.GatewayAPIResources{
			GatewayClasses: []gatewayv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: gatewayClassName},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerNameExample,
						ParametersRef: &gatewayv1.ParametersReference{
							Group:     gatewayv1.Group(paramsGroup),
							Kind:      gatewayv1.Kind(paramsKind),
							Name:      paramsName,
							Namespace: &ns,
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{gcRef, configRef},
		expectedEdges: []topology.Edge{
			parametersRefEdge(gcRef, configRef),
		},
	}
}

func gatewayClassParametersRefClusterScopedCase() translateGatewayAPITestCase {
	const (
		paramsGroup = "config.example.io"
		paramsKind  = "ClusterGatewayConfig"
		paramsName  = "cluster-config"
	)

	gcRef := topology.ResourceRef{
		Group: gatewayv1.GroupName, Kind: kindGatewayClass, Name: gatewayClassName,
	}
	configRef := topology.ResourceRef{
		Group: paramsGroup, Kind: paramsKind, Name: paramsName,
	}

	return translateGatewayAPITestCase{
		name: "translates GatewayClass with cluster-scoped parametersRef",
		resources: topology.GatewayAPIResources{
			GatewayClasses: []gatewayv1.GatewayClass{
				{
					ObjectMeta: metav1.ObjectMeta{Name: gatewayClassName},
					Spec: gatewayv1.GatewayClassSpec{
						ControllerName: controllerNameExample,
						ParametersRef: &gatewayv1.ParametersReference{
							Group: gatewayv1.Group(paramsGroup),
							Kind:  gatewayv1.Kind(paramsKind),
							Name:  paramsName,
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{gcRef, configRef},
		expectedEdges: []topology.Edge{
			parametersRefEdge(gcRef, configRef),
		},
	}
}

func gatewayParametersRefCase() translateGatewayAPITestCase {
	const (
		paramsGroup = "infra.example.io"
		paramsKind  = "GatewayConfig"
		paramsName  = "my-gw-config"
	)

	gatewayRef := topology.ResourceRef{
		Group:     gatewayv1.GroupName,
		Kind:      kindGateway,
		Namespace: namespaceDefault,
		Name:      nameEdge,
	}
	configRef := topology.ResourceRef{
		Group:     paramsGroup,
		Kind:      paramsKind,
		Namespace: namespaceDefault,
		Name:      paramsName,
	}
	gcRef := topology.ResourceRef{
		Group: gatewayv1.GroupName,
		Kind:  kindGatewayClass,
		Name:  gatewayClassName,
	}

	return translateGatewayAPITestCase{
		name: "translates Gateway with infrastructure parametersRef",
		resources: topology.GatewayAPIResources{
			Gateways: []gatewayv1.Gateway{
				{
					ObjectMeta: metav1.ObjectMeta{Namespace: namespaceDefault, Name: nameEdge},
					Spec: gatewayv1.GatewaySpec{
						GatewayClassName: gatewayv1.ObjectName(gatewayClassName),
						Infrastructure: &gatewayv1.GatewayInfrastructure{
							ParametersRef: &gatewayv1.LocalParametersReference{
								Group: gatewayv1.Group(paramsGroup),
								Kind:  gatewayv1.Kind(paramsKind),
								Name:  paramsName,
							},
						},
					},
				},
			},
		},
		expectedNodes: []topology.ResourceRef{gatewayRef, gcRef, configRef},
		expectedEdges: []topology.Edge{
			{
				From:   gatewayRef,
				To:     gcRef,
				Type:   topology.EdgeTypeAttachment,
				Detail: "gatewayClass",
			},
			parametersRefEdge(gatewayRef, configRef),
		},
	}
}

func parametersRefEdge(from, to topology.ResourceRef) topology.Edge {
	return topology.Edge{
		From:   from,
		To:     to,
		Type:   topology.EdgeTypeParametersRef,
		Detail: "parametersRef",
	}
}
