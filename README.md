# Gateway Lens

Gateway Lens is a diagnostic tool that watches Kubernetes
[Gateway API](https://gateway-api.sigs.k8s.io/) resources in a cluster and
presents them as an interactive topology graph in a browser dashboard.

Check out the [Releases page](https://github.com/sjberman/gateway-lens/releases/latest) for the latest binary artifacts.

## Architecture

The Go process uses **controller-runtime** to watch Gateway API resources and
build a topology snapshot of nodes, edges, conditions, and annotations. A
built-in HTTP server exposes this snapshot at `/data` and serves the compiled
React dashboard at all other paths.

The [dashboard frontend](web/dashboard) is a React + TypeScript application
built with Vite. It receives change notifications via Server-Sent Events and
fetches `/data` on demand, rendering the topology using React Flow with
automatic dagre layout.

## Prerequisites for building and running

- **Go 1.26+**
- **Node.js** (LTS) and **npm**
- A **kubeconfig** pointing at a cluster with Gateway API CRDs installed

## Quick Start

```sh
# Build the binary (installs dashboard deps, compiles dashboard + Go)
make build

# Run locally (reads kubeconfig from default location)
./bin/gateway-lens

# Open the dashboard
open http://localhost:8080
```

Use `--port` to change the listen port and `--log-level` to adjust verbosity
(`debug`, `info`, `error`, `panic`):

```sh
./bin/gateway-lens --port 9090 --log-level debug
```

### Running via Container

Gateway Lens can also run as a container. Build the image and run it with
access to a kubeconfig:

```sh
make image
```

## Developer Commands

| Command                | Description                                           |
| ---------------------- | ----------------------------------------------------- |
| `make build`           | Build dashboard + Go binary into `bin/gateway-lens`   |
| `make unit-test`       | Run Go tests with race detection and coverage         |
| `make lint`            | Run golangci-lint on Go code                          |
| `make dashboard-build` | Compile the React dashboard                           |
| `make dashboard-lint`  | Lint the React dashboard                              |
| `make dashboard-test`  | Run React dashboard unit tests                        |
| `make image`           | Build a container image (requires Docker)             |

### Dashboard Dev Server

For frontend iteration with hot reload:

```sh
cd web/dashboard
VITE_API_BASE_URL=http://localhost:8080 npm run dev
```

This runs the Vite dev server (default port 5173) proxying API calls to a
running `gateway-lens` process.
