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

package app

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/yaml"

	"github.com/nginxinc/gateway-lens/internal/app/dashboardui"
	"github.com/nginxinc/gateway-lens/internal/topology"
)

// dashboardPayload is the JSON response body served by the /data endpoint.
type dashboardPayload struct {
	// GeneratedAt is the RFC3339 timestamp when the payload was built.
	GeneratedAt string `json:"generatedAt"`
	// Nodes are the topology graph nodes (one per Gateway API resource).
	Nodes []dashboardNodeView `json:"nodes"`
	// Edges are the directional relationships between nodes.
	Edges []dashboardEdgeView `json:"edges"`
	// Annotations are adapter-provided metadata entries attached to nodes.
	Annotations []dashboardAnnotationView `json:"annotations,omitempty"`
}

// dashboardNodeView is the JSON representation of a single topology node.
type dashboardNodeView struct {
	// Ref identifies the Kubernetes resource this node represents.
	Ref dashboardResourceRefView `json:"ref"`
	// Attributes are key-value metadata pairs for the node.
	Attributes map[string]string `json:"attributes,omitempty"`
	// Conditions are the status conditions reported by the resource.
	Conditions []dashboardConditionView `json:"conditions,omitempty"`
	// Manifest is the YAML-encoded resource manifest.
	Manifest string `json:"manifest,omitempty"`
}

// dashboardEdgeView is the JSON representation of a directional graph edge.
type dashboardEdgeView struct {
	// From is the source resource of the edge.
	From dashboardResourceRefView `json:"from"`
	// To is the destination resource of the edge.
	To dashboardResourceRefView `json:"to"`
	// Type categorizes the relationship (e.g. parentRef, backendRef).
	Type topology.EdgeType `json:"type"`
	// Detail is an optional qualifier for the edge type.
	Detail string `json:"detail"`
}

// dashboardAnnotationView is the JSON representation of adapter-provided metadata on a node.
type dashboardAnnotationView struct {
	// Ref identifies the node this annotation belongs to.
	Ref dashboardResourceRefView `json:"ref"`
	// Source is the adapter that produced this annotation.
	Source string `json:"source"`
	// Key is the annotation key.
	Key string `json:"key"`
	// Value is the annotation value.
	Value string `json:"value"`
}

// dashboardConditionView is the JSON representation of a Kubernetes status condition.
type dashboardConditionView struct {
	// Type is the condition type (e.g. "Accepted", "Programmed").
	Type string `json:"type"`
	// Status is the condition status ("True", "False", or "Unknown").
	Status string `json:"status"`
	// Reason is a machine-readable reason for the condition.
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable description of the condition.
	Message string `json:"message,omitempty"`
}

// dashboardResourceRefView is the JSON representation of a Kubernetes resource identity.
type dashboardResourceRefView struct {
	// Group is the API group of the resource.
	Group string `json:"group"`
	// Kind is the resource kind.
	Kind string `json:"kind"`
	// Namespace is the resource namespace; empty for cluster-scoped resources.
	Namespace string `json:"namespace,omitempty"`
	// Name is the resource name.
	Name string `json:"name"`
}

// newDashboardHandler builds the HTTP mux for the /data API and static UI assets.
func newDashboardHandler(reader liveResourcesReader, logger logr.Logger, basePath string) (http.Handler, error) {
	assets, err := dashboardui.FileSystem()
	if err != nil {
		return nil, fmt.Errorf("loading dashboard UI: %w", err)
	}

	indexHTML, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		return nil, fmt.Errorf("reading dashboard index.html: %w", err)
	}

	var fileServer = http.FileServer(http.FS(assets))
	if basePath != "" {
		fileServer = http.StripPrefix(basePath, fileServer)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}

		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(basePath+"/data", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}

		resources := reader.Get()

		payload, err := dashboardPayloadFromResources(resources)
		if err != nil {
			logger.Error(err, "Error building dashboard snapshot")
			http.Error(w, fmt.Sprintf("Error building dashboard snapshot: %v", err), http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(payload); err != nil {
			logger.Error(err, "Error encoding dashboard snapshot")
		}
	})
	mux.HandleFunc(basePath+"/events", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}

		serveSSE(w, r, reader, logger)
	})
	mux.HandleFunc(basePath+"/", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}

		serveDashboardAsset(w, r, assets, indexHTML, fileServer, basePath)
	})

	return mux, nil
}

// requireGET returns true if the request method is GET; otherwise it writes a 405 response.
func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return false
	}

	return true
}

// serveDashboardAsset serves a static file or falls back to index.html for SPA routing.
func serveDashboardAsset(
	w http.ResponseWriter,
	r *http.Request,
	assets fs.FS,
	indexHTML []byte,
	fileServer http.Handler,
	basePath string,
) {
	rootPath := basePath + "/"

	if r.URL.Path == rootPath {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}

	if r.URL.Path != rootPath {
		path := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, basePath), "/")
		if _, err := fs.Stat(assets, path); err == nil {
			fileServer.ServeHTTP(w, r)

			return
		}
	}

	_, _ = w.Write(indexHTML)
}

// serveSSE streams server-sent events to the client whenever the resource store changes.
func serveSSE(w http.ResponseWriter, r *http.Request, reader liveResourcesReader, logger logr.Logger) {
	if !acceptsEventStream(r) {
		http.Error(w, "This endpoint serves an SSE event stream. Use EventSource in JavaScript to connect.",
			http.StatusNotAcceptable)

		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)

		return
	}

	changeCh := reader.Subscribe()
	defer reader.Unsubscribe(changeCh)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher.Flush()

	logger.V(1).Info("SSE client connected")
	defer logger.V(1).Info("SSE client disconnected")

	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-changeCh:
			if !ok {
				return
			}

			_, err := fmt.Fprintf(w, "event: changed\ndata: {}\n\n")
			if err != nil {
				return
			}

			flusher.Flush()
		}
	}
}

// dashboardPayloadFromResources translates typed resources into a dashboard payload via the topology layer.
func dashboardPayloadFromResources(resources topology.GatewayAPIResources) (dashboardPayload, error) {
	baseSnapshot := topology.TranslateGatewayAPI(resources)

	snapshot, err := topology.NewSnapshotBuilder().Build(baseSnapshot, resources)
	if err != nil {
		return dashboardPayload{}, fmt.Errorf("assembling topology snapshot: %w", err)
	}

	return newDashboardPayload(snapshot, resources)
}

// newDashboardPayload converts a topology snapshot into the JSON-serializable dashboard payload.
func newDashboardPayload(
	snapshot topology.Snapshot,
	resources topology.GatewayAPIResources,
) (dashboardPayload, error) {
	manifestByRef, err := buildManifestByRef(resources)
	if err != nil {
		return dashboardPayload{}, err
	}

	nodes := make([]dashboardNodeView, len(snapshot.Nodes))
	for idx, node := range snapshot.Nodes {
		nodes[idx] = dashboardNodeView{
			Ref:        newDashboardResourceRefView(node.Ref),
			Attributes: node.Attributes,
			Conditions: newDashboardConditionViews(node.Conditions),
			Manifest:   manifestByRef[node.Ref],
		}
	}

	edges := make([]dashboardEdgeView, len(snapshot.Edges))
	for idx, edge := range snapshot.Edges {
		edges[idx] = dashboardEdgeView{
			From:   newDashboardResourceRefView(edge.From),
			To:     newDashboardResourceRefView(edge.To),
			Type:   edge.Type,
			Detail: edge.Detail,
		}
	}

	annotations := make([]dashboardAnnotationView, len(snapshot.Annotations))
	for idx, annotation := range snapshot.Annotations {
		annotations[idx] = dashboardAnnotationView{
			Ref:    newDashboardResourceRefView(annotation.Ref),
			Source: annotation.Source,
			Key:    annotation.Key,
			Value:  annotation.Value,
		}
	}

	return dashboardPayload{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Nodes:       nodes,
		Edges:       edges,
		Annotations: annotations,
	}, nil
}

// manifestEntry pairs a resource kind with a function that adds its manifests to a map.
type manifestEntry struct {
	kind string
	add  func(map[topology.ResourceRef]string) error
}

// manifestEntries returns the ordered list of resource types whose manifests should be generated.
func manifestEntries(resources topology.GatewayAPIResources) []manifestEntry {
	v1 := gatewayv1.GroupVersion.String()

	return []manifestEntry{
		{"GatewayClass", addManifestsFunc(v1, "GatewayClass", resources.GatewayClasses)},
		{"Gateway", addManifestsFunc(v1, "Gateway", resources.Gateways)},
		{"HTTPRoute", addManifestsFunc(v1, "HTTPRoute", resources.HTTPRoutes)},
		{"GRPCRoute", addManifestsFunc(v1, "GRPCRoute", resources.GRPCRoutes)},
		{"TLSRoute", addManifestsFunc(v1, "TLSRoute", resources.TLSRoutes)},
		{"TCPRoute", addManifestsFunc(v1, "TCPRoute", resources.TCPRoutes)},
		{"UDPRoute", addManifestsFunc(v1, "UDPRoute", resources.UDPRoutes)},
		{"ReferenceGrant", addManifestsFunc(v1, "ReferenceGrant", resources.ReferenceGrants)},
		{"BackendTLSPolicy", addManifestsFunc(v1, "BackendTLSPolicy", resources.BackendTLSPolicies)},
		{"ListenerSet", addManifestsFunc(v1, "ListenerSet", resources.ListenerSets)},
	}
}

// buildManifestByRef generates a YAML manifest string for every resource, keyed by ResourceRef.
func buildManifestByRef(resources topology.GatewayAPIResources) (map[topology.ResourceRef]string, error) {
	manifestByRef := map[topology.ResourceRef]string{}

	for _, entry := range manifestEntries(resources) {
		if err := entry.add(manifestByRef); err != nil {
			return nil, fmt.Errorf("marshaling %s manifest: %w", entry.kind, err)
		}
	}

	for _, policy := range resources.Policies {
		if err := addUnstructuredManifest(manifestByRef, &policy.Object); err != nil {
			return nil, err
		}
	}

	for _, extRef := range resources.ExtensionRefs {
		if err := addUnstructuredManifest(manifestByRef, &extRef); err != nil {
			return nil, err
		}
	}

	for _, paramRef := range resources.ParametersRefs {
		if err := addUnstructuredManifest(manifestByRef, &paramRef); err != nil {
			return nil, err
		}
	}

	return manifestByRef, nil
}

// addUnstructuredManifest marshals an unstructured object to YAML and adds it to the manifest map.
func addUnstructuredManifest(
	manifests map[topology.ResourceRef]string,
	obj *unstructured.Unstructured,
) error {
	gvk := obj.GetObjectKind().GroupVersionKind()
	ref := topology.ResourceRef{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshaling %s %s/%s: %w", gvk.Kind, ref.Namespace, ref.Name, err)
	}

	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return fmt.Errorf("converting %s %s/%s to YAML: %w", gvk.Kind, ref.Namespace, ref.Name, err)
	}

	manifests[ref] = string(yamlBytes)

	return nil
}

// addManifestsFunc returns a closure that marshals each resource in the slice and adds it to the map.
// The two-type-parameter pattern lets callers pass value slices (e.g. []gatewayv1.Gateway)
// while still satisfying the pointer-receiver client.Object constraint.
func addManifestsFunc[T any, PT interface {
	*T
	client.Object
}](
	apiVersion, kind string,
	resources []T,
) func(map[topology.ResourceRef]string) error {
	return func(manifests map[topology.ResourceRef]string) error {
		for i := range resources {
			item := PT(&resources[i])
			ref := topology.ResourceRef{
				Group:     gatewayv1.GroupName,
				Kind:      kind,
				Namespace: item.GetNamespace(),
				Name:      item.GetName(),
			}

			yamlStr, err := marshalManifestYAML(apiVersion, kind, item)
			if err != nil {
				return fmt.Errorf("marshaling %s/%s: %w", item.GetNamespace(), item.GetName(), err)
			}

			manifests[ref] = yamlStr
		}

		return nil
	}
}

// marshalManifestYAML serializes a resource to YAML, ensuring TypeMeta (apiVersion/kind) is set.
func marshalManifestYAML(apiVersion, kind string, resource client.Object) (string, error) {
	resource.GetObjectKind().SetGroupVersionKind(schema.FromAPIVersionAndKind(apiVersion, kind))

	jsonBytes, err := json.Marshal(resource)
	if err != nil {
		return "", fmt.Errorf("marshaling to JSON: %w", err)
	}

	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return "", fmt.Errorf("converting JSON to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// newDashboardConditionViews converts topology conditions to their JSON view counterparts.
func newDashboardConditionViews(conditions []topology.Condition) []dashboardConditionView {
	views := make([]dashboardConditionView, len(conditions))
	for idx, condition := range conditions {
		views[idx] = dashboardConditionView{
			Type:    condition.Type,
			Status:  condition.Status,
			Reason:  condition.Reason,
			Message: condition.Message,
		}
	}

	return views
}

// newDashboardResourceRefView converts a graph ResourceRef to its JSON view counterpart.
func newDashboardResourceRefView(ref topology.ResourceRef) dashboardResourceRefView {
	return dashboardResourceRefView{
		Group:     ref.Group,
		Kind:      ref.Kind,
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
}

// acceptsEventStream returns true if the request's Accept header is compatible
// with text/event-stream (empty, exact match, or wildcard).
func acceptsEventStream(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	if accept == "" {
		return true
	}

	for mediaType := range strings.SplitSeq(accept, ",") {
		mediaType, _, _ = strings.Cut(strings.TrimSpace(mediaType), ";")
		if mediaType == "text/event-stream" || mediaType == "*/*" || mediaType == "text/*" {
			return true
		}
	}

	return false
}
