package resources_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/sjberman/gateway-lens/internal/k8s/resources"
	"github.com/sjberman/gateway-lens/internal/topology"
)

var errForbidden = errors.New("forbidden")

// fakeListReader implements client.Reader for testing NewListFunc.
type fakeListReader struct {
	listFunc func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

func (f *fakeListReader) Get(
	_ context.Context,
	_ client.ObjectKey,
	_ client.Object,
	_ ...client.GetOption,
) error {
	return nil
}

func (f *fakeListReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listFunc != nil {
		return f.listFunc(ctx, list, opts...)
	}

	return nil
}

func TestNewListFuncSuccess(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	listFn := resources.NewListFunc(
		func() *gatewayv1.GatewayList { return &gatewayv1.GatewayList{} },
		func(l *gatewayv1.GatewayList) []gatewayv1.Gateway { return l.Items },
		func(v *gatewayv1.Gateway) client.Object { return v.DeepCopy() },
		"gateways",
	)

	reader := &fakeListReader{
		listFunc: func(_ context.Context, list client.ObjectList, _ ...client.ListOption) error {
			gwList, ok := list.(*gatewayv1.GatewayList)
			g.Expect(ok).To(BeTrue())

			gwList.Items = []gatewayv1.Gateway{
				{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: nameGW1}},
				{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: nameGW2}},
			}

			return nil
		},
	}

	objects, err := listFn(t.Context(), reader)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(objects).To(HaveLen(2))
	g.Expect(objects[0].GetName()).To(Equal(nameGW1))
	g.Expect(objects[1].GetName()).To(Equal(nameGW2))
}

func TestNewListFuncError(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	listFn := resources.NewListFunc(
		func() *gatewayv1.GatewayList { return &gatewayv1.GatewayList{} },
		func(l *gatewayv1.GatewayList) []gatewayv1.Gateway { return l.Items },
		func(v *gatewayv1.Gateway) client.Object { return v.DeepCopy() },
		"gateways",
	)

	reader := &fakeListReader{
		listFunc: func(context.Context, client.ObjectList, ...client.ListOption) error {
			return errForbidden
		},
	}

	_, err := listFn(t.Context(), reader)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("listing gateways"))
	g.Expect(err.Error()).To(ContainSubstring("forbidden"))
}

func TestNewListFuncEmpty(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	listFn := resources.NewListFunc(
		func() *gatewayv1.GatewayList { return &gatewayv1.GatewayList{} },
		func(l *gatewayv1.GatewayList) []gatewayv1.Gateway { return l.Items },
		func(v *gatewayv1.Gateway) client.Object { return v.DeepCopy() },
		"gateways",
	)

	reader := &fakeListReader{} // No listFunc → returns nil (no items).

	objects, err := listFn(t.Context(), reader)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(objects).To(BeEmpty())
}

func TestDescriptorObjectTypes(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	descriptors := resources.ResourceDescriptors()

	expectedTypes := []client.Object{
		&gatewayv1.GatewayClass{},
		&gatewayv1.Gateway{},
		&gatewayv1.HTTPRoute{},
		&gatewayv1.GRPCRoute{},
		&gatewayv1.TLSRoute{},
		&gatewayv1alpha2.TCPRoute{},
		&gatewayv1alpha2.UDPRoute{},
		&gatewayv1.ReferenceGrant{},
		&gatewayv1.BackendTLSPolicy{},
		&gatewayv1.ListenerSet{},
	}

	for i, descriptor := range descriptors {
		g.Expect(descriptor.ObjectType()).To(Equal(reflect.TypeOf(expectedTypes[i]).String()),
			"descriptor %d type mismatch", i)
	}
}

func TestDescriptorProjectTo(t *testing.T) { //nolint:funlen // table-driven test with many resource kinds
	t.Parallel()

	g := NewWithT(t)

	descriptors := resources.ResourceDescriptors()
	logger := logr.Discard()

	testCases := []struct {
		name     string
		idx      int
		objects  []client.Object
		validate func(*topology.GatewayAPIResources)
	}{
		{
			name: "GatewayClass",
			idx:  0,
			objects: []client.Object{
				&gatewayv1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "my-class"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.GatewayClasses).To(HaveLen(1))
				g.Expect(r.GatewayClasses[0].Name).To(Equal("my-class"))
			},
		},
		{
			name: "Gateway",
			idx:  1,
			objects: []client.Object{
				&gatewayv1.Gateway{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: nameGW1}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.Gateways).To(HaveLen(1))
				g.Expect(r.Gateways[0].Name).To(Equal(nameGW1))
			},
		},
		{
			name: "HTTPRoute",
			idx:  2,
			objects: []client.Object{
				&gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "route-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.HTTPRoutes).To(HaveLen(1))
				g.Expect(r.HTTPRoutes[0].Name).To(Equal("route-1"))
			},
		},
		{
			name: "GRPCRoute",
			idx:  3,
			objects: []client.Object{
				&gatewayv1.GRPCRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "grpc-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.GRPCRoutes).To(HaveLen(1))
				g.Expect(r.GRPCRoutes[0].Name).To(Equal("grpc-1"))
			},
		},
		{
			name: "TLSRoute",
			idx:  4,
			objects: []client.Object{
				&gatewayv1.TLSRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "tls-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.TLSRoutes).To(HaveLen(1))
				g.Expect(r.TLSRoutes[0].Name).To(Equal("tls-1"))
			},
		},
		{
			name: "TCPRoute",
			idx:  5,
			objects: []client.Object{
				&gatewayv1alpha2.TCPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "tcp-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.TCPRoutes).To(HaveLen(1))
				g.Expect(r.TCPRoutes[0].Name).To(Equal("tcp-1"))
			},
		},
		{
			name: "UDPRoute",
			idx:  6,
			objects: []client.Object{
				&gatewayv1alpha2.UDPRoute{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "udp-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.UDPRoutes).To(HaveLen(1))
				g.Expect(r.UDPRoutes[0].Name).To(Equal("udp-1"))
			},
		},
		{
			name: "ReferenceGrant",
			idx:  7,
			objects: []client.Object{
				&gatewayv1.ReferenceGrant{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "rg-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.ReferenceGrants).To(HaveLen(1))
				g.Expect(r.ReferenceGrants[0].Name).To(Equal("rg-1"))
			},
		},
		{
			name: "BackendTLSPolicy",
			idx:  8,
			objects: []client.Object{
				&gatewayv1.BackendTLSPolicy{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "btp-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.BackendTLSPolicies).To(HaveLen(1))
				g.Expect(r.BackendTLSPolicies[0].Name).To(Equal("btp-1"))
			},
		},
		{
			name: "ListenerSet",
			idx:  9,
			objects: []client.Object{
				&gatewayv1.ListenerSet{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "ls-1"}},
			},
			validate: func(r *topology.GatewayAPIResources) {
				g.Expect(r.ListenerSets).To(HaveLen(1))
				g.Expect(r.ListenerSets[0].Name).To(Equal("ls-1"))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res := topology.GatewayAPIResources{}
			descriptors[tc.idx].ProjectTo(&res, tc.objects, logger)
			tc.validate(&res)
		})
	}
}
