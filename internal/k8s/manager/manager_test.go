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

package manager_test

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/k8s/manager"
)

func TestNewGatewayAPIScheme(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	scheme, err := manager.NewScheme()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scheme).ToNot(BeNil())

	// Core v1 types should be registered.
	g.Expect(scheme.IsGroupRegistered(corev1.GroupName)).To(BeTrue(),
		"core v1 group should be registered")

	// Gateway API v1 types should be registered.
	gatewayV1GV := schema.GroupVersion{Group: gatewayv1.GroupVersion.Group, Version: gatewayv1.GroupVersion.Version}
	g.Expect(scheme.IsVersionRegistered(gatewayV1GV)).To(BeTrue(),
		"gateway v1 group version should be registered")
}

func TestNewGatewayAPISchemeKnownTypes(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	scheme, err := manager.NewScheme()
	g.Expect(err).ToNot(HaveOccurred())

	// Spot-check that specific types are registered.
	gvks, _, err := scheme.ObjectKinds(&corev1.Pod{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "Pod should be a known type")

	gvks, _, err = scheme.ObjectKinds(&gatewayv1.Gateway{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "Gateway should be a known type")

	gvks, _, err = scheme.ObjectKinds(&gatewayv1.HTTPRoute{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "HTTPRoute should be a known type")

	gvks, _, err = scheme.ObjectKinds(&gatewayv1.GatewayClass{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "GatewayClass should be a known type")

	gvks, _, err = scheme.ObjectKinds(&gatewayv1.TCPRoute{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "TCPRoute should be a known type")

	gvks, _, err = scheme.ObjectKinds(&gatewayv1.UDPRoute{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "UDPRoute should be a known type")

	gvks, _, err = scheme.ObjectKinds(&gatewayv1.ReferenceGrant{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(gvks).ToNot(BeEmpty(), "ReferenceGrant should be a known type")
}

const testNSDefault = "default"

func TestBuildNamespaceMap(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		namespaces []string
		expected   map[string]cache.Config
	}{
		{
			name:       "single namespace",
			namespaces: []string{testNSDefault},
			expected:   map[string]cache.Config{testNSDefault: {}},
		},
		{
			name:       "multiple namespaces",
			namespaces: []string{testNSDefault, "kube-system"},
			expected: map[string]cache.Config{
				testNSDefault: {},
				"kube-system": {},
			},
		},
		{
			name:       "duplicates are collapsed",
			namespaces: []string{testNSDefault, testNSDefault, "test"},
			expected: map[string]cache.Config{
				testNSDefault: {},
				"test":        {},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := NewWithT(t)
			g.Expect(manager.BuildNamespaceMap(tc.namespaces)).To(Equal(tc.expected))
		})
	}
}
