package resources

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/topology"
)

const (
	// policyLabelKey is the Gateway API policy CRD label key (from GEP-713).
	policyLabelKey = "gateway.networking.k8s.io/policy"
	// policyLabelValueDirect indicates a direct policy attachment.
	policyLabelValueDirect = "direct"
	// policyLabelValueInherited indicates an inherited policy attachment.
	policyLabelValueInherited = "inherited"
	// policyLabelValueTrue is a legacy/fallback value.
	policyLabelValueTrue = "true"
	// kindBackendTLSPolicy is the kind string for BackendTLSPolicy.
	kindBackendTLSPolicy = "BackendTLSPolicy"
)

// PolicyCRD represents a discovered Gateway API policy CRD.
type PolicyCRD struct {
	// GVK is the GroupVersionKind for policy instances.
	GVK schema.GroupVersionKind
	// Type indicates whether the policy is direct or inherited.
	Type topology.PolicyType
}

// PolicyStore discovers policy CRDs and maintains the current set of policy instances.
type PolicyStore struct {
	// cache provides read access to cluster resources.
	cache cache.Cache
	// logger is the structured logger for policy operations.
	logger logr.Logger

	// mu guards the policy state.
	mu sync.RWMutex
	// policyCRDs maps GVK string to policy CRD metadata.
	policyCRDs map[string]PolicyCRD
	// policies maps GVK+NamespacedName to the parsed policy.
	policies map[objectStoreKey]topology.Policy
}

// NewPolicyStore creates a policy store backed by the given cache.
func NewPolicyStore(runtimeCache cache.Cache, logger logr.Logger) *PolicyStore {
	return &PolicyStore{
		cache:      runtimeCache,
		logger:     logger,
		policyCRDs: make(map[string]PolicyCRD),
		policies:   make(map[objectStoreKey]topology.Policy),
	}
}

// DiscoverPolicyCRDs scans all CRDs for policy label and returns matching policy CRDs.
func (s *PolicyStore) DiscoverPolicyCRDs(ctx context.Context) ([]PolicyCRD, error) {
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := s.cache.List(ctx, crdList); err != nil {
		return nil, fmt.Errorf("listing CRDs: %w", err)
	}

	var discovered []PolicyCRD

	for i := range crdList.Items {
		crd := &crdList.Items[i]

		labelValue, hasPolicyLabel := crd.Labels[policyLabelKey]
		if !hasPolicyLabel {
			continue
		}

		policyType, valid := parsePolicyLabelValue(labelValue)
		if !valid {
			s.logger.Info("Skipping CRD with invalid policy label",
				"crd", crd.Name,
				"label", labelValue,
			)

			continue
		}

		// Skip BackendTLSPolicy since it's already watched with typed informers.
		if crd.Spec.Group == gatewayv1.GroupName && crd.Spec.Names.Kind == kindBackendTLSPolicy {
			s.logger.V(1).Info("Skipping BackendTLSPolicy (handled by typed informer)", "crd", crd.Name)

			continue
		}

		gvk := preferredGVK(crd)

		discovered = append(discovered, PolicyCRD{
			GVK:  gvk,
			Type: policyType,
		})

		s.logger.V(1).Info("Discovered policy CRD",
			"gvk", gvk.String(),
			"type", policyType,
		)
	}

	s.mu.Lock()

	s.policyCRDs = make(map[string]PolicyCRD, len(discovered))
	for _, p := range discovered {
		s.policyCRDs[p.GVK.String()] = p
	}
	s.mu.Unlock()

	return discovered, nil
}

// SyncPolicies fetches all instances of each discovered policy CRD and updates the store.
//
// Note: there is a small eventual-consistency window between the snapshot read (under RLock)
// and the store swap (under Lock). Concurrent UpsertPolicy calls during that window may be
// overwritten, but the next event batch will reconcile the state.
func (s *PolicyStore) SyncPolicies(ctx context.Context) error {
	s.mu.RLock()

	crds := make([]PolicyCRD, 0, len(s.policyCRDs))
	for _, crd := range s.policyCRDs {
		crds = append(crds, crd)
	}

	s.mu.RUnlock()

	newPolicies := make(map[objectStoreKey]topology.Policy)

	for _, crd := range crds {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   crd.GVK.Group,
			Version: crd.GVK.Version,
			Kind:    crd.GVK.Kind + "List",
		})

		if err := s.cache.List(ctx, list); err != nil {
			s.logger.Error(err, "Failed to list policies", "gvk", crd.GVK.String())

			continue
		}

		for i := range list.Items {
			obj := list.Items[i]
			key := objectStoreKey{
				gvk: crd.GVK.String(),
				nn:  types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
			}

			targetRefs, err := parseTargetRefs(&obj)
			if err != nil {
				s.logger.Error(err, "Failed to parse policy targetRefs",
					"policy", key.nn.String(),
					"gvk", crd.GVK.String(),
				)

				continue
			}

			newPolicies[key] = topology.Policy{
				Object:     *obj.DeepCopy(),
				TargetRefs: targetRefs,
				Type:       crd.Type,
			}
		}
	}

	s.mu.Lock()
	s.policies = newPolicies
	s.mu.Unlock()

	return nil
}

// UpsertPolicy adds or updates a single policy in the store.
func (s *PolicyStore) UpsertPolicy(obj *unstructured.Unstructured) {
	gvk := obj.GetObjectKind().GroupVersionKind()

	s.mu.RLock()
	crd, found := s.policyCRDs[gvk.String()]
	s.mu.RUnlock()

	if !found {
		return
	}

	targetRefs, err := parseTargetRefs(obj)
	if err != nil {
		s.logger.Error(err, "Failed to parse policy targetRefs on upsert",
			"policy", obj.GetName(),
			"namespace", obj.GetNamespace(),
			"gvk", gvk.String(),
		)

		return
	}

	key := objectStoreKey{
		gvk: gvk.String(),
		nn:  types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
	}

	s.mu.Lock()
	s.policies[key] = topology.Policy{
		Object:     *obj.DeepCopy(),
		TargetRefs: targetRefs,
		Type:       crd.Type,
	}
	s.mu.Unlock()
}

// DeletePolicy removes a policy from the store.
func (s *PolicyStore) DeletePolicy(gvk schema.GroupVersionKind, nn types.NamespacedName) {
	key := objectStoreKey{gvk: gvk.String(), nn: nn}

	s.mu.Lock()
	delete(s.policies, key)
	s.mu.Unlock()
}

// Get returns a sorted slice of all current policies.
func (s *PolicyStore) Get() []topology.Policy {
	s.mu.RLock()
	defer s.mu.RUnlock()

	policies := make([]topology.Policy, 0, len(s.policies))
	for _, p := range s.policies {
		policies = append(policies, p)
	}

	// Sort for deterministic output.
	slices.SortFunc(policies, func(a, b topology.Policy) int {
		return compareUnstructuredByGVKAndName(&a.Object, &b.Object)
	})

	return policies
}

// PolicyCRDs returns a copy of the discovered policy CRDs.
func (s *PolicyStore) PolicyCRDs() []PolicyCRD {
	s.mu.RLock()
	defer s.mu.RUnlock()

	crds := make([]PolicyCRD, 0, len(s.policyCRDs))
	for _, crd := range s.policyCRDs {
		crds = append(crds, crd)
	}

	return crds
}

// parsePolicyLabelValue parses the policy CRD label value and returns the policy type.
func parsePolicyLabelValue(value string) (topology.PolicyType, bool) {
	normalized := strings.ToLower(value)

	switch normalized {
	case policyLabelValueDirect:
		return topology.PolicyTypeDirect, true
	case policyLabelValueInherited:
		return topology.PolicyTypeInherited, true
	case policyLabelValueTrue:
		// "true" defaults to direct policy.
		return topology.PolicyTypeDirect, true
	default:
		return "", false
	}
}

// parseTargetRefs extracts target references from an unstructured policy object
// by reading directly from the unstructured map, avoiding a full JSON round-trip.
func parseTargetRefs(obj *unstructured.Unstructured) ([]topology.PolicyTargetRef, error) {
	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		return nil, nil
	}

	policyNamespace := obj.GetNamespace()

	var refs []topology.PolicyTargetRef

	// Collect from plural targetRefs.
	if rawRefs, ok := spec["targetRefs"].([]any); ok {
		for _, rawRef := range rawRefs {
			refMap, ok := rawRef.(map[string]any)
			if !ok {
				continue
			}

			refs = append(refs, resolveUnstructuredTargetRef(policyNamespace, refMap))
		}
	}

	// Collect from singular targetRef if present and not already included via targetRefs.
	if rawRef, ok := spec["targetRef"].(map[string]any); ok {
		name, nameOK := rawRef["name"].(string)
		if nameOK && name != "" {
			ref := resolveUnstructuredTargetRef(policyNamespace, rawRef)
			if !containsTargetRef(refs, ref) {
				refs = append(refs, ref)
			}
		}
	}

	return refs, nil
}

// resolveUnstructuredTargetRef resolves a target reference map to a PolicyTargetRef.
func resolveUnstructuredTargetRef(
	policyNamespace string,
	refMap map[string]any,
) topology.PolicyTargetRef {
	group, _ := refMap["group"].(string) //nolint:revive // zero-value on missing key is intentional
	kind, _ := refMap["kind"].(string)  //nolint:revive // zero-value on missing key is intentional
	name, _ := refMap["name"].(string)  //nolint:revive // zero-value on missing key is intentional

	namespace := policyNamespace
	if ns, ok := refMap["namespace"].(string); ok && ns != "" {
		namespace = ns
	}

	return topology.PolicyTargetRef{
		Group:     group,
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
	}
}

// containsTargetRef checks if a target ref is already in the slice.
func containsTargetRef(refs []topology.PolicyTargetRef, target topology.PolicyTargetRef) bool {
	return slices.Contains(refs, target)
}
