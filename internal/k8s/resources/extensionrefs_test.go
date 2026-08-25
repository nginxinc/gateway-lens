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

package resources_test

import (
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/k8s/resources"
)

const (
	groupFilters      = "filters.example.io"
	kindRateLimitFilt = "RateLimitFilter"
	kindAuthFilter    = "AuthFilter"
	keyNamespace      = "namespace"
)

// ---- CollectHTTPRouteExtensionRefGVKs ----

func TestCollectHTTPRouteExtensionRefGVKs(t *testing.T) {
	t.Parallel()

	for _, tt := range httpRouteExtRefGVKCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			gvks := make(map[string]schema.GroupVersionKind)
			resources.CollectHTTPRouteExtensionRefGVKs(tt.route, gvks)
			g.Expect(gvks).To(Equal(tt.expected))
		})
	}
}

type httpRouteExtRefGVKCase struct {
	name     string
	route    *gatewayv1.HTTPRoute
	expected map[string]schema.GroupVersionKind
}

func httpRouteExtRefGVKCases() []httpRouteExtRefGVKCase {
	return []httpRouteExtRefGVKCase{
		{
			name:     "no filters",
			route:    newHTTPRouteWithFilters(nil, nil),
			expected: map[string]schema.GroupVersionKind{},
		},
		{
			name: "rule-level extensionRef",
			route: newHTTPRouteWithFilters(
				[]gatewayv1.HTTPRouteFilter{
					newHTTPExtensionRefFilter(kindRateLimitFilt),
				},
				nil,
			),
			expected: map[string]schema.GroupVersionKind{
				extRefGVK(kindRateLimitFilt).String(): extRefGVK(kindRateLimitFilt),
			},
		},
		{
			name: "backend-level extensionRef",
			route: newHTTPRouteWithFilters(
				nil,
				[]gatewayv1.HTTPRouteFilter{
					newHTTPExtensionRefFilter(kindAuthFilter),
				},
			),
			expected: map[string]schema.GroupVersionKind{
				extRefGVK(kindAuthFilter).String(): extRefGVK(kindAuthFilter),
			},
		},
		{
			name: "deduplicates same group+kind",
			route: newHTTPRouteWithFilters(
				[]gatewayv1.HTTPRouteFilter{
					newHTTPExtensionRefFilter(kindRateLimitFilt),
				},
				[]gatewayv1.HTTPRouteFilter{
					newHTTPExtensionRefFilter(kindRateLimitFilt),
				},
			),
			expected: map[string]schema.GroupVersionKind{
				extRefGVK(kindRateLimitFilt).String(): extRefGVK(kindRateLimitFilt),
			},
		},
		{
			name: "ignores non-extensionRef filters",
			route: newHTTPRouteWithFilters(
				[]gatewayv1.HTTPRouteFilter{
					{Type: gatewayv1.HTTPRouteFilterRequestRedirect},
				},
				nil,
			),
			expected: map[string]schema.GroupVersionKind{},
		},
	}
}

// ---- CollectGRPCRouteExtensionRefGVKs ----

func TestCollectGRPCRouteExtensionRefGVKs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		route    *gatewayv1.GRPCRoute
		expected map[string]schema.GroupVersionKind
	}{
		{
			name:     "no filters",
			route:    newGRPCRouteWithFilters(nil, nil),
			expected: map[string]schema.GroupVersionKind{},
		},
		{
			name: "rule-level extensionRef",
			route: newGRPCRouteWithFilters(
				[]gatewayv1.GRPCRouteFilter{
					newGRPCExtensionRefFilter(kindRateLimitFilt),
				},
				nil,
			),
			expected: map[string]schema.GroupVersionKind{
				extRefGVK(kindRateLimitFilt).String(): extRefGVK(kindRateLimitFilt),
			},
		},
		{
			name: "backend-level extensionRef",
			route: newGRPCRouteWithFilters(
				nil,
				[]gatewayv1.GRPCRouteFilter{
					newGRPCExtensionRefFilter(kindAuthFilter),
				},
			),
			expected: map[string]schema.GroupVersionKind{
				extRefGVK(kindAuthFilter).String(): extRefGVK(kindAuthFilter),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			gvks := make(map[string]schema.GroupVersionKind)
			resources.CollectGRPCRouteExtensionRefGVKs(tt.route, gvks)
			g.Expect(gvks).To(Equal(tt.expected))
		})
	}
}

// ---- ExtensionRefStore Upsert / Delete / Get ----

func TestExtensionRefStoreUpsertAndGet(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewExtensionRefStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupFilters, Version: "v1", Kind: kindRateLimitFilt}
	store.SetExtensionRefGVKs([]schema.GroupVersionKind{gvk})

	obj := newExtensionRefUnstructured(gvk, "my-filter", nsDefault)
	store.Upsert(obj)

	objects := store.Get()
	g.Expect(objects).To(HaveLen(1))
	g.Expect(objects[0].GetName()).To(Equal("my-filter"))
	g.Expect(objects[0].GetNamespace()).To(Equal(nsDefault))
	g.Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(gvk))
}

func TestExtensionRefStoreDelete(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewExtensionRefStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupFilters, Version: "v1", Kind: kindRateLimitFilt}
	store.SetExtensionRefGVKs([]schema.GroupVersionKind{gvk})

	obj := newExtensionRefUnstructured(gvk, "my-filter", nsDefault)
	store.Upsert(obj)
	g.Expect(store.Get()).To(HaveLen(1))

	store.Delete(gvk, types.NamespacedName{Namespace: nsDefault, Name: "my-filter"})
	g.Expect(store.Get()).To(BeEmpty())
}

func TestExtensionRefStoreGetSortsDeterministically(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewExtensionRefStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupFilters, Version: "v1", Kind: kindRateLimitFilt}
	store.SetExtensionRefGVKs([]schema.GroupVersionKind{gvk})

	for _, name := range []string{"c-filter", "a-filter", "b-filter"} {
		store.Upsert(newExtensionRefUnstructured(gvk, name, nsDefault))
	}

	objects := store.Get()
	g.Expect(objects).To(HaveLen(3))
	g.Expect(objects[0].GetName()).To(Equal("a-filter"))
	g.Expect(objects[1].GetName()).To(Equal("b-filter"))
	g.Expect(objects[2].GetName()).To(Equal("c-filter"))
}

// ---- IsRouteType ----

func TestIsRouteType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	g.Expect(resources.IsRouteType(&gatewayv1.HTTPRoute{})).To(BeTrue())
	g.Expect(resources.IsRouteType(&gatewayv1.GRPCRoute{})).To(BeTrue())
	g.Expect(resources.IsRouteType(&gatewayv1.Gateway{})).To(BeFalse())
	g.Expect(resources.IsRouteType(&unstructured.Unstructured{})).To(BeFalse())
}

// ---- Helpers ----

func extRefGVK(kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: groupFilters, Version: "v1", Kind: kind}
}

func newHTTPRouteWithFilters(
	ruleFilters []gatewayv1.HTTPRouteFilter,
	backendFilters []gatewayv1.HTTPRouteFilter,
) *gatewayv1.HTTPRoute {
	var backendRefs []gatewayv1.HTTPBackendRef
	if len(backendFilters) > 0 {
		backendRefs = []gatewayv1.HTTPBackendRef{
			{Filters: backendFilters},
		}
	}

	return &gatewayv1.HTTPRoute{
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Filters:     ruleFilters,
					BackendRefs: backendRefs,
				},
			},
		},
	}
}

func newGRPCRouteWithFilters(
	ruleFilters []gatewayv1.GRPCRouteFilter,
	backendFilters []gatewayv1.GRPCRouteFilter,
) *gatewayv1.GRPCRoute {
	var backendRefs []gatewayv1.GRPCBackendRef
	if len(backendFilters) > 0 {
		backendRefs = []gatewayv1.GRPCBackendRef{
			{Filters: backendFilters},
		}
	}

	return &gatewayv1.GRPCRoute{
		Spec: gatewayv1.GRPCRouteSpec{
			Rules: []gatewayv1.GRPCRouteRule{
				{
					Filters:     ruleFilters,
					BackendRefs: backendRefs,
				},
			},
		},
	}
}

func newHTTPExtensionRefFilter(kind string) gatewayv1.HTTPRouteFilter {
	return gatewayv1.HTTPRouteFilter{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: gatewayv1.Group(groupFilters),
			Kind:  gatewayv1.Kind(kind),
			Name:  "some-ref",
		},
	}
}

func newGRPCExtensionRefFilter(kind string) gatewayv1.GRPCRouteFilter {
	return gatewayv1.GRPCRouteFilter{
		Type: gatewayv1.GRPCRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: gatewayv1.Group(groupFilters),
			Kind:  gatewayv1.Kind(kind),
			Name:  "some-ref",
		},
	}
}

func newExtensionRefUnstructured(
	gvk schema.GroupVersionKind,
	name, namespace string,
) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			fieldAPIVersion: gvk.Group + "/" + gvk.Version,
			"kind":          gvk.Kind,
			fieldMetadata: map[string]any{
				keyNamespace: namespace,
				keyName:      name,
			},
			fieldSpec: map[string]any{},
		},
	}

	return obj
}
