package resources

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nginxinc/gateway-lens/internal/topology"
)

// resourceDescriptors returns the full list of Gateway API resource descriptors to watch.
func resourceDescriptors() []resourceDescriptor { //nolint:funlen // descriptor registration is necessarily verbose
	return []resourceDescriptor{
		// Core resources.
		{
			name:   "resources-GatewayClass",
			object: &gatewayv1.GatewayClass{},
			list: newListFunc(
				func() *gatewayv1.GatewayClassList { return &gatewayv1.GatewayClassList{} },
				func(l *gatewayv1.GatewayClassList) []gatewayv1.GatewayClass { return l.Items },
				func(v *gatewayv1.GatewayClass) client.Object { return v.DeepCopy() },
				"gateway classes",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.GatewayClasses = projectValues(
					objects,
					func(item *gatewayv1.GatewayClass) gatewayv1.GatewayClass { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-Gateway",
			object: &gatewayv1.Gateway{},
			list: newListFunc(
				func() *gatewayv1.GatewayList { return &gatewayv1.GatewayList{} },
				func(l *gatewayv1.GatewayList) []gatewayv1.Gateway { return l.Items },
				func(v *gatewayv1.Gateway) client.Object { return v.DeepCopy() },
				"gateways",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.Gateways = projectValues(
					objects,
					func(item *gatewayv1.Gateway) gatewayv1.Gateway { return *item },
					logger,
				)
			},
		},
		// Route resources.
		{
			name:   "resources-HTTPRoute",
			object: &gatewayv1.HTTPRoute{},
			list: newListFunc(
				func() *gatewayv1.HTTPRouteList { return &gatewayv1.HTTPRouteList{} },
				func(l *gatewayv1.HTTPRouteList) []gatewayv1.HTTPRoute { return l.Items },
				func(v *gatewayv1.HTTPRoute) client.Object { return v.DeepCopy() },
				"http routes",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.HTTPRoutes = projectValues(
					objects,
					func(item *gatewayv1.HTTPRoute) gatewayv1.HTTPRoute { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-GRPCRoute",
			object: &gatewayv1.GRPCRoute{},
			list: newListFunc(
				func() *gatewayv1.GRPCRouteList { return &gatewayv1.GRPCRouteList{} },
				func(l *gatewayv1.GRPCRouteList) []gatewayv1.GRPCRoute { return l.Items },
				func(v *gatewayv1.GRPCRoute) client.Object { return v.DeepCopy() },
				"grpc routes",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.GRPCRoutes = projectValues(
					objects,
					func(item *gatewayv1.GRPCRoute) gatewayv1.GRPCRoute { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-TLSRoute",
			object: &gatewayv1.TLSRoute{},
			list: newListFunc(
				func() *gatewayv1.TLSRouteList { return &gatewayv1.TLSRouteList{} },
				func(l *gatewayv1.TLSRouteList) []gatewayv1.TLSRoute { return l.Items },
				func(v *gatewayv1.TLSRoute) client.Object { return v.DeepCopy() },
				"tls routes",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.TLSRoutes = projectValues(
					objects,
					func(item *gatewayv1.TLSRoute) gatewayv1.TLSRoute { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-TCPRoute",
			object: &gatewayv1.TCPRoute{},
			list: newListFunc(
				func() *gatewayv1.TCPRouteList { return &gatewayv1.TCPRouteList{} },
				func(l *gatewayv1.TCPRouteList) []gatewayv1.TCPRoute { return l.Items },
				func(v *gatewayv1.TCPRoute) client.Object { return v.DeepCopy() },
				"tcp routes",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.TCPRoutes = projectValues(
					objects,
					func(item *gatewayv1.TCPRoute) gatewayv1.TCPRoute { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-UDPRoute",
			object: &gatewayv1.UDPRoute{},
			list: newListFunc(
				func() *gatewayv1.UDPRouteList { return &gatewayv1.UDPRouteList{} },
				func(l *gatewayv1.UDPRouteList) []gatewayv1.UDPRoute { return l.Items },
				func(v *gatewayv1.UDPRoute) client.Object { return v.DeepCopy() },
				"udp routes",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.UDPRoutes = projectValues(
					objects,
					func(item *gatewayv1.UDPRoute) gatewayv1.UDPRoute { return *item },
					logger,
				)
			},
		},
		// Policy and extension resources.
		{
			name:   "resources-ReferenceGrant",
			object: &gatewayv1.ReferenceGrant{},
			list: newListFunc(
				func() *gatewayv1.ReferenceGrantList { return &gatewayv1.ReferenceGrantList{} },
				func(l *gatewayv1.ReferenceGrantList) []gatewayv1.ReferenceGrant { return l.Items },
				func(v *gatewayv1.ReferenceGrant) client.Object { return v.DeepCopy() },
				"reference grants",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.ReferenceGrants = projectValues(
					objects,
					func(item *gatewayv1.ReferenceGrant) gatewayv1.ReferenceGrant { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-BackendTLSPolicy",
			object: &gatewayv1.BackendTLSPolicy{},
			list: newListFunc(
				func() *gatewayv1.BackendTLSPolicyList { return &gatewayv1.BackendTLSPolicyList{} },
				func(l *gatewayv1.BackendTLSPolicyList) []gatewayv1.BackendTLSPolicy { return l.Items },
				func(v *gatewayv1.BackendTLSPolicy) client.Object { return v.DeepCopy() },
				"backend tls policies",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.BackendTLSPolicies = projectValues(
					objects,
					func(item *gatewayv1.BackendTLSPolicy) gatewayv1.BackendTLSPolicy { return *item },
					logger,
				)
			},
		},
		{
			name:   "resources-ListenerSet",
			object: &gatewayv1.ListenerSet{},
			list: newListFunc(
				func() *gatewayv1.ListenerSetList { return &gatewayv1.ListenerSetList{} },
				func(l *gatewayv1.ListenerSetList) []gatewayv1.ListenerSet { return l.Items },
				func(v *gatewayv1.ListenerSet) client.Object { return v.DeepCopy() },
				"listener sets",
			),
			projectTo: func(resources *topology.GatewayAPIResources, objects []client.Object, logger logr.Logger) {
				resources.ListenerSets = projectValues(
					objects,
					func(item *gatewayv1.ListenerSet) gatewayv1.ListenerSet { return *item },
					logger,
				)
			},
		},
	}
}

// newListFunc creates a generic list function for a resource type.
// It takes a constructor for the list object, an accessor for its Items slice,
// a toObject function that deep-copies an item to a client.Object, and a label
// for error messages.
func newListFunc[L client.ObjectList, V any](
	newList func() L,
	items func(L) []V,
	toObject func(*V) client.Object,
	label string,
) func(context.Context, client.Reader) ([]client.Object, error) {
	return func(ctx context.Context, reader client.Reader) ([]client.Object, error) {
		list := newList()
		if err := reader.List(ctx, list); err != nil {
			return nil, fmt.Errorf("listing %s: %w", label, err)
		}

		vals := items(list)
		objects := make([]client.Object, 0, len(vals))

		for idx := range vals {
			objects = append(objects, toObject(&vals[idx]))
		}

		return objects, nil
	}
}
