package resources

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/sjberman/gateway-lens/internal/topology"
)

// ResourceDescriptor is an exported alias for tests.
type ResourceDescriptor = resourceDescriptor

// ObjectIndex is an exported alias for tests.
type ObjectIndex = objectIndex

// StoreState is an exported alias for tests.
type StoreState = storeState

// NewStoreState wraps newStoreState for testing.
func NewStoreState(descriptors []ResourceDescriptor) StoreState {
	return newStoreState(descriptors)
}

// SortedObjects wraps sortedObjects for testing.
func SortedObjects(index ObjectIndex) []client.Object {
	return sortedObjects(index)
}

// NamespacedNameForObject wraps namespacedNameForObject for testing.
func NamespacedNameForObject(object client.Object) types.NamespacedName {
	return namespacedNameForObject(object)
}

// ProjectValues wraps projectValues for testing.
func ProjectValues[P client.Object, V any](objects []client.Object, clone func(P) V, logger logr.Logger) []V {
	return projectValues(objects, clone, logger)
}

// ResourceDescriptors wraps resourceDescriptors for testing.
func ResourceDescriptors() []ResourceDescriptor {
	return resourceDescriptors()
}

// NewListFunc wraps newListFunc for testing.
func NewListFunc[L client.ObjectList, V any](
	newList func() L,
	items func(L) []V,
	toObject func(*V) client.Object,
	label string,
) func(context.Context, client.Reader) ([]client.Object, error) {
	return newListFunc(newList, items, toObject, label)
}

// NewTestResources creates a Resources with test descriptors for external tests.
func NewTestResources(descriptors []ResourceDescriptor) *Resources {
	return &Resources{
		logger:      logr.Discard(),
		descriptors: descriptors,
		store:       newStoreState(descriptors),
	}
}

// NewTestResourcesWithLogger creates a Resources with specified logger for external tests.
func NewTestResourcesWithLogger(descriptors []ResourceDescriptor, logger logr.Logger) *Resources {
	return &Resources{
		logger:      logger,
		descriptors: descriptors,
		store:       newStoreState(descriptors),
	}
}

// ApplyUpsert wraps applyUpsert for testing.
func (r *Resources) ApplyUpsert(object client.Object) error {
	return r.applyUpsert(object)
}

// ApplyDelete wraps applyDelete for testing.
func (r *Resources) ApplyDelete(objectType client.Object, key types.NamespacedName) error {
	return r.applyDelete(objectType, key)
}

// SyncFromCache wraps syncFromCache for testing.
func (r *Resources) SyncFromCache(ctx context.Context) error {
	return r.syncFromCache(ctx)
}

// StoreLen returns the count of objects for a given type in the store.
func (r *Resources) StoreLen(objectType client.Object) int {
	typeKey := reflect.TypeOf(objectType)

	idx, ok := r.store[typeKey]
	if !ok {
		return -1
	}

	return len(idx)
}

// StoreHasKey checks if a key exists for a given type.
func (r *Resources) StoreHasKey(objectType client.Object, key types.NamespacedName) bool {
	typeKey := reflect.TypeOf(objectType)

	idx, ok := r.store[typeKey]
	if !ok {
		return false
	}

	_, exists := idx[key]

	return exists
}

// StoreGetName returns the name of the stored object for a given type and key.
func (r *Resources) StoreGetName(objectType client.Object, key types.NamespacedName) (string, bool) {
	typeKey := reflect.TypeOf(objectType)

	idx, ok := r.store[typeKey]
	if !ok {
		return "", false
	}

	obj, exists := idx[key]
	if !exists {
		return "", false
	}

	return obj.GetName(), true
}

// StoreGetLabels returns the labels of the stored object for a given type and key.
func (r *Resources) StoreGetLabels(objectType client.Object, key types.NamespacedName) (map[string]string, bool) {
	typeKey := reflect.TypeOf(objectType)

	idx, ok := r.store[typeKey]
	if !ok {
		return nil, false
	}

	obj, exists := idx[key]
	if !exists {
		return nil, false
	}

	return obj.GetLabels(), true
}

// NewTestDescriptor creates a test ResourceDescriptor.
func NewTestDescriptor(
	name string,
	object client.Object,
	list func(context.Context, client.Reader) ([]client.Object, error),
	projectTo func(*topology.GatewayAPIResources, []client.Object, logr.Logger),
) ResourceDescriptor {
	return ResourceDescriptor{
		name:      name,
		object:    object,
		list:      list,
		projectTo: projectTo,
	}
}

// ObjectType returns the descriptor's object reflect type string for comparison.
func (d ResourceDescriptor) ObjectType() string {
	return reflect.TypeOf(d.object).String()
}

// ProjectTo calls the descriptor's projectTo function.
func (d ResourceDescriptor) ProjectTo(res *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
	d.projectTo(res, objects, logger)
}

// ParseTargetRefs wraps parseTargetRefs for testing.
func ParseTargetRefs(obj *unstructured.Unstructured) ([]topology.PolicyTargetRef, error) {
	return parseTargetRefs(obj)
}

// ParsePolicyLabelValue wraps parsePolicyLabelValue for testing.
func ParsePolicyLabelValue(value string) (topology.PolicyType, bool) {
	return parsePolicyLabelValue(value)
}

// PreferredGVK wraps preferredGVK for testing.
func PreferredGVK(crd *apiextensionsv1.CustomResourceDefinition) schema.GroupVersionKind {
	return preferredGVK(crd)
}

// TestCRDVersion is a helper struct for constructing test CRDs.
type TestCRDVersion struct {
	Name   string
	Served bool
}

// NewTestCRD creates an apiextensionsv1.CustomResourceDefinition for testing.
func NewTestCRD(group, kind string, versions []TestCRDVersion) *apiextensionsv1.CustomResourceDefinition {
	crdVersions := make([]apiextensionsv1.CustomResourceDefinitionVersion, len(versions))
	for i, v := range versions {
		crdVersions[i] = apiextensionsv1.CustomResourceDefinitionVersion{
			Name:   v.Name,
			Served: v.Served,
		}
	}

	return &apiextensionsv1.CustomResourceDefinition{
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: kind,
			},
			Versions: crdVersions,
		},
	}
}

// SetPolicyCRDs sets policy CRDs on a PolicyStore for testing.
func (s *PolicyStore) SetPolicyCRDs(crds []PolicyCRD) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.policyCRDs = make(map[string]PolicyCRD, len(crds))
	for _, crd := range crds {
		s.policyCRDs[crd.GVK.String()] = crd
	}
}

// SetExtensionRefGVKs sets known GVKs on an ExtensionRefStore for testing.
func (s *ExtensionRefStore) SetExtensionRefGVKs(gvks []schema.GroupVersionKind) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gvks = make(map[string]schema.GroupVersionKind, len(gvks))
	for _, gvk := range gvks {
		s.gvks[gvk.String()] = gvk
	}
}

// CollectHTTPRouteExtensionRefGVKs wraps collectHTTPRouteExtensionRefGVKs for testing.
func CollectHTTPRouteExtensionRefGVKs(route *gatewayv1.HTTPRoute, gvks map[string]schema.GroupVersionKind) {
	collectHTTPRouteExtensionRefGVKs(route, gvks)
}

// CollectGRPCRouteExtensionRefGVKs wraps collectGRPCRouteExtensionRefGVKs for testing.
func CollectGRPCRouteExtensionRefGVKs(route *gatewayv1.GRPCRoute, gvks map[string]schema.GroupVersionKind) {
	collectGRPCRouteExtensionRefGVKs(route, gvks)
}

// IsRouteType wraps isRouteType for testing.
func IsRouteType(obj client.Object) bool {
	return isRouteType(obj)
}

// IsGatewayClassOrGatewayType wraps isGatewayClassOrGatewayType for testing.
func IsGatewayClassOrGatewayType(obj client.Object) bool {
	return isGatewayClassOrGatewayType(obj)
}

// SetParametersRefGVKs sets known GVKs on a ParametersRefStore for testing.
func (s *ParametersRefStore) SetParametersRefGVKs(gvks []schema.GroupVersionKind) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.gvks = make(map[string]schema.GroupVersionKind, len(gvks))
	for _, gvk := range gvks {
		s.gvks[gvk.String()] = gvk
	}
}

// CollectGatewayClassParametersRefGVKs wraps collectGatewayClassParametersRefGVKs for testing.
func CollectGatewayClassParametersRefGVKs(
	gatewayClass *gatewayv1.GatewayClass,
	gvks map[string]schema.GroupVersionKind,
) {
	collectGatewayClassParametersRefGVKs(gatewayClass, gvks)
}

// CollectGatewayParametersRefGVKs wraps collectGatewayParametersRefGVKs for testing.
func CollectGatewayParametersRefGVKs(
	gateway *gatewayv1.Gateway,
	gvks map[string]schema.GroupVersionKind,
) {
	collectGatewayParametersRefGVKs(gateway, gvks)
}
