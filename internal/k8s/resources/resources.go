// Package resources provides watch-backed Gateway API resource state.
package resources

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctlrmanager "sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	frameworkcontroller "github.com/sjberman/gateway-lens/internal/k8s/framework/controller"
	frameworkevents "github.com/sjberman/gateway-lens/internal/k8s/framework/events"
	"github.com/sjberman/gateway-lens/internal/topology"
)

var (
	errResourcesCacheSync            = errors.New("waiting for resources cache sync")
	errUnsupportedResourcesEventType = errors.New("unsupported resources event type")
	errUnsupportedStoreObjectType    = errors.New("unsupported store object type")
	errDeepCopyClientObjectType      = errors.New("unexpected deep copied client object type")
	errUnexpectedObjectType          = errors.New("unexpected object type")
)

const resourceEventBufferSize = 128

// objectIndex maps namespaced names to their latest object for a single resource kind.
type objectIndex map[types.NamespacedName]client.Object

// storeState maps each resource type to its object index.
type storeState map[reflect.Type]objectIndex

// Resources keeps the latest Gateway API resources synchronized from controller events.
type Resources struct {
	// cache is the controller-runtime informer cache.
	cache cache.Cache
	// eventCh carries upsert/delete events from reconcilers to the event loop.
	eventCh chan any
	// logger is the structured logger for resource operations.
	logger logr.Logger

	// descriptors define the watched resource kinds and their projections.
	descriptors []resourceDescriptor

	// policyStore discovers and tracks generic Gateway API policy resources.
	policyStore *PolicyStore

	// extensionRefStore discovers and tracks ExtensionRef CRD instances.
	extensionRefStore *ExtensionRefStore

	// parametersRefStore discovers and tracks ParametersRef CRD instances.
	parametersRefStore *ParametersRefStore

	// mu guards the store.
	mu sync.RWMutex
	// store is the in-memory state of all watched resources, keyed by type.
	store storeState

	// subscribersMu guards the subscribers slice.
	subscribersMu sync.Mutex
	// subscribers are channels notified when the store changes.
	subscribers []chan struct{}

	// registrar registers controllers with the manager; stored for deferred policy controller registration.
	registrar ControllerRegistrar
}

// ControllerRegistrar registers per-kind controllers with a manager runtime.
type ControllerRegistrar interface {
	RegisterController(name string, object client.Object, rec reconcile.Reconciler) error
}

// resourceDescriptor defines how a single Gateway API kind is watched, listed, and projected.
type resourceDescriptor struct {
	// name is the controller name registered with the manager.
	name string
	// object is a zero-value exemplar of the resource type.
	object client.Object
	// list fetches all objects of this kind from the cache.
	list func(context.Context, client.Reader) ([]client.Object, error)
	// projectTo copies the objects into the appropriate GatewayAPIResources field.
	projectTo func(*topology.GatewayAPIResources, []client.Object, logr.Logger)
}

var (
	_ ctlrmanager.Runnable         = (*Resources)(nil)
	_ frameworkevents.EventHandler = (*Resources)(nil)
)

// New creates a watch-backed resource state.
func New(runtimeCache cache.Cache, logger logr.Logger) *Resources {
	descriptors := resourceDescriptors()
	extRefStore := NewExtensionRefStore(runtimeCache, logger.WithName("extensionRefs"))
	paramsRefStore := NewParametersRefStore(runtimeCache, logger.WithName("parametersRefs"))

	// Pre-exclude all static descriptor group/kinds so ExtensionRef and ParametersRef discovery
	// won't register duplicate controllers for types we already watch.
	for _, d := range descriptors {
		kind := reflect.TypeOf(d.object).Elem().Name()
		gvk := d.object.GetObjectKind().GroupVersionKind()
		group := gvk.Group

		// Zero-value exemplars don't carry GVK metadata, so fall back to the
		// well-known Gateway API group when the group is empty.
		if group == "" {
			group = gatewayv1.GroupVersion.Group
		}

		extRefStore.ExcludeGroupKind(group, kind)
		paramsRefStore.ExcludeGroupKind(group, kind)
	}

	return &Resources{
		cache:              runtimeCache,
		eventCh:            make(chan any, resourceEventBufferSize),
		logger:             logger,
		descriptors:        descriptors,
		policyStore:        NewPolicyStore(runtimeCache, logger.WithName("policies")),
		extensionRefStore:  extRefStore,
		parametersRefStore: paramsRefStore,
		store:              newStoreState(descriptors),
	}
}

// RegisterControllers registers generic reconcilers for all watched resource kinds.
func (r *Resources) RegisterControllers(registrar ControllerRegistrar) error {
	r.registrar = registrar

	for _, descriptor := range r.descriptors {
		reconciler := frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
			Getter:     r.cache,
			ObjectType: descriptor.object,
			OnUpsert:   r.sendUpsertEvent,
			OnDelete:   r.sendDeleteEvent,
		})

		if err := registrar.RegisterController(descriptor.name, descriptor.object, reconciler); err != nil {
			return fmt.Errorf("setting up resource watches: %w", err)
		}
	}

	return nil
}

// Start waits for cache sync, seeds the store, and starts batched event handling.
func (r *Resources) Start(ctx context.Context) error {
	r.logger.V(1).Info("Waiting for cache sync")

	if !r.cache.WaitForCacheSync(ctx) {
		return errResourcesCacheSync
	}

	r.logger.V(1).Info("Cache synced, seeding store from cache")

	if err := r.syncFromCache(ctx); err != nil {
		return err
	}

	if err := r.discoverAndSyncPolicies(ctx); err != nil {
		return err
	}

	if err := r.discoverAndSyncExtensionRefs(ctx); err != nil {
		return err
	}

	if err := r.discoverAndSyncParametersRefs(ctx); err != nil {
		return err
	}

	r.logger.V(1).Info("Store seeded, starting event loop")

	loop := frameworkevents.NewEventLoop(r.eventCh, r, r.logger)

	if err := loop.Start(ctx); err != nil {
		return fmt.Errorf("starting resources event loop: %w", err)
	}

	return nil
}

// HandleEventBatch applies resource update events to the in-memory store.
func (r *Resources) HandleEventBatch(ctx context.Context, batch frameworkevents.EventBatch) {
	r.logger.V(1).Info("Processing event batch", "events", len(batch))

	routeChanged, gatewayOrClassChanged := r.applyEventBatch(batch)

	if routeChanged {
		if err := r.discoverAndSyncExtensionRefs(ctx); err != nil {
			r.logger.Error(err, "Failed to discover ExtensionRef GVKs from route change")
		}
	}

	if gatewayOrClassChanged {
		if err := r.discoverAndSyncParametersRefs(ctx); err != nil {
			r.logger.Error(err, "Failed to discover ParametersRef GVKs from GatewayClass/Gateway change")
		}
	}

	r.notifySubscribers()
}

// Get returns a copy of the latest synchronized resources.
func (r *Resources) Get() topology.GatewayAPIResources {
	r.mu.RLock()
	defer r.mu.RUnlock()

	resources := topology.GatewayAPIResources{}

	for _, descriptor := range r.descriptors {
		objects := sortedObjects(r.store[reflect.TypeOf(descriptor.object)])
		descriptor.projectTo(&resources, objects, r.logger)
	}

	if r.policyStore != nil {
		resources.Policies = r.policyStore.Get()
	}

	if r.extensionRefStore != nil {
		resources.ExtensionRefs = r.extensionRefStore.Get()
	}

	if r.parametersRefStore != nil {
		resources.ParametersRefs = r.parametersRefStore.Get()
	}

	return resources
}

// Subscribe returns a channel that receives a notification each time the resource store changes.
// Callers must call Unsubscribe when done to avoid leaking the channel.
func (r *Resources) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)

	r.subscribersMu.Lock()
	r.subscribers = append(r.subscribers, ch)
	r.subscribersMu.Unlock()

	return ch
}

// Unsubscribe removes a previously subscribed channel and closes it.
func (r *Resources) Unsubscribe(ch <-chan struct{}) {
	r.subscribersMu.Lock()
	defer r.subscribersMu.Unlock()

	for i, subscriber := range r.subscribers {
		if subscriber == ch {
			r.subscribers = append(r.subscribers[:i], r.subscribers[i+1:]...)

			close(subscriber)

			return
		}
	}
}

// applyEventBatch processes all events in the batch under a single lock,
// returning whether any route or GatewayClass/Gateway resources changed.
func (r *Resources) applyEventBatch(batch frameworkevents.EventBatch) (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var routeChanged, gatewayOrClassChanged bool

	for _, rawEvent := range batch {
		switch event := rawEvent.(type) {
		case *frameworkevents.UpsertEvent:
			r.logger.V(1).Info("Upserting resource",
				"type", reflect.TypeOf(event.Resource).Elem().Name(),
				"name", namespacedNameForObject(event.Resource),
			)

			if err := r.applyUpsert(event.Resource); err != nil {
				r.logger.Error(err, "Failed to upsert resource")
			}

			if isRouteType(event.Resource) {
				routeChanged = true
			}

			if isGatewayClassOrGatewayType(event.Resource) {
				gatewayOrClassChanged = true
			}
		case *frameworkevents.DeleteEvent:
			r.logger.V(1).Info("Deleting resource",
				"type", reflect.TypeOf(event.Type).Elem().Name(),
				"name", event.NamespacedName,
			)

			if err := r.applyDelete(event.Type, event.NamespacedName); err != nil {
				r.logger.Error(err, "Failed to delete resource")
			}
		default:
			r.logger.Error(
				fmt.Errorf("%w: %T", errUnsupportedResourcesEventType, rawEvent),
				"Unknown event type",
			)
		}
	}

	return routeChanged, gatewayOrClassChanged
}

// sendUpsertEvent sends an upsert event on the event channel.
func (r *Resources) sendUpsertEvent(ctx context.Context, obj client.Object) {
	select {
	case r.eventCh <- &frameworkevents.UpsertEvent{Resource: obj}:
	case <-ctx.Done():
	}
}

// sendDeleteEvent sends a delete event on the event channel.
func (r *Resources) sendDeleteEvent(ctx context.Context, objectType client.Object, nn types.NamespacedName) {
	select {
	case r.eventCh <- &frameworkevents.DeleteEvent{Type: objectType, NamespacedName: nn}:
	case <-ctx.Done():
	}
}

// notifySubscribers sends a non-blocking notification to all subscribers.
func (r *Resources) notifySubscribers() {
	r.subscribersMu.Lock()
	defer r.subscribersMu.Unlock()

	for _, ch := range r.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// discoverAndSyncPolicies discovers policy CRDs, performs an initial policy sync,
// and registers controllers for each discovered policy kind.
func (r *Resources) discoverAndSyncPolicies(ctx context.Context) error {
	if r.policyStore == nil {
		return nil
	}

	discovered, err := r.policyStore.DiscoverPolicyCRDs(ctx)
	if err != nil {
		return fmt.Errorf("discovering policy CRDs: %w", err)
	}

	if err := r.policyStore.SyncPolicies(ctx); err != nil {
		return fmt.Errorf("syncing policies: %w", err)
	}

	gvks := make([]schema.GroupVersionKind, len(discovered))
	for i, crd := range discovered {
		gvks[i] = crd.GVK
	}

	if err := r.registerDynamicControllers(
		gvks,
		"policy",
		func(u *unstructured.Unstructured) { r.policyStore.UpsertPolicy(u) },
		func(gvk schema.GroupVersionKind, nn types.NamespacedName) { r.policyStore.DeletePolicy(gvk, nn) },
	); err != nil {
		return fmt.Errorf("registering policy controllers: %w", err)
	}

	// Exclude discovered policy group/kinds from ExtensionRef and ParametersRef discovery
	// so we don't register duplicate controllers for the same types.
	for _, gvk := range gvks {
		r.extensionRefStore.ExcludeGroupKind(gvk.Group, gvk.Kind)
		r.parametersRefStore.ExcludeGroupKind(gvk.Group, gvk.Kind)
	}

	return nil
}

// discoverAndSyncExtensionRefs discovers ExtensionRef GVKs from synced routes,
// fetches all instances, and registers controllers for ongoing updates.
// It is called both at startup and whenever routes change.
func (r *Resources) discoverAndSyncExtensionRefs(ctx context.Context) error {
	if r.extensionRefStore == nil {
		return nil
	}

	httpRoutes, grpcRoutes := r.extractRoutes()

	return r.discoverAndSyncDynamic(dynamicRefConfig{
		label:  "ExtensionRef",
		prefix: "extensionref",
		discover: func() ([]schema.GroupVersionKind, error) {
			return r.extensionRefStore.DiscoverGVKs(ctx, httpRoutes, grpcRoutes)
		},
		upsert: func(u *unstructured.Unstructured) { r.extensionRefStore.Upsert(u) },
		delete: func(gvk schema.GroupVersionKind, nn types.NamespacedName) { r.extensionRefStore.Delete(gvk, nn) },
		sync:   func(gvks []schema.GroupVersionKind) error { return r.extensionRefStore.SyncObjects(ctx, gvks) },
	})
}

func (r *Resources) discoverAndSyncParametersRefs(ctx context.Context) error {
	if r.parametersRefStore == nil {
		return nil
	}

	gatewayClasses, gateways := r.extractGatewayClassesAndGateways()

	return r.discoverAndSyncDynamic(dynamicRefConfig{
		label:  "ParametersRef",
		prefix: "parametersref",
		discover: func() ([]schema.GroupVersionKind, error) {
			return r.parametersRefStore.DiscoverGVKs(ctx, gatewayClasses, gateways)
		},
		upsert: func(u *unstructured.Unstructured) { r.parametersRefStore.Upsert(u) },
		delete: func(gvk schema.GroupVersionKind, nn types.NamespacedName) { r.parametersRefStore.Delete(gvk, nn) },
		sync:   func(gvks []schema.GroupVersionKind) error { return r.parametersRefStore.SyncObjects(ctx, gvks) },
	})
}

// dynamicRefConfig holds the varying parts for dynamic CRD discovery and sync.
type dynamicRefConfig struct {
	label    string
	prefix   string
	discover func() ([]schema.GroupVersionKind, error)
	upsert   func(*unstructured.Unstructured)
	delete   func(schema.GroupVersionKind, types.NamespacedName)
	sync     func([]schema.GroupVersionKind) error
}

func (r *Resources) discoverAndSyncDynamic(cfg dynamicRefConfig) error {
	discovered, err := cfg.discover()
	if err != nil {
		return fmt.Errorf("discovering %s GVKs: %w", cfg.label, err)
	}

	if len(discovered) == 0 {
		return nil
	}

	if err := r.registerDynamicControllers(discovered, cfg.prefix, cfg.upsert, cfg.delete); err != nil {
		return fmt.Errorf("registering %s controllers: %w", cfg.label, err)
	}

	if err := cfg.sync(discovered); err != nil {
		return fmt.Errorf("syncing %s objects: %w", cfg.label, err)
	}

	return nil
}

// extractGatewayClassesAndGateways reads the current GatewayClasses and Gateways from the store.
func (r *Resources) extractGatewayClassesAndGateways() ([]gatewayv1.GatewayClass, []gatewayv1.Gateway) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		gatewayClasses []gatewayv1.GatewayClass
		gateways       []gatewayv1.Gateway
	)

	for _, idx := range r.store {
		for _, obj := range idx {
			switch resource := obj.(type) {
			case *gatewayv1.GatewayClass:
				gatewayClasses = append(gatewayClasses, *resource)
			case *gatewayv1.Gateway:
				gateways = append(gateways, *resource)
			}
		}
	}

	return gatewayClasses, gateways
}

// extractRoutes reads the current HTTPRoutes and GRPCRoutes from the store.
func (r *Resources) extractRoutes() ([]gatewayv1.HTTPRoute, []gatewayv1.GRPCRoute) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		httpRoutes []gatewayv1.HTTPRoute
		grpcRoutes []gatewayv1.GRPCRoute
	)

	for _, idx := range r.store {
		for _, obj := range idx {
			switch route := obj.(type) {
			case *gatewayv1.HTTPRoute:
				httpRoutes = append(httpRoutes, *route)
			case *gatewayv1.GRPCRoute:
				grpcRoutes = append(grpcRoutes, *route)
			}
		}
	}

	return httpRoutes, grpcRoutes
}

// registerDynamicControllers registers a controller for each GVK, wiring the provided
// upsert and delete callbacks. This is used for both policy and ExtensionRef controllers.
func (r *Resources) registerDynamicControllers(
	gvks []schema.GroupVersionKind,
	prefix string,
	onUpsert func(*unstructured.Unstructured),
	onDelete func(schema.GroupVersionKind, types.NamespacedName),
) error {
	if r.registrar == nil {
		return nil
	}

	for _, gvk := range gvks {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)

		reconciler := frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
			Getter:     r.cache,
			ObjectType: obj,
			OnUpsert: func(_ context.Context, obj client.Object) {
				u, ok := obj.(*unstructured.Unstructured)
				if !ok {
					return
				}

				onUpsert(u)
				r.notifySubscribers()
			},
			OnDelete: func(_ context.Context, _ client.Object, nn types.NamespacedName) {
				onDelete(gvk, nn)
				r.notifySubscribers()
			},
		})

		name := prefix + "-" + strings.ToLower(gvk.Kind)

		if err := r.registrar.RegisterController(name, obj, reconciler); err != nil {
			return fmt.Errorf("registering %s controller for %s: %w", prefix, gvk.String(), err)
		}

		r.logger.V(1).Info("Registered dynamic controller", "prefix", prefix, "gvk", gvk.String())
	}

	return nil
}

// applyUpsert stores a deep copy of the object in the appropriate type index.
func (r *Resources) applyUpsert(object client.Object) error {
	typeKey := reflect.TypeOf(object)
	if _, exists := r.store[typeKey]; !exists {
		return fmt.Errorf("%w: %T", errUnsupportedStoreObjectType, object)
	}

	copied, ok := object.DeepCopyObject().(client.Object)
	if !ok {
		return fmt.Errorf("%w: %T", errDeepCopyClientObjectType, object)
	}

	key := namespacedNameForObject(copied)
	r.store[typeKey][key] = copied

	return nil
}

// applyDelete removes an object from the appropriate type index.
func (r *Resources) applyDelete(objectType client.Object, key types.NamespacedName) error {
	typeKey := reflect.TypeOf(objectType)
	if _, exists := r.store[typeKey]; !exists {
		return fmt.Errorf("%w: %T", errUnsupportedStoreObjectType, objectType)
	}

	delete(r.store[typeKey], key)

	return nil
}

// syncFromCache seeds the store from the informer cache on startup.
func (r *Resources) syncFromCache(ctx context.Context) error {
	seededStore := newStoreState(r.descriptors)

	for _, descriptor := range r.descriptors {
		objects, err := descriptor.list(ctx, r.cache)
		if err != nil {
			return err
		}

		index := make(objectIndex, len(objects))
		for _, object := range objects {
			index[namespacedNameForObject(object)] = object
		}

		seededStore[reflect.TypeOf(descriptor.object)] = index
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.store = seededStore

	return nil
}

// newStoreState creates an empty store with an index for each descriptor.
func newStoreState(descriptors []resourceDescriptor) storeState {
	store := make(storeState, len(descriptors))
	for _, descriptor := range descriptors {
		store[reflect.TypeOf(descriptor.object)] = make(objectIndex)
	}

	return store
}

// sortedObjects returns the objects from an index sorted by namespace then name.
func sortedObjects(index objectIndex) []client.Object {
	keys := make([]types.NamespacedName, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}

	slices.SortFunc(keys, func(a, b types.NamespacedName) int {
		if a.Namespace != b.Namespace {
			if a.Namespace < b.Namespace {
				return -1
			}

			return 1
		}

		if a.Name < b.Name {
			return -1
		}

		if a.Name > b.Name {
			return 1
		}

		return 0
	})

	objects := make([]client.Object, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, index[key])
	}

	return objects
}

// namespacedNameForObject extracts the NamespacedName from a client.Object.
func namespacedNameForObject(object client.Object) types.NamespacedName {
	return types.NamespacedName{Namespace: object.GetNamespace(), Name: object.GetName()}
}

// projectValues type-asserts client.Objects into typed domain values via the convert function.
func projectValues[P client.Object, V any](objects []client.Object, convert func(P) V, logger logr.Logger) []V {
	values := make([]V, 0, len(objects))
	for _, object := range objects {
		typed, ok := object.(P)
		if !ok {
			logger.Error(
				fmt.Errorf("%w: expected %T, got %T", errUnexpectedObjectType, *new(P), object),
				"Skipping object with unexpected type during projection",
			)

			continue
		}

		values = append(values, convert(typed))
	}

	return values
}

// isRouteType returns true if the object is a Gateway API route type that can reference ExtensionRefs.
func isRouteType(obj client.Object) bool {
	switch obj.(type) {
	case *gatewayv1.HTTPRoute, *gatewayv1.GRPCRoute:
		return true
	default:
		return false
	}
}

// isGatewayClassOrGatewayType returns true if the object is a GatewayClass or Gateway
// that can reference ParametersRefs.
func isGatewayClassOrGatewayType(obj client.Object) bool {
	switch obj.(type) {
	case *gatewayv1.GatewayClass, *gatewayv1.Gateway:
		return true
	default:
		return false
	}
}
