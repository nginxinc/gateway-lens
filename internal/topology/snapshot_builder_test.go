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

package topology_test

import (
	"errors"
	"testing"

	"github.com/nginxinc/gateway-lens/internal/topology"
	. "github.com/onsi/gomega"
)

var errAnnotateFailed = errors.New("annotate failed")

const (
	namespaceDefault = "default"
	adapterBroken    = "broken"
)

type fakePolicyAdapter struct {
	name        string
	annotations []topology.NodeAnnotation
	annotateErr error
}

func (f fakePolicyAdapter) Name() string {
	return f.name
}

func (f fakePolicyAdapter) Annotate(
	_ topology.GatewayAPIResources,
	_ topology.Snapshot,
) ([]topology.NodeAnnotation, error) {
	if f.annotateErr != nil {
		return nil, f.annotateErr
	}

	return f.annotations, nil
}

type snapshotBuilderTestCase struct {
	name               string
	base               topology.Snapshot
	adapters           []topology.PolicyAdapter
	expectedAnnotation []topology.NodeAnnotation
	expectedError      string
}

func TestSnapshotBuilderBuild(t *testing.T) {
	t.Parallel()

	for _, testCase := range snapshotBuilderTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runSnapshotBuilderTest(t, testCase)
		})
	}
}

func snapshotBuilderTestCases() []snapshotBuilderTestCase {
	return []snapshotBuilderTestCase{
		{
			name: "returns deterministic sorted snapshot with annotations",
			base: topology.Snapshot{
				Nodes: []topology.Node{
					{Ref: topology.ResourceRef{Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute}},
					{Ref: topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge}},
				},
				Edges: []topology.Edge{
					{
						From: topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
						To:   topology.ResourceRef{Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute},
						Type: topology.EdgeTypeParentRef,
					},
				},
			},
			adapters: []topology.PolicyAdapter{
				fakePolicyAdapter{
					name: classExample,
					annotations: []topology.NodeAnnotation{
						{
							Ref:    topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
							Source: classExample,
							Key:    "mode",
							Value:  "strict",
						},
					},
				},
			},
			expectedAnnotation: []topology.NodeAnnotation{
				{
					Ref:    topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
					Source: classExample,
					Key:    "mode",
					Value:  "strict",
				},
			},
		},
		{
			name:          "returns error for invalid node",
			base:          topology.Snapshot{Nodes: []topology.Node{{Ref: topology.ResourceRef{Kind: kindGateway, Name: ""}}}},
			expectedError: "invalid node reference",
		},
		{
			name: "wraps adapter errors",
			base: topology.Snapshot{Nodes: []topology.Node{{Ref: topology.ResourceRef{Kind: kindGateway, Name: nameEdge}}}},
			adapters: []topology.PolicyAdapter{
				fakePolicyAdapter{name: adapterBroken, annotateErr: errAnnotateFailed},
			},
			expectedError: "annotating snapshot with adapter \"broken\": annotate failed",
		},
	}
}

func runSnapshotBuilderTest(t *testing.T, testCase snapshotBuilderTestCase) {
	t.Helper()

	g := NewWithT(t)
	builder := topology.NewSnapshotBuilder(testCase.adapters...)

	assembled, err := builder.Build(testCase.base, topology.GatewayAPIResources{})

	if testCase.expectedError != "" {
		g.Expect(err).To(MatchError(ContainSubstring(testCase.expectedError)))

		return
	}

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(assembled.Nodes[0].Ref.Kind).To(Equal(kindGateway))
	g.Expect(assembled.Annotations).To(Equal(testCase.expectedAnnotation))
}
