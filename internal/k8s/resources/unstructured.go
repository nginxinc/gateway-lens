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

package resources

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
)

// objectStoreKey uniquely identifies an unstructured object by its GVK and namespaced name.
type objectStoreKey struct {
	// gvk is the stringified GroupVersionKind.
	gvk string
	// nn is the namespaced name.
	nn types.NamespacedName
}

// compareUnstructuredByGVKAndName is a sort comparator that orders unstructured objects
// by GVK string, then namespace, then name.
func compareUnstructuredByGVKAndName(a, b *unstructured.Unstructured) int {
	aGVK := a.GetObjectKind().GroupVersionKind().String()
	bGVK := b.GetObjectKind().GroupVersionKind().String()

	if aGVK != bGVK {
		if aGVK < bGVK {
			return -1
		}

		return 1
	}

	aNS := a.GetNamespace()
	bNS := b.GetNamespace()

	if aNS != bNS {
		if aNS < bNS {
			return -1
		}

		return 1
	}

	aName := a.GetName()
	bName := b.GetName()

	if aName < bName {
		return -1
	}

	if aName > bName {
		return 1
	}

	return 0
}

// resolveGVKsFromCRDs looks up CRD versions for the given group+kind pairs, filtering out
// any group/kinds in the excluded set. It returns only the resolved GVKs.
func resolveGVKsFromCRDs(
	ctx context.Context,
	runtimeCache cache.Cache,
	groupKinds map[string]schema.GroupVersionKind,
	excludedGroupKinds map[string]struct{},
	label string,
) (map[string]schema.GroupVersionKind, error) {
	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := runtimeCache.List(ctx, crdList); err != nil {
		return nil, fmt.Errorf("listing CRDs for %s version resolution: %w", label, err)
	}

	crdVersions := make(map[string]string, len(crdList.Items))

	for i := range crdList.Items {
		crd := &crdList.Items[i]
		key := crd.Spec.Group + "/" + crd.Spec.Names.Kind
		crdVersions[key] = preferredGVK(crd).Version
	}

	resolved := make(map[string]schema.GroupVersionKind)

	for _, gk := range groupKinds {
		groupKindKey := gk.Group + "/" + gk.Kind

		if _, excluded := excludedGroupKinds[groupKindKey]; excluded {
			continue
		}

		version := crdVersions[groupKindKey]
		if version == "" {
			version = "v1" // fallback
		}

		gvk := schema.GroupVersionKind{Group: gk.Group, Version: version, Kind: gk.Kind}
		resolved[gvk.String()] = gvk
	}

	return resolved, nil
}

// preferredGVK returns the GVK using the first served version.
func preferredGVK(crd *apiextensionsv1.CustomResourceDefinition) schema.GroupVersionKind {
	var version string

	for _, v := range crd.Spec.Versions {
		if v.Served {
			version = v.Name

			break
		}
	}

	return schema.GroupVersionKind{
		Group:   crd.Spec.Group,
		Version: version,
		Kind:    crd.Spec.Names.Kind,
	}
}

// syncObjectsFromCache fetches all instances of the given GVKs from the cache
// and stores them in the provided objects map. The logger is used to report
// errors for individual GVKs without aborting the entire sync.
func syncObjectsFromCache(
	ctx context.Context,
	runtimeCache cache.Cache,
	gvks []schema.GroupVersionKind,
	objects map[objectStoreKey]unstructured.Unstructured,
	label string,
	logger logr.Logger,
) {
	for _, gvk := range gvks {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind + "List",
		})

		if err := runtimeCache.List(ctx, list); err != nil {
			logger.Error(err, fmt.Sprintf("Failed to list %s objects", label), "gvk", gvk.String())

			continue
		}

		for i := range list.Items {
			obj := list.Items[i]
			key := objectStoreKey{
				gvk: gvk.String(),
				nn:  types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()},
			}
			objects[key] = *obj.DeepCopy()
		}
	}
}
