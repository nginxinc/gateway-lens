package controller_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	frameworkcontroller "github.com/nginxinc/gateway-lens/internal/k8s/framework/controller"
)

var errConnectionRefused = errors.New("connection refused")

// fakeReader implements client.Reader for testing.
type fakeReader struct {
	getFunc  func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	listFunc func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

func (f *fakeReader) Get(
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

func (f *fakeReader) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if f.listFunc != nil {
		return f.listFunc(ctx, list, opts...)
	}

	return nil
}

const (
	nsDefault = "default"
	podName   = "my-pod"
)

func reconcileRequest() reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: nsDefault,
			Name:      podName,
		},
	}
}

func contextWithLogger(t *testing.T) context.Context {
	t.Helper()

	return log.IntoContext(t.Context(), logr.Discard())
}

func TestReconcileUpsert(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	var upsertedObj client.Object

	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: nsDefault,
			Name:      podName,
		},
	}

	getter := &fakeReader{
		getFunc: func(_ context.Context, key types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
			pod, ok := obj.(*corev1.Pod)
			g.Expect(ok).To(BeTrue(), "expected *corev1.Pod")

			pod.Namespace = key.Namespace
			pod.Name = key.Name

			return nil
		},
	}

	rec, err := frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
		Getter:     getter,
		ObjectType: existingPod,
		OnUpsert: func(_ context.Context, obj client.Object) {
			upsertedObj = obj
		},
		OnDelete: func(context.Context, client.Object, types.NamespacedName) {},
	})
	g.Expect(err).ToNot(HaveOccurred())

	result, err := rec.Reconcile(contextWithLogger(t), reconcileRequest())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(reconcile.Result{}))

	g.Expect(upsertedObj).ToNot(BeNil())
	g.Expect(upsertedObj.GetNamespace()).To(Equal(nsDefault))
	g.Expect(upsertedObj.GetName()).To(Equal(podName))
}

func TestReconcileDeleteNotFound(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	var deletedNN types.NamespacedName

	notFoundErr := apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, podName)

	getter := &fakeReader{
		getFunc: func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
			return notFoundErr
		},
	}

	rec, err := frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
		Getter:     getter,
		ObjectType: &corev1.Pod{},
		OnUpsert:   func(context.Context, client.Object) {},
		OnDelete: func(_ context.Context, _ client.Object, nn types.NamespacedName) {
			deletedNN = nn
		},
	})
	g.Expect(err).ToNot(HaveOccurred())

	result, reconcileErr := rec.Reconcile(contextWithLogger(t), reconcileRequest())
	g.Expect(reconcileErr).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(reconcile.Result{}))

	g.Expect(deletedNN).To(Equal(types.NamespacedName{Namespace: nsDefault, Name: podName}))
}

func TestReconcileGetError(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	upsertCalled := false
	deleteCalled := false

	getter := &fakeReader{
		getFunc: func(context.Context, types.NamespacedName, client.Object, ...client.GetOption) error {
			return errConnectionRefused
		},
	}

	rec, err := frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
		Getter:     getter,
		ObjectType: &corev1.Pod{},
		OnUpsert: func(context.Context, client.Object) {
			upsertCalled = true
		},
		OnDelete: func(context.Context, client.Object, types.NamespacedName) {
			deleteCalled = true
		},
	})
	g.Expect(err).ToNot(HaveOccurred())

	_, reconcileErr := rec.Reconcile(contextWithLogger(t), reconcileRequest())
	g.Expect(reconcileErr).To(HaveOccurred())
	g.Expect(reconcileErr.Error()).To(ContainSubstring("connection refused"))

	g.Expect(upsertCalled).To(BeFalse())
	g.Expect(deleteCalled).To(BeFalse())
}

func TestNewReconcilerRequiresCallbacks(t *testing.T) {
	t.Parallel()

	g := NewWithT(t)

	_, err := frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
		Getter:     &fakeReader{},
		ObjectType: &corev1.Pod{},
		OnDelete:   func(context.Context, client.Object, types.NamespacedName) {},
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("OnUpsert"))

	_, err = frameworkcontroller.NewReconciler(frameworkcontroller.ReconcilerConfig{
		Getter:     &fakeReader{},
		ObjectType: &corev1.Pod{},
		OnUpsert:   func(context.Context, client.Object) {},
	})
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("OnDelete"))
}
