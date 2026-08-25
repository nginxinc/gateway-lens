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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/k8s/resources"
)

const (
	groupConfig      = "config.example.io"
	kindGWClassConf  = "GatewayClassConfig"
	kindGWConfig     = "GatewayConfig"
	groupInfra       = "infra.example.io"
	kindInfraConfig  = "InfraConfig"
)

// ---- CollectGatewayClassParametersRefGVKs ----

func TestCollectGatewayClassParametersRefGVKs(t *testing.T) {
	t.Parallel()

	for _, tt := range gatewayClassParamsRefGVKCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			gvks := make(map[string]schema.GroupVersionKind)
			resources.CollectGatewayClassParametersRefGVKs(tt.gatewayClass, gvks)
			g.Expect(gvks).To(Equal(tt.expected))
		})
	}
}

type gatewayClassParamsRefGVKCase struct {
	name         string
	gatewayClass *gatewayv1.GatewayClass
	expected     map[string]schema.GroupVersionKind
}

func gatewayClassParamsRefGVKCases() []gatewayClassParamsRefGVKCase {
	ns := gatewayv1.Namespace(nsDefault)

	return []gatewayClassParamsRefGVKCase{
		{
			name:         "no parametersRef",
			gatewayClass: &gatewayv1.GatewayClass{},
			expected:     map[string]schema.GroupVersionKind{},
		},
		{
			name: "namespace-scoped parametersRef",
			gatewayClass: &gatewayv1.GatewayClass{
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: "example.com/controller",
					ParametersRef: &gatewayv1.ParametersReference{
						Group:     gatewayv1.Group(groupConfig),
						Kind:      gatewayv1.Kind(kindGWClassConf),
						Name:      "my-config",
						Namespace: &ns,
					},
				},
			},
			expected: map[string]schema.GroupVersionKind{
				paramsRefGVK(groupConfig, kindGWClassConf).String(): paramsRefGVK(groupConfig, kindGWClassConf),
			},
		},
		{
			name: "cluster-scoped parametersRef",
			gatewayClass: &gatewayv1.GatewayClass{
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: "example.com/controller",
					ParametersRef: &gatewayv1.ParametersReference{
						Group: gatewayv1.Group(groupConfig),
						Kind:  gatewayv1.Kind(kindGWClassConf),
						Name:  "cluster-config",
					},
				},
			},
			expected: map[string]schema.GroupVersionKind{
				paramsRefGVK(groupConfig, kindGWClassConf).String(): paramsRefGVK(groupConfig, kindGWClassConf),
			},
		},
	}
}

// ---- CollectGatewayParametersRefGVKs ----

func TestCollectGatewayParametersRefGVKs(t *testing.T) {
	t.Parallel()

	for _, tt := range gatewayParamsRefGVKCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			gvks := make(map[string]schema.GroupVersionKind)
			resources.CollectGatewayParametersRefGVKs(tt.gateway, gvks)
			g.Expect(gvks).To(Equal(tt.expected))
		})
	}
}

type gatewayParamsRefGVKCase struct {
	name     string
	gateway  *gatewayv1.Gateway
	expected map[string]schema.GroupVersionKind
}

func gatewayParamsRefGVKCases() []gatewayParamsRefGVKCase {
	return []gatewayParamsRefGVKCase{
		{
			name:     "no infrastructure",
			gateway:  &gatewayv1.Gateway{},
			expected: map[string]schema.GroupVersionKind{},
		},
		{
			name: "infrastructure without parametersRef",
			gateway: &gatewayv1.Gateway{
				Spec: gatewayv1.GatewaySpec{
					Infrastructure: &gatewayv1.GatewayInfrastructure{},
				},
			},
			expected: map[string]schema.GroupVersionKind{},
		},
		{
			name: "infrastructure with parametersRef",
			gateway: &gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Namespace: nsDefault, Name: "my-gw"},
				Spec: gatewayv1.GatewaySpec{
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: gatewayv1.Group(groupInfra),
							Kind:  gatewayv1.Kind(kindInfraConfig),
							Name:  "my-infra-config",
						},
					},
				},
			},
			expected: map[string]schema.GroupVersionKind{
				paramsRefGVK(groupInfra, kindInfraConfig).String(): paramsRefGVK(groupInfra, kindInfraConfig),
			},
		},
	}
}

// ---- ParametersRefStore Upsert / Delete / Get ----

func TestParametersRefStoreUpsertAndGet(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewParametersRefStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupConfig, Version: "v1", Kind: kindGWClassConf}
	store.SetParametersRefGVKs([]schema.GroupVersionKind{gvk})

	obj := newParametersRefUnstructured(gvk, "my-config", nsDefault)
	store.Upsert(obj)

	objects := store.Get()
	g.Expect(objects).To(HaveLen(1))
	g.Expect(objects[0].GetName()).To(Equal("my-config"))
	g.Expect(objects[0].GetNamespace()).To(Equal(nsDefault))
	g.Expect(objects[0].GetObjectKind().GroupVersionKind()).To(Equal(gvk))
}

func TestParametersRefStoreDelete(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewParametersRefStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupConfig, Version: "v1", Kind: kindGWClassConf}
	store.SetParametersRefGVKs([]schema.GroupVersionKind{gvk})

	obj := newParametersRefUnstructured(gvk, "my-config", nsDefault)
	store.Upsert(obj)
	g.Expect(store.Get()).To(HaveLen(1))

	store.Delete(gvk, types.NamespacedName{Namespace: nsDefault, Name: "my-config"})
	g.Expect(store.Get()).To(BeEmpty())
}

func TestParametersRefStoreGetSortsDeterministically(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	store := resources.NewParametersRefStore(nil, logr.Discard())

	gvk := schema.GroupVersionKind{Group: groupConfig, Version: "v1", Kind: kindGWClassConf}
	store.SetParametersRefGVKs([]schema.GroupVersionKind{gvk})

	for _, name := range []string{"c-config", "a-config", "b-config"} {
		store.Upsert(newParametersRefUnstructured(gvk, name, nsDefault))
	}

	objects := store.Get()
	g.Expect(objects).To(HaveLen(3))
	g.Expect(objects[0].GetName()).To(Equal("a-config"))
	g.Expect(objects[1].GetName()).To(Equal("b-config"))
	g.Expect(objects[2].GetName()).To(Equal("c-config"))
}

// ---- IsGatewayClassOrGatewayType ----

func TestIsGatewayClassOrGatewayType(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	g.Expect(resources.IsGatewayClassOrGatewayType(&gatewayv1.GatewayClass{})).To(BeTrue())
	g.Expect(resources.IsGatewayClassOrGatewayType(&gatewayv1.Gateway{})).To(BeTrue())
	g.Expect(resources.IsGatewayClassOrGatewayType(&gatewayv1.HTTPRoute{})).To(BeFalse())
	g.Expect(resources.IsGatewayClassOrGatewayType(&unstructured.Unstructured{})).To(BeFalse())
}

// ---- Helpers ----

func paramsRefGVK(group, kind string) schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: group, Version: "v1", Kind: kind}
}

func newParametersRefUnstructured(
	gvk schema.GroupVersionKind,
	name, namespace string,
) *unstructured.Unstructured {
	return &unstructured.Unstructured{
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
}
