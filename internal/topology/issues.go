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

package topology

import (
	"slices"
	"strings"
)

// Issue represents a single problem detected in the topology,
// associated with the resource where it was found.
type Issue struct {
	// Resource identifies the node that has the issue.
	Resource ResourceRef
	// Severity classifies how serious the issue is.
	Severity DiagnosticSeverity
	// Reason is a machine-readable reason for the issue.
	Reason string
	// Message is a human-readable description of the issue.
	Message string
}

// errorConditionTypes returns the condition types where a status of "False"
// indicates a meaningful error.
func errorConditionTypes() map[string]struct{} {
	return map[string]struct{}{
		"Accepted":     {},
		"Programmed":   {},
		"ResolvedRefs": {},
	}
}

// Issues returns a deterministically sorted list of problems detected in the snapshot.
//
// An issue is produced for each Diagnostic on every node, and for each
// Condition matching the negative-condition rule (bare type in the
// errorConditionTypes set with status "False").
func (s Snapshot) Issues() []Issue {
	var issues []Issue

	for _, node := range s.Nodes {
		for _, diagnostic := range node.Diagnostics {
			issues = append(issues, Issue{
				Resource: node.Ref,
				Severity: diagnostic.Severity,
				Reason:   diagnostic.Reason,
				Message:  diagnostic.Message,
			})
		}

		for _, condition := range node.Conditions {
			if isNegativeCondition(condition) {
				issues = append(issues, Issue{
					Resource: node.Ref,
					Severity: DiagnosticSeverityError,
					Reason:   condition.Reason,
					Message:  condition.Message,
				})
			}
		}
	}

	slices.SortFunc(issues, compareIssues)

	return issues
}

// compareIssues provides deterministic ordering: Error before Info,
// then by resource reference.
func compareIssues(a, b Issue) int {
	if a.Severity != b.Severity {
		// Error sorts before Info (E < I lexically, which happens to be correct).
		if a.Severity == DiagnosticSeverityError {
			return -1
		}

		return 1
	}

	return compareResourceRef(a.Resource, b.Resource)
}

// isNegativeCondition reports whether a condition indicates an error:
// its bare type is in the errorConditionTypes set and its status is "False".
func isNegativeCondition(condition Condition) bool {
	bare := bareConditionType(condition.Type)

	if _, ok := errorConditionTypes()[bare]; !ok {
		return false
	}

	return strings.EqualFold(condition.Status, "False")
}

// bareConditionType strips any prefix from a condition type.
// Condition types may be prefixed (e.g. "listener/http/Accepted"),
// so we extract the last segment after the final "/".
func bareConditionType(conditionType string) string {
	if idx := strings.LastIndex(conditionType, "/"); idx >= 0 {
		return conditionType[idx+1:]
	}

	return conditionType
}
