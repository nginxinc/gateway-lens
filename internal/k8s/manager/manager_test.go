package manager_test

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/sjberman/gateway-lens/internal/k8s/manager"
)

func TestNewGatewayAPIScheme(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	scheme, err := manager.NewGatewayAPIScheme()
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

	scheme, err := manager.NewGatewayAPIScheme()
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
