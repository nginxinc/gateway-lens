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

// ExtensionRefStore discovers and tracks ExtensionRef CRD instances referenced by routes.
type ExtensionRefStore struct {
	// cache provides read access to cluster resources.
	cache cache.Cache
	// logger is the structured logger for ExtensionRef operations.
	logger logr.Logger

	// mu guards the ExtensionRef state.
	mu sync.RWMutex
	// gvks maps GVK string to its parsed GroupVersionKind.
	gvks map[string]schema.GroupVersionKind
	// objects maps key to unstructured object.
	objects map[objectStoreKey]unstructured.Unstructured
	// excludedGroupKinds contains group/kind pairs already watched by other controllers.
	// These are skipped during discovery to avoid registering duplicate controllers.
	excludedGroupKinds map[string]struct{}
}

// NewExtensionRefStore creates an ExtensionRef store backed by the given cache.
func NewExtensionRefStore(runtimeCache cache.Cache, logger logr.Logger) *ExtensionRefStore {
	return &ExtensionRefStore{
		cache:              runtimeCache,
		logger:             logger,
		gvks:               make(map[string]schema.GroupVersionKind),
		objects:            make(map[objectStoreKey]unstructured.Unstructured),
		excludedGroupKinds: make(map[string]struct{}),
	}
}

// ExcludeGroupKind marks a group/kind pair as already watched, preventing duplicate controller registration.
func (s *ExtensionRefStore) ExcludeGroupKind(group, kind string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.excludedGroupKinds[group+"/"+kind] = struct{}{}
}

// DiscoverGVKs extracts unique ExtensionRef GVKs from the given routes.
// Since ExtensionRef CRDs have no standard label, we infer them from route filter references.
// Route references only include group+kind, so we look up the preferred served version
// from the cluster's CRD definitions.
// Returns the list of newly discovered GVKs (not previously known).
func (s *ExtensionRefStore) DiscoverGVKs(
	ctx context.Context,
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
) ([]schema.GroupVersionKind, error) {
	// Collect group+kind pairs from route filters.
	groupKinds := make(map[string]schema.GroupVersionKind)

	for _, route := range httpRoutes {
		collectHTTPRouteExtensionRefGVKs(&route, groupKinds)
	}

	for _, route := range grpcRoutes {
		collectGRPCRouteExtensionRefGVKs(&route, groupKinds)
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

			s.logger.V(1).Info("Discovered ExtensionRef GVK", "gvk", gvk.String())
		}
	}

	return newGVKs, nil
}

// SyncObjects fetches all instances of the given ExtensionRef GVKs and adds them to the store.
// If gvks is nil, all known GVKs are synced. Existing objects from other GVKs are preserved.
func (s *ExtensionRefStore) SyncObjects(ctx context.Context, gvks []schema.GroupVersionKind) error {
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

	syncObjectsFromCache(ctx, s.cache, gvks, s.objects, "ExtensionRef", s.logger)

	return nil
}

// Upsert adds or updates a single ExtensionRef object in the store.
func (s *ExtensionRefStore) Upsert(obj *unstructured.Unstructured) {
	gvk := obj.GetObjectKind().GroupVersionKind()

	key := objectStoreKey{
		gvk: gvk.String(),
		nn:  types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
	}

	s.mu.Lock()
	s.objects[key] = *obj.DeepCopy()
	s.mu.Unlock()
}

// Delete removes an ExtensionRef object from the store.
func (s *ExtensionRefStore) Delete(gvk schema.GroupVersionKind, nn types.NamespacedName) {
	key := objectStoreKey{gvk: gvk.String(), nn: nn}

	s.mu.Lock()
	delete(s.objects, key)
	s.mu.Unlock()
}

// Get returns a sorted slice of all current ExtensionRef objects.
func (s *ExtensionRefStore) Get() []unstructured.Unstructured {
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
func (s *ExtensionRefStore) resolveGVKs(
	ctx context.Context,
	groupKinds map[string]schema.GroupVersionKind,
) (map[string]schema.GroupVersionKind, error) {
	s.mu.RLock()
	excluded := maps.Clone(s.excludedGroupKinds)
	s.mu.RUnlock()

	return resolveGVKsFromCRDs(ctx, s.cache, groupKinds, excluded, "ExtensionRef")
}

// collectHTTPRouteExtensionRefGVKs extracts ExtensionRef GVKs from an HTTPRoute's filters.
func collectHTTPRouteExtensionRefGVKs(
	route *gatewayv1.HTTPRoute,
	gvks map[string]schema.GroupVersionKind,
) {
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if filter.Type == gatewayv1.HTTPRouteFilterExtensionRef && filter.ExtensionRef != nil {
				addExtRefGVK(gvks, filter.ExtensionRef)
			}
		}

		for _, backendRef := range rule.BackendRefs {
			for _, filter := range backendRef.Filters {
				if filter.Type == gatewayv1.HTTPRouteFilterExtensionRef && filter.ExtensionRef != nil {
					addExtRefGVK(gvks, filter.ExtensionRef)
				}
			}
		}
	}
}

// collectGRPCRouteExtensionRefGVKs extracts ExtensionRef GVKs from a GRPCRoute's filters.
func collectGRPCRouteExtensionRefGVKs(
	route *gatewayv1.GRPCRoute,
	gvks map[string]schema.GroupVersionKind,
) {
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if filter.Type == gatewayv1.GRPCRouteFilterExtensionRef && filter.ExtensionRef != nil {
				addExtRefGVK(gvks, filter.ExtensionRef)
			}
		}

		for _, backendRef := range rule.BackendRefs {
			for _, filter := range backendRef.Filters {
				if filter.Type == gatewayv1.GRPCRouteFilterExtensionRef && filter.ExtensionRef != nil {
					addExtRefGVK(gvks, filter.ExtensionRef)
				}
			}
		}
	}
}

// addExtRefGVK resolves the GVK for a LocalObjectReference and adds it to the map.
// The version is not available from the route reference (only group+kind), so we
// default to "v1" as a reasonable fallback. If the CRD uses a different version,
// the cache List will simply return zero results for this GVK.
func addExtRefGVK(
	gvks map[string]schema.GroupVersionKind,
	ref *gatewayv1.LocalObjectReference,
) {
	gvk := schema.GroupVersionKind{
		Group:   string(ref.Group),
		Version: "v1",
		Kind:    string(ref.Kind),
	}

	gvks[gvk.String()] = gvk
}
