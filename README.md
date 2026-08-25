# Gateway Lens

Gateway Lens is a diagnostic tool that watches Kubernetes
[Gateway API](https://gateway-api.sigs.k8s.io/) resources in a cluster and
presents them as an interactive topology graph in a browser dashboard.

> Requires **Gateway API v1.6+** CRDs to be installed in the cluster.

## Architecture

The Go process uses **controller-runtime** to watch Gateway API resources and
build a topology snapshot of nodes, edges, conditions, and annotations. A
built-in HTTP server exposes this snapshot at `/data` and serves the compiled
React dashboard at all other paths.

The [dashboard frontend](dashboard) is a React + TypeScript application
built with Vite. It receives change notifications via Server-Sent Events and
fetches `/data` on demand, rendering the topology using React Flow with
automatic dagre layout.

## Getting Started

There are two ways to run Gateway Lens: downloading and running the binary
locally against a kubeconfig, or installing it into the cluster as a Pod.

### Option 1: Run Locally

**Prerequisites:**

- A **kubeconfig** pointing at a cluster with Gateway API v1.6+ CRDs installed

Download the latest binary for your platform from the
[Releases page](https://github.com/nginxinc/gateway-lens/releases/latest),
then run it:

```sh
# Linux/macOS example
tar -xzf gateway-lens_<version>_<os>_<arch>.tar.gz
chmod +x gateway-lens
./gateway-lens

# Open the dashboard
open http://localhost:8080
```

### Option 2: Run in Kubernetes

Gateway Lens can run as a Pod inside the cluster it's watching, either via manifests or a Helm chart.

#### Manifests

Provided in [`deploy/manifests.yaml`](deploy/manifests.yaml):

```sh
kubectl apply -f deploy/manifests.yaml
```

If your cluster's Gateway API implementation defines its own extension CRDs
(policies, `parametersRef`/`extensionRef` targets), use the manifest for that
provider instead so RBAC covers those CRDs too, e.g.
[`deploy/manifests-nginx-gateway-fabric.yaml`](deploy/manifests-nginx-gateway-fabric.yaml)
for [NGINX Gateway Fabric](https://github.com/nginx/nginx-gateway-fabric):

```sh
kubectl apply -f deploy/manifests-nginx-gateway-fabric.yaml
```

#### Helm

Provided in [`charts/gateway-lens`](charts/gateway-lens):

```sh
helm install gateway-lens oci://ghcr.io/nginxinc/charts/gateway-lens
```

If your cluster's Gateway API implementation defines its own extension CRDs,
set `rbac.providers` to its name so the chart's `ClusterRole` covers them
too, e.g. for [NGINX Gateway Fabric](https://github.com/nginx/nginx-gateway-fabric):

```sh
helm install gateway-lens oci://ghcr.io/nginxinc/charts/gateway-lens --set rbac.providers={nginx-gateway-fabric}
```

See [`charts/gateway-lens/README.md`](charts/gateway-lens/README.md#providers)
for the full list of available providers.

Either option deploys Gateway Lens into the `default` namespace (or the
current namespace, for Helm), exposed via a `ClusterIP` Service named
`gateway-lens` on port `80`. Port-forward to access the dashboard:

```sh
kubectl port-forward svc/gateway-lens 8080:80
open http://localhost:8080
```

To expose it externally instead, create an `HTTPRoute` attached to a Gateway
in your cluster, pointing at the `gateway-lens` Service.

## Configuration

Gateway Lens is configured via CLI flags, regardless of whether it's run
locally or in Kubernetes:

| Flag           | Description                                             | Default |
| -------------- | -------------------------------------------------------- | ------- |
| `--port`       | Port for the dashboard HTTP server                       | `8080`  |
| `--log-level`  | Log level (`debug`, `info`, `error`, `panic`)             | `info`  |
| `--namespaces` | Comma-separated list of namespaces to watch (default: all) | (all) |

```sh
./gateway-lens --port 9090 --log-level debug --namespaces default,gateway-system
```

When running via the Helm chart, these flags are exposed as the explicit
`port`, `logLevel`, and `namespaces` values (see
[`charts/gateway-lens/README.md`](charts/gateway-lens/README.md#values)). For the plain manifests in
[`deploy/`](deploy/), edit the container's `args`
directly.

## Building From Source

**Prerequisites:**

- **Go 1.26+**
- **Node.js** (LTS) and **npm**
- A **kubeconfig** pointing at a cluster with Gateway API v1.6+ CRDs installed

```sh
# Build the binary (installs dashboard deps, compiles dashboard + Go)
make build

# Run locally (reads kubeconfig from default location)
./bin/gateway-lens

# Open the dashboard
open http://localhost:8080
```

### Running via Container

Gateway Lens can also run as a container. Build the image and run it with
access to a kubeconfig:

```sh
make image
```

## Developer Commands

To view all available `make` targets for development, run `make help`.

### Dashboard Dev Server

For frontend iteration with hot reload:

```sh
cd dashboard
VITE_API_BASE_URL=http://localhost:8080 npm run dev
```

This runs the Vite dev server (default port 5173) proxying API calls to a
running `gateway-lens` process.
