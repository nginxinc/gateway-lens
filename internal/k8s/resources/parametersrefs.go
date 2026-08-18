package resources

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ParametersRefStore discovers and tracks ParametersRef CRD instances referenced by
// GatewayClasses and Gateways.
type ParametersRefStore struct {
	// cache provides read access to cluster resources.
	cache cache.Cache
	// logger is the structured logger for ParametersRef operations.
	logger logr.Logger

	// mu guards the ParametersRef state.
	mu sync.RWMutex
	// gvks maps GVK string to its parsed GroupVersionKind.
	gvks map[string]schema.GroupVersionKind
	// objects maps key to unstructured object.
	objects map[objectStoreKey]unstructured.Unstructured
	// excludedGroupKinds contains group/kind pairs already watched by other controllers.
	// These are skipped during discovery to avoid registering duplicate controllers.
	excludedGroupKinds map[string]struct{}
}

// NewParametersRefStore creates a ParametersRef store backed by the given cache.
func NewParametersRefStore(runtimeCache cache.Cache, logger logr.Logger) *ParametersRefStore {
	return &ParametersRefStore{
		cache:              runtimeCache,
		logger:             logger,
		gvks:               make(map[string]schema.GroupVersionKind),
		objects:            make(map[objectStoreKey]unstructured.Unstructured),
		excludedGroupKinds: make(map[string]struct{}),
	}
}

// ExcludeGroupKind marks a group/kind pair as already watched, preventing duplicate controller registration.
func (s *ParametersRefStore) ExcludeGroupKind(group, kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.excludedGroupKinds[group+"/"+kind] = struct{}{}
}

// DiscoverGVKs extracts unique ParametersRef GVKs from the given GatewayClasses and Gateways.
// GatewayClass.Spec.ParametersRef and Gateway.Spec.Infrastructure.ParametersRef only include
// group+kind, so we look up the preferred served version from the cluster's CRD definitions.
// Returns the list of newly discovered GVKs (not previously known).
func (s *ParametersRefStore) DiscoverGVKs(
	ctx context.Context,
	gatewayClasses []gatewayv1.GatewayClass,
	gateways []gatewayv1.Gateway,
) ([]schema.GroupVersionKind, error) {
	// Collect group+kind pairs from GatewayClass and Gateway specs.
	groupKinds := make(map[string]schema.GroupVersionKind)

	for i := range gatewayClasses {
		collectGatewayClassParametersRefGVKs(&gatewayClasses[i], groupKinds)
	}

	for i := range gateways {
		collectGatewayParametersRefGVKs(&gateways[i], groupKinds)
	}

	if len(groupKinds) == 0 {
		return nil, nil
	}

	resolved, err := s.resolveGVKs(ctx, groupKinds)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var newGVKs []schema.GroupVersionKind

	for key, gvk := range resolved {
		if _, exists := s.gvks[key]; !exists {
			s.gvks[key] = gvk
			newGVKs = append(newGVKs, gvk)

			s.logger.V(1).Info("Discovered ParametersRef GVK", "gvk", gvk.String())
		}
	}

	return newGVKs, nil
}

// SyncObjects fetches all instances of the given ParametersRef GVKs and adds them to the store.
// If gvks is nil, all known GVKs are synced. Existing objects from other GVKs are preserved.
func (s *ParametersRefStore) SyncObjects(ctx context.Context, gvks []schema.GroupVersionKind) error {
	if gvks == nil {
		s.mu.RLock()

		gvks = make([]schema.GroupVersionKind, 0, len(s.gvks))
		for _, gvk := range s.gvks {
			gvks = append(gvks, gvk)
		}

		s.mu.RUnlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	syncObjectsFromCache(ctx, s.cache, gvks, s.objects, "ParametersRef", s.logger)

	return nil
}

// Upsert adds or updates a single ParametersRef object in the store.
func (s *ParametersRefStore) Upsert(obj *unstructured.Unstructured) {
	gvk := obj.GetObjectKind().GroupVersionKind()

	key := objectStoreKey{
		gvk: gvk.String(),
		nn:  types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
	}

	s.mu.Lock()
	s.objects[key] = *obj.DeepCopy()
	s.mu.Unlock()
}

// Delete removes a ParametersRef object from the store.
func (s *ParametersRefStore) Delete(gvk schema.GroupVersionKind, nn types.NamespacedName) {
	key := objectStoreKey{gvk: gvk.String(), nn: nn}

	s.mu.Lock()
	delete(s.objects, key)
	s.mu.Unlock()
}

// Get returns a sorted slice of all current ParametersRef objects.
func (s *ParametersRefStore) Get() []unstructured.Unstructured {
	s.mu.RLock()
	defer s.mu.RUnlock()

	objects := make([]unstructured.Unstructured, 0, len(s.objects))
	for _, obj := range s.objects {
		objects = append(objects, obj)
	}

	slices.SortFunc(objects, func(a, b unstructured.Unstructured) int {
		return compareUnstructuredByGVKAndName(&a, &b)
	})

	return objects
}

// resolveGVKs looks up CRD versions for the given group+kind pairs and filters out
// any group/kinds already watched by other controllers.
func (s *ParametersRefStore) resolveGVKs(
	ctx context.Context,
	groupKinds map[string]schema.GroupVersionKind,
) (map[string]schema.GroupVersionKind, error) {
	s.mu.RLock()
	excluded := maps.Clone(s.excludedGroupKinds)
	s.mu.RUnlock()

	return resolveGVKsFromCRDs(ctx, s.cache, groupKinds, excluded, "ParametersRef")
}

// collectGatewayClassParametersRefGVKs extracts ParametersRef GVKs from a GatewayClass.
func collectGatewayClassParametersRefGVKs(
	gatewayClass *gatewayv1.GatewayClass,
	gvks map[string]schema.GroupVersionKind,
) {
	if gatewayClass.Spec.ParametersRef == nil {
		return
	}

	ref := gatewayClass.Spec.ParametersRef

	gvk := schema.GroupVersionKind{
		Group:   string(ref.Group),
		Version: "v1",
		Kind:    string(ref.Kind),
	}

	gvks[gvk.String()] = gvk
}

// collectGatewayParametersRefGVKs extracts ParametersRef GVKs from a Gateway's Infrastructure.
func collectGatewayParametersRefGVKs(
	gateway *gatewayv1.Gateway,
	gvks map[string]schema.GroupVersionKind,
) {
	if gateway.Spec.Infrastructure == nil || gateway.Spec.Infrastructure.ParametersRef == nil {
		return
	}

	ref := gateway.Spec.Infrastructure.ParametersRef

	gvk := schema.GroupVersionKind{
		Group:   string(ref.Group),
		Version: "v1",
		Kind:    string(ref.Kind),
	}

	gvks[gvk.String()] = gvk
}
