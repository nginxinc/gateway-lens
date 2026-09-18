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
	"testing"

	. "github.com/onsi/gomega"

	"github.com/nginxinc/gateway-lens/internal/topology"
)

const msgServiceNotFound = "The Service does not exist."

func TestIssuesReturnsEmptyForHealthySnapshot(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
				Conditions: []topology.Condition{
					{Type: conditionTypeAccepted, Status: conditionTrue, Reason: conditionTypeAccepted, Message: "OK"},
				},
			},
		},
	}

	g.Expect(snapshot.Issues()).To(BeEmpty())
}

func TestIssuesFromDiagnostics(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindService, Namespace: namespaceDefault, Name: nameBackend},
				Diagnostics: []topology.Diagnostic{
					{
						Severity: topology.DiagnosticSeverityError,
						Reason:   reasonServiceNotFound,
						Message:  msgServiceNotFound,
					},
				},
			},
		},
	}

	issues := snapshot.Issues()

	g.Expect(issues).To(HaveLen(1))
	g.Expect(issues[0]).To(Equal(topology.Issue{
		Resource: topology.ResourceRef{Kind: kindService, Namespace: namespaceDefault, Name: nameBackend},
		Severity: topology.DiagnosticSeverityError,
		Reason:   reasonServiceNotFound,
		Message:  msgServiceNotFound,
	}))
}

func TestIssuesFromNegativeCondition(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute},
				Conditions: []topology.Condition{
					{Type: conditionTypeAccepted, Status: conditionFalse, Reason: "NoMatchingParent", Message: "No matching parent"},
				},
			},
		},
	}

	issues := snapshot.Issues()

	g.Expect(issues).To(HaveLen(1))
	g.Expect(issues[0]).To(Equal(topology.Issue{
		Resource: topology.ResourceRef{Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute},
		Severity: topology.DiagnosticSeverityError,
		Reason:   "NoMatchingParent",
		Message:  "No matching parent",
	}))
}

func TestIssuesFromPrefixedConditionType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
				Conditions: []topology.Condition{
					{Type: "listener/http/Accepted", Status: conditionFalse, Reason: "InvalidConfig", Message: "Bad config"},
				},
			},
		},
	}

	issues := snapshot.Issues()

	g.Expect(issues).To(HaveLen(1))
	g.Expect(issues[0].Reason).To(Equal("InvalidConfig"))
}

func TestIssuesIgnoresTrueCondition(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
				Conditions: []topology.Condition{
					{Type: conditionTypeAccepted, Status: conditionTrue, Reason: conditionTypeAccepted, Message: "OK"},
					{Type: conditionTypeProgrammed, Status: conditionTrue, Reason: conditionTypeProgrammed, Message: "OK"},
				},
			},
		},
	}

	g.Expect(snapshot.Issues()).To(BeEmpty())
}

func TestIssuesIgnoresNonErrorConditionType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindGateway, Namespace: namespaceDefault, Name: nameEdge},
				Conditions: []topology.Condition{
					{Type: "CustomType", Status: conditionFalse, Reason: "Something", Message: "Not relevant"},
				},
			},
		},
	}

	g.Expect(snapshot.Issues()).To(BeEmpty())
}

func TestIssuesSortsSeverityThenRef(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindService, Namespace: namespaceDefault, Name: "z-service"},
				Diagnostics: []topology.Diagnostic{
					{Severity: topology.DiagnosticSeverityInfo, Reason: "Info", Message: "info msg"},
				},
			},
			{
				Ref: topology.ResourceRef{Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: "a-route"},
				Diagnostics: []topology.Diagnostic{
					{Severity: topology.DiagnosticSeverityError, Reason: "Error", Message: "error msg"},
				},
			},
		},
	}

	issues := snapshot.Issues()

	g.Expect(issues).To(HaveLen(2))
	g.Expect(issues[0].Severity).To(Equal(topology.DiagnosticSeverityError))
	g.Expect(issues[0].Resource.Name).To(Equal("a-route"))
	g.Expect(issues[1].Severity).To(Equal(topology.DiagnosticSeverityInfo))
	g.Expect(issues[1].Resource.Name).To(Equal("z-service"))
}

func TestIssuesCombinesDiagnosticsAndConditions(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)
	snapshot := topology.Snapshot{
		Nodes: []topology.Node{
			{
				Ref: topology.ResourceRef{Kind: kindHTTPRoute, Namespace: namespaceDefault, Name: nameRoute},
				Conditions: []topology.Condition{
					{Type: conditionTypeResolved, Status: conditionFalse, Reason: "RefNotFound", Message: "ref not found"},
				},
				Diagnostics: []topology.Diagnostic{
					{Severity: topology.DiagnosticSeverityError, Reason: "GatewayNotFound", Message: "gw missing"},
				},
			},
		},
	}

	issues := snapshot.Issues()

	g.Expect(issues).To(HaveLen(2))
	// Both are Error severity with same ref — order within same key is diagnostics then conditions
	// (from iteration order), but both should be present.
	reasons := []string{issues[0].Reason, issues[1].Reason}
	g.Expect(reasons).To(ConsistOf("GatewayNotFound", "RefNotFound"))
}
