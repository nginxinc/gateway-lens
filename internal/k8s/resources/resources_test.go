package resources_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	frameworkevents "github.com/sjberman/gateway-lens/internal/k8s/framework/events"
	"github.com/sjberman/gateway-lens/internal/k8s/resources"
	"github.com/sjberman/gateway-lens/internal/topology"
)

var errListFailed = errors.New("list failed")

const (
	testDescGateway   = "test-Gateway"
	testDescHTTPRoute = "test-HTTPRoute"
	nsDefault         = "default"
	nameMyGateway     = "my-gateway"
	nameGW1           = "gw-1"
	nameGW2           = "gw-2"
)

// ---- Helpers ----

func testDescriptors() []resources.ResourceDescriptor {
	return []resources.ResourceDescriptor{
		resources.NewTestDescriptor(
			testDescGateway,
			&gatewayv1.Gateway{},
			func(_ context.Context, _ client.Reader) ([]client.Object, error) {
				return nil, nil
			},
			func(res *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				res.Gateways = resources.ProjectValues(
					objects,
					func(item *gatewayv1.Gateway) gatewayv1.Gateway { return *item.DeepCopy() },
					logger,
				)
			},
		),
		resources.NewTestDescriptor(
			testDescHTTPRoute,
			&gatewayv1.HTTPRoute{},
			func(_ context.Context, _ client.Reader) ([]client.Object, error) {
				return nil, nil
			},
			func(res *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				res.HTTPRoutes = resources.ProjectValues(
					objects,
					func(item *gatewayv1.HTTPRoute) gatewayv1.HTTPRoute { return *item.DeepCopy() },
					logger,
				)
			},
		),
	}
}

func testRes() *resources.Resources {
	return resources.NewTestResources(testDescriptors())
}

func gateway(ns, name string) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

func httpRoute(ns, name string) *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}
}

// ---- NewStoreState ----

func TestNewStoreState(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	descriptors := testDescriptors()
	store := resources.NewStoreState(descriptors)

	g.Expect(store).To(HaveLen(2))
}

// ---- SortedObjects ----

func TestSortedObjects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		index    resources.ObjectIndex
		expected []types.NamespacedName
	}{
		{
			name:     "empty index",
			index:    resources.ObjectIndex{},
			expected: nil,
		},
		{
			name: "single entry",
			index: resources.ObjectIndex{
				{Namespace: "ns", Name: "a"}: gateway("ns", "a"),
			},
			expected: []types.NamespacedName{{Namespace: "ns", Name: "a"}},
		},
		{
			name: "sorted by namespace then name",
			index: resources.ObjectIndex{
				{Namespace: "b", Name: "z"}: gateway("b", "z"),
				{Namespace: "a", Name: "y"}: gateway("a", "y"),
				{Namespace: "a", Name: "x"}: gateway("a", "x"),
			},
			expected: []types.NamespacedName{
				{Namespace: "a", Name: "x"},
				{Namespace: "a", Name: "y"},
				{Namespace: "b", Name: "z"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)

			objects := resources.SortedObjects(tc.index)

			if tc.expected == nil {
				g.Expect(objects).To(BeEmpty())

				return
			}

			g.Expect(objects).To(HaveLen(len(tc.expected)))

			for i, obj := range objects {
				g.Expect(resources.NamespacedNameForObject(obj)).To(Equal(tc.expected[i]))
			}
		})
	}
}

// ---- NamespacedNameForObject ----

func TestNamespacedNameForObject(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	gw := gateway("my-ns", "my-gw")
	nn := resources.NamespacedNameForObject(gw)
	g.Expect(nn).To(Equal(types.NamespacedName{Namespace: "my-ns", Name: "my-gw"}))
}

// ---- ProjectValues ----

func TestProjectValues(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	objects := []client.Object{
		gateway("ns", "gw-a"),
		gateway("ns", "gw-b"),
	}

	vals := resources.ProjectValues(
		objects,
		func(item *gatewayv1.Gateway) gatewayv1.Gateway { return *item.DeepCopy() },
		logr.Discard(),
	)

	g.Expect(vals).To(HaveLen(2))
	g.Expect(vals[0].Name).To(Equal("gw-a"))
	g.Expect(vals[1].Name).To(Equal("gw-b"))
}

func TestProjectValuesSkipsWrongType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	// Mix Gateway and Pod objects — Pod should be skipped.
	objects := []client.Object{
		gateway("ns", "gw-a"),
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "pod-a"}},
		gateway("ns", "gw-b"),
	}

	logger := testr.New(t)

	vals := resources.ProjectValues(
		objects,
		func(item *gatewayv1.Gateway) gatewayv1.Gateway { return *item.DeepCopy() },
		logger,
	)

	g.Expect(vals).To(HaveLen(2))
	g.Expect(vals[0].Name).To(Equal("gw-a"))
	g.Expect(vals[1].Name).To(Equal("gw-b"))
}

func TestProjectValuesEmpty(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	vals := resources.ProjectValues(
		nil,
		func(item *gatewayv1.Gateway) gatewayv1.Gateway { return *item.DeepCopy() },
		logr.Discard(),
	)

	g.Expect(vals).To(BeEmpty())
}

// ---- ApplyUpsert / ApplyDelete ----

func TestApplyUpsert(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()
	gw := gateway(nsDefault, nameMyGateway)

	err := r.ApplyUpsert(gw)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(1))

	name, ok := r.StoreGetName(&gatewayv1.Gateway{}, types.NamespacedName{Namespace: nsDefault, Name: nameMyGateway})
	g.Expect(ok).To(BeTrue())
	g.Expect(name).To(Equal(nameMyGateway))

	// Verify deep copy: mutating original should not affect stored.
	gw.Name = "mutated"

	name, ok = r.StoreGetName(&gatewayv1.Gateway{}, types.NamespacedName{Namespace: nsDefault, Name: nameMyGateway})
	g.Expect(ok).To(BeTrue())
	g.Expect(name).To(Equal(nameMyGateway))
}

func TestApplyUpsertUpdate(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	gw := gateway(nsDefault, nameMyGateway)
	gw.Labels = map[string]string{"version": "1"}
	g.Expect(r.ApplyUpsert(gw)).To(Succeed())

	// Update with new labels.
	gw2 := gateway(nsDefault, nameMyGateway)
	gw2.Labels = map[string]string{"version": "2"}
	g.Expect(r.ApplyUpsert(gw2)).To(Succeed())

	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(1))

	labels, ok := r.StoreGetLabels(&gatewayv1.Gateway{}, types.NamespacedName{Namespace: nsDefault, Name: nameMyGateway})
	g.Expect(ok).To(BeTrue())
	g.Expect(labels).To(HaveKeyWithValue("version", "2"))
}

func TestApplyUpsertUnsupportedType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	// corev1.Pod is not in the store.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: nsDefault, Name: "my-pod"},
	}

	err := r.ApplyUpsert(pod)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unsupported store object type"))
}

func TestApplyDelete(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	gw := gateway(nsDefault, nameMyGateway)
	g.Expect(r.ApplyUpsert(gw)).To(Succeed())

	err := r.ApplyDelete(&gatewayv1.Gateway{}, types.NamespacedName{Namespace: nsDefault, Name: nameMyGateway})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(0))
}

func TestApplyDeleteNonExistentKey(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	// Deleting a key that doesn't exist should not error.
	err := r.ApplyDelete(&gatewayv1.Gateway{}, types.NamespacedName{Namespace: nsDefault, Name: "nonexistent"})
	g.Expect(err).ToNot(HaveOccurred())
}

func TestApplyDeleteUnsupportedType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	err := r.ApplyDelete(&corev1.Pod{}, types.NamespacedName{Namespace: nsDefault, Name: "my-pod"})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unsupported store object type"))
}

// ---- HandleEventBatch ----

func TestHandleEventBatchUpsert(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	batch := frameworkevents.EventBatch{
		&frameworkevents.UpsertEvent{Resource: gateway(nsDefault, nameMyGateway)},
	}

	r.HandleEventBatch(t.Context(), batch)

	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(1))
}

func TestHandleEventBatchDelete(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	// Insert first.
	g.Expect(r.ApplyUpsert(gateway(nsDefault, nameMyGateway))).To(Succeed())

	batch := frameworkevents.EventBatch{
		&frameworkevents.DeleteEvent{
			Type:           &gatewayv1.Gateway{},
			NamespacedName: types.NamespacedName{Namespace: nsDefault, Name: nameMyGateway},
		},
	}

	r.HandleEventBatch(t.Context(), batch)

	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(0))
}

func TestHandleEventBatchUnknownType(t *testing.T) {
	t.Parallel()

	r := testRes()

	// Should log error but not panic.
	batch := frameworkevents.EventBatch{"not-a-valid-event"}

	r.HandleEventBatch(t.Context(), batch)
}

func TestHandleEventBatchMixedEvents(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	// Upsert two gateways, then delete one.
	batch := frameworkevents.EventBatch{
		&frameworkevents.UpsertEvent{Resource: gateway(nsDefault, nameGW1)},
		&frameworkevents.UpsertEvent{Resource: gateway(nsDefault, nameGW2)},
		&frameworkevents.DeleteEvent{
			Type:           &gatewayv1.Gateway{},
			NamespacedName: types.NamespacedName{Namespace: nsDefault, Name: nameGW1},
		},
	}

	r.HandleEventBatch(t.Context(), batch)

	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(1))
	g.Expect(r.StoreHasKey(&gatewayv1.Gateway{}, types.NamespacedName{Namespace: nsDefault, Name: nameGW2})).
		To(BeTrue())
}

// ---- Get ----

func TestGetReturnsProjectedResources(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	g.Expect(r.ApplyUpsert(gateway("ns-b", nameGW2))).To(Succeed())
	g.Expect(r.ApplyUpsert(gateway("ns-a", nameGW1))).To(Succeed())
	g.Expect(r.ApplyUpsert(httpRoute("ns-a", "route-1"))).To(Succeed())

	got := r.Get()

	// Gateways should be sorted by namespace then name.
	g.Expect(got.Gateways).To(HaveLen(2))
	g.Expect(got.Gateways[0].Name).To(Equal(nameGW1))
	g.Expect(got.Gateways[0].Namespace).To(Equal("ns-a"))
	g.Expect(got.Gateways[1].Name).To(Equal(nameGW2))
	g.Expect(got.Gateways[1].Namespace).To(Equal("ns-b"))

	g.Expect(got.HTTPRoutes).To(HaveLen(1))
	g.Expect(got.HTTPRoutes[0].Name).To(Equal("route-1"))
}

func TestGetEmptyStore(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	got := r.Get()

	g.Expect(got.Gateways).To(BeEmpty())
	g.Expect(got.HTTPRoutes).To(BeEmpty())
}

// ---- SyncFromCache ----

func TestSyncFromCache(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	gw := gateway(nsDefault, "my-gw")
	route := httpRoute(nsDefault, "my-route")

	descriptors := []resources.ResourceDescriptor{
		resources.NewTestDescriptor(
			testDescGateway,
			&gatewayv1.Gateway{},
			func(_ context.Context, _ client.Reader) ([]client.Object, error) {
				return []client.Object{gw}, nil
			},
			func(res *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				res.Gateways = resources.ProjectValues(
					objects,
					func(item *gatewayv1.Gateway) gatewayv1.Gateway { return *item.DeepCopy() },
					logger,
				)
			},
		),
		resources.NewTestDescriptor(
			testDescHTTPRoute,
			&gatewayv1.HTTPRoute{},
			func(_ context.Context, _ client.Reader) ([]client.Object, error) {
				return []client.Object{route}, nil
			},
			func(res *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				res.HTTPRoutes = resources.ProjectValues(
					objects,
					func(item *gatewayv1.HTTPRoute) gatewayv1.HTTPRoute { return *item.DeepCopy() },
					logger,
				)
			},
		),
	}

	r := resources.NewTestResources(descriptors)

	err := r.SyncFromCache(t.Context())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(r.StoreLen(&gatewayv1.Gateway{})).To(Equal(1))
	g.Expect(r.StoreLen(&gatewayv1.HTTPRoute{})).To(Equal(1))
}

func TestSyncFromCacheError(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	descriptors := []resources.ResourceDescriptor{
		resources.NewTestDescriptor(
			testDescGateway,
			&gatewayv1.Gateway{},
			func(_ context.Context, _ client.Reader) ([]client.Object, error) {
				return nil, errListFailed
			},
			nil,
		),
	}

	r := resources.NewTestResources(descriptors)

	err := r.SyncFromCache(t.Context())
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("list failed"))
}

// ---- Subscribe / Unsubscribe ----

func TestSubscribeReceivesNotificationOnBatch(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	ch := r.Subscribe()
	defer r.Unsubscribe(ch)

	batch := frameworkevents.EventBatch{
		&frameworkevents.UpsertEvent{Resource: gateway(nsDefault, nameMyGateway)},
	}

	r.HandleEventBatch(t.Context(), batch)

	g.Eventually(ch).Should(Receive())
}

func TestSubscribeMultipleSubscribers(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	ch1 := r.Subscribe()
	defer r.Unsubscribe(ch1)

	ch2 := r.Subscribe()
	defer r.Unsubscribe(ch2)

	batch := frameworkevents.EventBatch{
		&frameworkevents.UpsertEvent{Resource: gateway(nsDefault, nameMyGateway)},
	}

	r.HandleEventBatch(t.Context(), batch)

	g.Eventually(ch1).Should(Receive())
	g.Eventually(ch2).Should(Receive())
}

func TestUnsubscribeStopsNotifications(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	r := testRes()

	ch := r.Subscribe()
	r.Unsubscribe(ch)

	batch := frameworkevents.EventBatch{
		&frameworkevents.UpsertEvent{Resource: gateway(nsDefault, nameMyGateway)},
	}

	r.HandleEventBatch(t.Context(), batch)

	g.Consistently(ch).ShouldNot(Receive())
}

// ---- Full descriptors count ----

func TestResourceDescriptorsCount(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	descriptors := resources.ResourceDescriptors()
	g.Expect(descriptors).To(HaveLen(10))
}
