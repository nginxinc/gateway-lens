package resources_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	frameworkcontroller "github.com/sjberman/gateway-lens/internal/k8s/framework/controller"
	"github.com/sjberman/gateway-lens/internal/k8s/resources"
	"github.com/sjberman/gateway-lens/internal/topology"
)

const (
	groupGatewayAPI  = "gateway.networking.k8s.io"
	kindGateway      = "Gateway"
	kindRateLimit    = "RateLimit"
	groupExampleIO   = "example.io"
	keyTargetRef     = "targetRef"
	keyName          = "name"
	keyGroup         = "group"
	keyKind          = "kind"
	nameRateLimitOne = "rate-limit-1"
)

var errConnectionRefused = errors.New("connection refused")

// ---- ParseTargetRefs ----

func TestParseTargetRefs(t *testing.T) {
	t.Parallel()

	for _, testCase := range parseTargetRefsTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			refs, err := resources.ParseTargetRefs(testCase.obj)

			if testCase.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(refs).To(Equal(testCase.expectedRefs))
			}
		})
	}
}

type parseTargetRefsTestCase struct {
	name         string
	obj          *unstructured.Unstructured
	expectedRefs []topology.PolicyTargetRef
	expectErr    bool
}

func parseTargetRefsTestCases() []parseTargetRefsTestCase {
	return []parseTargetRefsTestCase{
		singularTargetRefCase(),
		pluralTargetRefsCase(),
		bothTargetRefAndRefsCase(),
		noTargetRefsCase(),
		targetRefWithExplicitNamespaceCase(),
	}
}

func singularTargetRefCase() parseTargetRefsTestCase {
	return parseTargetRefsTestCase{
		name: "parses singular targetRef",
		obj: newPolicyUnstructured("my-policy", map[string]any{
			keyTargetRef: map[string]any{
				keyGroup: groupGatewayAPI,
				keyKind:  kindGateway,
				keyName:  "my-gateway",
			},
		}),
		expectedRefs: []topology.PolicyTargetRef{
			{
				Group:     groupGatewayAPI,
				Kind:      kindGateway,
				Namespace: nsDefault,
				Name:      "my-gateway",
			},
		},
	}
}

func pluralTargetRefsCase() parseTargetRefsTestCase {
	return parseTargetRefsTestCase{
		name: "parses plural targetRefs",
		obj: newPolicyUnstructured("my-policy", map[string]any{
			"targetRefs": []any{
				map[string]any{
					keyGroup: groupGatewayAPI,
					keyKind:  kindGateway,
					keyName:  nameGW1,
				},
				map[string]any{
					keyGroup: "",
					keyKind:  "Service",
					keyName:  "svc-1",
				},
			},
		}),
		expectedRefs: []topology.PolicyTargetRef{
			{
				Group:     groupGatewayAPI,
				Kind:      kindGateway,
				Namespace: nsDefault,
				Name:      nameGW1,
			},
			{
				Group:     "",
				Kind:      "Service",
				Namespace: nsDefault,
				Name:      "svc-1",
			},
		},
	}
}

func bothTargetRefAndRefsCase() parseTargetRefsTestCase {
	return parseTargetRefsTestCase{
		name: "deduplicates when both targetRef and targetRefs have same entry",
		obj: newPolicyUnstructured("my-policy", map[string]any{
			keyTargetRef: map[string]any{
				keyGroup: groupGatewayAPI,
				keyKind:  kindGateway,
				keyName:  nameGW1,
			},
			"targetRefs": []any{
				map[string]any{
					keyGroup: groupGatewayAPI,
					keyKind:  kindGateway,
					keyName:  nameGW1,
				},
			},
		}),
		expectedRefs: []topology.PolicyTargetRef{
			{
				Group:     groupGatewayAPI,
				Kind:      kindGateway,
				Namespace: nsDefault,
				Name:      nameGW1,
			},
		},
	}
}

func noTargetRefsCase() parseTargetRefsTestCase {
	return parseTargetRefsTestCase{
		name:         "returns empty when no targetRef or targetRefs",
		obj:          newPolicyUnstructured("my-policy", map[string]any{}),
		expectedRefs: nil,
	}
}

func targetRefWithExplicitNamespaceCase() parseTargetRefsTestCase {
	return parseTargetRefsTestCase{
		name: "uses explicit namespace from targetRef",
		obj: newPolicyUnstructured("my-policy", map[string]any{
			keyTargetRef: map[string]any{
				keyGroup:    groupGatewayAPI,
				keyKind:     kindGateway,
				keyName:     "gw-other",
				keyNamespace: "other-ns",
			},
		}),
		expectedRefs: []topology.PolicyTargetRef{
			{
				Group:     groupGatewayAPI,
				Kind:      kindGateway,
				Namespace: "other-ns",
				Name:      "gw-other",
			},
		},
	}
}

// ---- ParsePolicyLabelValue ----

func TestParsePolicyLabelValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        string
		expectedType topology.PolicyType
		expectedOK   bool
	}{
		{name: "direct", value: "direct", expectedType: topology.PolicyTypeDirect, expectedOK: true},
		{name: "inherited", value: "inherited", expectedType: topology.PolicyTypeInherited, expectedOK: true},
		{name: "true", value: "true", expectedType: topology.PolicyTypeDirect, expectedOK: true},
		{name: "Direct uppercase", value: "Direct", expectedType: topology.PolicyTypeDirect, expectedOK: true},
		{name: "INHERITED uppercase", value: "INHERITED", expectedType: topology.PolicyTypeInherited, expectedOK: true},
		{name: "invalid", value: "invalid", expectedType: "", expectedOK: false},
		{name: "empty", value: "", expectedType: "", expectedOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			policyType, ok := resources.ParsePolicyLabelValue(tt.value)
			g.Expect(ok).To(Equal(tt.expectedOK))
			g.Expect(policyType).To(Equal(tt.expectedType))
		})
	}
}

// ---- PreferredGVK ----

func TestPreferredGVK(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	crd := resources.NewTestCRD(groupExampleIO, "TestPolicy", []resources.TestCRDVersion{
		{Name: "v1alpha1", Served: false},
		{Name: "v1beta1", Served: true},
		{Name: "v1", Served: true},
	})

	gvk := resources.PreferredGVK(crd)
	g.Expect(gvk).To(Equal(schema.GroupVersionKind{
		Group:   groupExampleIO,
		Version: "v1beta1",
		Kind:    "TestPolicy",
	}))
}

// ---- PolicyStore UpsertPolicy / DeletePolicy / Get ----

func TestPolicyStoreUpsertAndGet(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewPolicyStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupExampleIO, Version: "v1", Kind: kindRateLimit}
	store.SetPolicyCRDs([]resources.PolicyCRD{
		{GVK: gvk, Type: topology.PolicyTypeDirect},
	})

	obj := newPolicyUnstructured(nameRateLimitOne, map[string]any{
		keyTargetRef: map[string]any{
			keyGroup: groupGatewayAPI,
			keyKind:  kindGateway,
			keyName:  nameGW1,
		},
	})
	obj.SetGroupVersionKind(gvk)

	store.UpsertPolicy(obj)

	policies := store.Get()
	g.Expect(policies).To(HaveLen(1))
	g.Expect(policies[0].Object.GetName()).To(Equal(nameRateLimitOne))
	g.Expect(policies[0].Type).To(Equal(topology.PolicyTypeDirect))
	g.Expect(policies[0].TargetRefs).To(Equal([]topology.PolicyTargetRef{
		{
			Group:     groupGatewayAPI,
			Kind:      kindGateway,
			Namespace: nsDefault,
			Name:      nameGW1,
		},
	}))
}

func TestPolicyStoreDeletePolicy(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewPolicyStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupExampleIO, Version: "v1", Kind: kindRateLimit}
	store.SetPolicyCRDs([]resources.PolicyCRD{
		{GVK: gvk, Type: topology.PolicyTypeDirect},
	})

	obj := newPolicyUnstructured(nameRateLimitOne, map[string]any{
		keyTargetRef: map[string]any{
			keyGroup: groupGatewayAPI,
			keyKind:  kindGateway,
			keyName:  nameGW1,
		},
	})
	obj.SetGroupVersionKind(gvk)

	store.UpsertPolicy(obj)
	g.Expect(store.Get()).To(HaveLen(1))

	store.DeletePolicy(gvk, types.NamespacedName{Namespace: nsDefault, Name: nameRateLimitOne})
	g.Expect(store.Get()).To(BeEmpty())
}

func TestPolicyStoreIgnoresUnknownGVK(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewPolicyStore(nil, logr.Discard())

	obj := newPolicyUnstructured("unknown-1", map[string]any{})
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "unknown.io", Version: "v1", Kind: "Unknown"})

	store.UpsertPolicy(obj)
	g.Expect(store.Get()).To(BeEmpty())
}

func TestPolicyStoreGetSortsDeterministically(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewPolicyStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupExampleIO, Version: "v1", Kind: kindRateLimit}
	store.SetPolicyCRDs([]resources.PolicyCRD{
		{GVK: gvk, Type: topology.PolicyTypeDirect},
	})

	for _, name := range []string{"c-policy", "a-policy", "b-policy"} {
		obj := newPolicyUnstructured(name, map[string]any{})
		obj.SetGroupVersionKind(gvk)
		store.UpsertPolicy(obj)
	}

	policies := store.Get()
	g.Expect(policies).To(HaveLen(3))
	g.Expect(policies[0].Object.GetName()).To(Equal("a-policy"))
	g.Expect(policies[1].Object.GetName()).To(Equal("b-policy"))
	g.Expect(policies[2].Object.GetName()).To(Equal("c-policy"))
}

// ---- Policy reconciler via framework Reconciler ----

func newPolicyReconciler(
	getter *fakePolicyReader,
	gvk schema.GroupVersionKind,
	onUpsert func(context.Context, client.Object),
	onDelete func(context.Context, client.Object, types.NamespacedName),
) *frameworkcontroller.Reconciler {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	return frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
		Getter:     getter,
		ObjectType: obj,
		OnUpsert:   onUpsert,
		OnDelete:   onDelete,
	})
}

func fakePolicyGetter(
	g Gomega,
	gvk schema.GroupVersionKind,
) *fakePolicyReader {
	return &fakePolicyReader{
		getFunc: func(_ context.Context, key types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			u, ok := obj.(*unstructured.Unstructured)
			g.Expect(ok).To(BeTrue())
			u.SetName(key.Name)
			u.SetNamespace(key.Namespace)
			u.SetGroupVersionKind(gvk)
			u.Object[fieldSpec] = map[string]any{
				keyTargetRef: map[string]any{
					keyGroup: groupGatewayAPI,
					keyKind:  kindGateway,
					keyName:  nameGW1,
				},
			}

			return nil
		},
	}
}

func TestPolicyReconcilerUpsert(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	gvk := schema.GroupVersionKind{Group: groupExampleIO, Version: "v1", Kind: kindRateLimit}
	store := resources.NewPolicyStore(nil, logr.Discard())
	store.SetPolicyCRDs([]resources.PolicyCRD{{GVK: gvk, Type: topology.PolicyTypeDirect}})

	changed := make(chan struct{}, 1)
	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}

	rec := newPolicyReconciler(fakePolicyGetter(g, gvk), gvk,
		func(_ context.Context, obj client.Object) {
			u, ok := obj.(*unstructured.Unstructured)
			g.Expect(ok).To(BeTrue())
			store.UpsertPolicy(u)
			notify()
		},
		func(_ context.Context, _ client.Object, nn types.NamespacedName) {
			store.DeletePolicy(gvk, nn)
			notify()
		},
	)

	ctx := log.IntoContext(t.Context(), logr.Discard())
	result, err := rec.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: nsDefault, Name: nameRateLimitOne},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(reconcile.Result{}))

	policies := store.Get()
	g.Expect(policies).To(HaveLen(1))
	g.Expect(policies[0].Object.GetName()).To(Equal(nameRateLimitOne))
	g.Expect(changed).To(Receive())
}

func TestPolicyReconcilerDelete(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	gvk := schema.GroupVersionKind{Group: groupExampleIO, Version: "v1", Kind: kindRateLimit}
	store := resources.NewPolicyStore(nil, logr.Discard())
	store.SetPolicyCRDs([]resources.PolicyCRD{
		{GVK: gvk, Type: topology.PolicyTypeDirect},
	})

	// Seed the store with a policy.
	seedObj := newPolicyUnstructured(nameRateLimitOne, map[string]any{
		keyTargetRef: map[string]any{
			keyGroup: groupGatewayAPI,
			keyKind:  kindGateway,
			keyName:  nameGW1,
		},
	})
	seedObj.SetGroupVersionKind(gvk)
	store.UpsertPolicy(seedObj)
	g.Expect(store.Get()).To(HaveLen(1))

	changed := make(chan struct{}, 1)

	getter := &fakePolicyReader{
		getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: groupExampleIO, Resource: kindRateLimit}, nameRateLimitOne)
		},
	}

	rec := newPolicyReconciler(getter, gvk,
		func(context.Context, client.Object) {},
		func(_ context.Context, _ client.Object, nn types.NamespacedName) {
			store.DeletePolicy(gvk, nn)

			select {
			case changed <- struct{}{}:
			default:
			}
		},
	)

	ctx := log.IntoContext(t.Context(), logr.Discard())
	result, err := rec.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: nsDefault, Name: nameRateLimitOne},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(reconcile.Result{}))

	g.Expect(store.Get()).To(BeEmpty())
	g.Expect(changed).To(Receive())
}

func TestPolicyReconcilerGetError(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	gvk := schema.GroupVersionKind{Group: groupExampleIO, Version: "v1", Kind: kindRateLimit}

	upsertCalled := false
	deleteCalled := false

	getter := &fakePolicyReader{
		getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
			return errConnectionRefused
		},
	}

	rec := newPolicyReconciler(getter, gvk,
		func(context.Context, client.Object) { upsertCalled = true },
		func(context.Context, client.Object, types.NamespacedName) { deleteCalled = true },
	)

	ctx := log.IntoContext(t.Context(), logr.Discard())
	_, err := rec.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: nsDefault, Name: nameRateLimitOne},
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("connection refused"))
	g.Expect(upsertCalled).To(BeFalse())
	g.Expect(deleteCalled).To(BeFalse())
}

// fakePolicyReader implements client.Reader for policy reconciler tests.
type fakePolicyReader struct {
	getFunc  func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	listFunc func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

func (f *fakePolicyReader) Get(
	ctx context.Context,
	key types.NamespacedName,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if f.getFunc != nil {
		return f.getFunc(ctx, key, obj, opts...)
	}

	return nil
}

func (f *fakePolicyReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if f.listFunc != nil {
		return f.listFunc(ctx, list, opts...)
	}

	return nil
}

// ---- Helpers ----

func newPolicyUnstructured(name string, spec map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			fieldAPIVersion: "example.io/v1",
			keyKind:         "TestPolicy",
			fieldMetadata: map[string]any{
					keyNamespace: nsDefault,
				keyName:     name,
			},
			fieldSpec: spec,
		},
	}

	return obj
}
