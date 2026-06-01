package manager

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// NewGatewayAPIScheme wraps newGatewayAPIScheme for testing.
func NewGatewayAPIScheme() (*runtime.Scheme, error) {
	return newGatewayAPIScheme()
}

// BuildNamespaceMap wraps buildNamespaceMap for testing.
func BuildNamespaceMap(namespaces []string) map[string]cache.Config {
	return buildNamespaceMap(namespaces)
}
