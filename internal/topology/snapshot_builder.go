package topology

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

var (
	errInvalidNodeReference = errors.New("invalid node reference")
	errInvalidEdgeReference = errors.New("invalid edge reference")
)

// SnapshotBuilder assembles deterministic topology snapshots.
type SnapshotBuilder struct {
	// policyAdapters enrich the snapshot with implementor-specific metadata.
	policyAdapters []PolicyAdapter
}

// NewSnapshotBuilder creates a snapshot builder with optional policy adapters.
func NewSnapshotBuilder(policyAdapters ...PolicyAdapter) SnapshotBuilder {
	copiedAdapters := make([]PolicyAdapter, len(policyAdapters))
	copy(copiedAdapters, policyAdapters)

	return SnapshotBuilder{policyAdapters: copiedAdapters}
}

// Build validates, enriches, and sorts the input snapshot.
func (b SnapshotBuilder) Build(base Snapshot, resources GatewayAPIResources) (Snapshot, error) {
	if err := validateSnapshot(base); err != nil {
		return Snapshot{}, err
	}

	assembled := base.Clone()

	for _, policyAdapter := range b.policyAdapters {
		annotations, err := policyAdapter.Annotate(resources, assembled)
		if err != nil {
			return Snapshot{}, fmt.Errorf("annotating snapshot with adapter %q: %w", policyAdapter.Name(), err)
		}

		assembled.Annotations = append(assembled.Annotations, annotations...)
	}

	sortSnapshot(&assembled)

	return assembled, nil
}

// validateSnapshot checks that all nodes and edges have valid resource references.
func validateSnapshot(snapshot Snapshot) error {
	for _, node := range snapshot.Nodes {
		if !node.IsValid() {
			return fmt.Errorf("%w: kind=%q name=%q", errInvalidNodeReference, node.Ref.Kind, node.Ref.Name)
		}
	}

	for _, edge := range snapshot.Edges {
		if !edge.IsValid() {
			return fmt.Errorf(
				"%w: from=%q/%q to=%q/%q",
				errInvalidEdgeReference,
				edge.From.Kind,
				edge.From.Name,
				edge.To.Kind,
				edge.To.Name,
			)
		}
	}

	return nil
}

// sortSnapshot deterministically sorts nodes, edges, and annotations.
func sortSnapshot(snapshot *Snapshot) {
	slices.SortFunc(snapshot.Nodes, func(a, b Node) int {
		return compareResourceRef(a.Ref, b.Ref)
	})

	slices.SortFunc(snapshot.Edges, func(a, b Edge) int {
		if c := compareResourceRef(a.From, b.From); c != 0 {
			return c
		}

		if c := compareResourceRef(a.To, b.To); c != 0 {
			return c
		}

		if c := cmp.Compare(a.Type, b.Type); c != 0 {
			return c
		}

		return cmp.Compare(a.Detail, b.Detail)
	})

	slices.SortFunc(snapshot.Annotations, func(a, b NodeAnnotation) int {
		if c := compareResourceRef(a.Ref, b.Ref); c != 0 {
			return c
		}

		if c := cmp.Compare(a.Source, b.Source); c != 0 {
			return c
		}

		if c := cmp.Compare(a.Key, b.Key); c != 0 {
			return c
		}

		return cmp.Compare(a.Value, b.Value)
	})
}

// compareResourceRef provides a deterministic ordering for resource references.
func compareResourceRef(a, b ResourceRef) int {
	if c := cmp.Compare(a.Group, b.Group); c != 0 {
		return c
	}

	if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}

	if c := cmp.Compare(a.Namespace, b.Namespace); c != 0 {
		return c
	}

	return cmp.Compare(a.Name, b.Name)
}
