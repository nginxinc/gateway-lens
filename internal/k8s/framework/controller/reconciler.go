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

// Package controller provides generic controller-runtime reconcilers.
package controller

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	errObjectTypeMustImplementClientObject = errors.New("object type must implement client.Object")
	errNilOnUpsertCallback                 = errors.New("OnUpsert callback must not be nil")
	errNilOnDeleteCallback                 = errors.New("OnDelete callback must not be nil")
)

// ReconcilerConfig configures a generic resource reconciler.
type ReconcilerConfig struct {
	// Getter reads objects from the cache.
	Getter client.Reader
	// ObjectType is a zero-value exemplar of the watched resource kind.
	ObjectType client.Object
	// OnUpsert is called when a resource is found (created or updated).
	OnUpsert func(ctx context.Context, obj client.Object)
	// OnDelete is called when a resource is not found (deleted).
	OnDelete func(ctx context.Context, objectType client.Object, nn types.NamespacedName)
}

// Reconciler fetches resources and delegates to caller-provided upsert/delete callbacks.
type Reconciler struct {
	// getter reads objects from the cache.
	getter client.Reader
	// objectType is a zero-value exemplar of the watched resource kind.
	objectType client.Object
	// onUpsert is called when a resource is found.
	onUpsert func(ctx context.Context, obj client.Object)
	// onDelete is called when a resource is not found.
	onDelete func(ctx context.Context, objectType client.Object, nn types.NamespacedName)
}

// NewReconciler creates a generic controller-runtime reconciler.
// Returns an error if the OnUpsert or OnDelete callbacks are nil.
func NewReconciler(cfg ReconcilerConfig) (*Reconciler, error) {
	if cfg.OnUpsert == nil {
		return nil, errNilOnUpsertCallback
	}

	if cfg.OnDelete == nil {
		return nil, errNilOnDeleteCallback
	}

	return &Reconciler{
		getter:     cfg.Getter,
		objectType: cfg.ObjectType,
		onUpsert:   cfg.OnUpsert,
		onDelete:   cfg.OnDelete,
	}, nil
}

// Reconcile fetches the resource and calls the appropriate upsert or delete callback.
func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Reconciling the resource")

	object, ok := r.objectType.DeepCopyObject().(client.Object)
	if !ok {
		err := fmt.Errorf("%w: %T", errObjectTypeMustImplementClientObject, r.objectType)

		return reconcile.Result{}, err
	}

	if err := r.getter.Get(ctx, req.NamespacedName, object); err != nil {
		if apierrors.IsNotFound(err) {
			r.onDelete(ctx, r.objectType, req.NamespacedName)
			logger.Info("Deleted the resource")

			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, fmt.Errorf("getting %T %s: %w", object, req.String(), err)
	}

	r.onUpsert(ctx, object)
	logger.Info("Upserted the resource")

	return reconcile.Result{}, nil
}
