package topology_test

import (
	"testing"

	"github.com/nginxinc/gateway-lens/internal/topology"
	. "github.com/onsi/gomega"
)

const (
	kindGateway   = "Gateway"
	kindHTTPRoute = "HTTPRoute"
	kindService   = "Service"
	nameEdge      = "edge"
	nameRoute     = "route"
	nameBackend   = "backend"
	classKey      = "class"
	classExample  = "example"
)

type resourceRefTestCase struct {
	name   string
	ref    topology.ResourceRef
	assert func(*WithT, topology.ResourceRef)
}

func TestResourceRef(t *testing.T) {
	t.Parallel()

	testCases := []resourceRefTestCase{
		{
			name: "is valid",
			ref:  topology.ResourceRef{Group: "", Kind: "Gateway", Namespace: "", Name: "edge"},
			assert: func(g *WithT, ref topology.ResourceRef) {
				g.Expect(ref.IsValid()).To(BeTrue())
			},
		},
		{
			name: "is invalid without kind",
			ref:  topology.ResourceRef{Group: "", Kind: "", Namespace: "", Name: "edge"},
			assert: func(g *WithT, ref topology.ResourceRef) {
				g.Expect(ref.IsValid()).To(BeFalse())
			},
		},
		{
			name: "is namespaced",
			ref:  topology.ResourceRef{Group: "", Kind: "HTTPRoute", Namespace: "default", Name: "route"},
			assert: func(g *WithT, ref topology.ResourceRef) {
				g.Expect(ref.IsNamespaced()).To(BeTrue())
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			testCase.assert(g, testCase.ref)
		})
	}
}

func TestSnapshotClone(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	original := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref:        topology.ResourceRef{Kind: kindGateway, Name: nameEdge},
				Attributes: map[string]string{classKey: classExample},
				Conditions: []topology.Condition{{Type: conditionTypeAccepted, Status: conditionTrue}},
			},
		},
		Edges: []topology.Edge{
			{
				From: topology.ResourceRef{Kind: kindHTTPRoute, Name: nameRoute},
				To:   topology.ResourceRef{Kind: kindService, Name: nameBackend},
				Type: topology.EdgeTypeBackendRef,
			},
		},
		Annotations: []topology.NodeAnnotation{
			{
				Ref:    topology.ResourceRef{Kind: kindGateway, Name: nameEdge},
				Source: "test",
				Key:    "policy",
				Value:  "enabled",
			},
		},
	}

	copied := original.Clone()
	copied.Nodes[0].Attributes[classKey] = "changed"
	copied.Nodes[0].Conditions[0].Status = "False"
	copied.Edges[0].Detail = "changed"
	copied.Annotations[0].Value = "disabled"

	g.Expect(original.Nodes[0].Attributes[classKey]).To(Equal(classExample))
	g.Expect(original.Nodes[0].Conditions[0].Status).To(Equal("True"))
	g.Expect(original.Edges[0].Detail).To(BeEmpty())
	g.Expect(original.Annotations[0].Value).To(Equal("enabled"))
}

func TestSnapshotHasNode(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{Nodes: []topology.Node{{Ref: topology.ResourceRef{Kind: kindGateway, Name: nameEdge}}}}

	g.Expect(snapshot.HasNode(topology.ResourceRef{Kind: kindGateway, Name: nameEdge})).To(BeTrue())
	g.Expect(snapshot.HasNode(topology.ResourceRef{Kind: kindGateway, Name: "other"})).To(BeFalse())
}

func TestValidityChecks(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	node := topology.Node{Ref: topology.ResourceRef{Kind: kindGateway, Name: nameEdge}}
	g.Expect(node.IsValid()).To(BeTrue())

	edge := topology.Edge{
		From: topology.ResourceRef{Kind: kindHTTPRoute, Name: nameRoute},
		To:   topology.ResourceRef{Kind: kindService, Name: nameBackend},
		Type: topology.EdgeTypeBackendRef,
	}

	g.Expect(edge.IsValid()).To(BeTrue())

	invalidEdge := topology.Edge{
		From: topology.ResourceRef{Kind: kindHTTPRoute, Name: nameRoute},
		To:   topology.ResourceRef{Kind: kindService, Name: ""},
	}

	g.Expect(invalidEdge.IsValid()).To(BeFalse())
}
