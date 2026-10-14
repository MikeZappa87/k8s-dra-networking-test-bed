# Kubernetes DRA networking test bed

A local kind cluster built from a custom Kubernetes branch, with a minimal
Dynamic Resource Allocation (DRA) driver that adds a veth interface to a pod's
network namespace. One end stays in the kind node's host network namespace.

The driver uses the fork's `ConfigurePodResources` callback and the sandbox's
`netns_path`. This RPC is not available in released kubeletplugin packages, so
the driver and node must be built against the compatible Kubernetes fork.

## Prerequisites

- Docker with a running Linux container engine (Docker Desktop on macOS).
- Go 1.27 or newer, as required by the Kubernetes checkout.
- kind, kubectl, Git, and Make.
- Enough disk space and memory to build Kubernetes from source.

The default source repository is
[MikeZappa87/kubernetes](https://github.com/MikeZappa87/kubernetes), branch
`mzappa/nonnetworkpodsmain-nodeconfigurepoddrapoc`, checked out alongside this repository as
`../kubernetes`. `make source` clones it if missing; an existing checkout is
**never pulled, reset, or switched to another branch**. Ensure an existing
checkout contains the intended changes before building.

The driver's [Go module](driver/go.mod) uses relative replacements pointing to
that sibling checkout. Overriding `KUBERNETES_DIR` changes the node build source,
but does not change those driver replacements; update them if using another
source location.

## Quick start

Run `make up` from the repository root. It:

1. Builds a kind base node image from Kubernetes, then overlays custom containerd.
2. Compiles the Linux driver and builds its container image.
3. Creates the `dra-poc` cluster with one control-plane node and one worker.
4. Loads the driver image into the cluster's nodes.
5. Installs RBAC, the driver DaemonSet, and the `veth` DeviceClass.

Next, run `make example` to create the ResourceClaim and demo pod, followed by
`make check` to inspect `net1` and ping the host-side address. `make status`
shows nodes and cluster workloads.

The example retains kind's default CNI interface and adds `net1`, with
`192.0.2.2/30` on the pod end and `192.0.2.1/30` on the host end. Its interface
name, MTU, and addresses are defined in the ResourceClaim's opaque parameters.
See [driver/example.yaml](driver/example.yaml).

`make up` expects the cluster name to be unused. To rebuild the cluster, run
`make delete` first. To update only the driver in an existing cluster, run
`make driver-deploy`; this also restarts the DaemonSet when reusing the same
development image tag.

## Make targets

| Target | Purpose |
| --- | --- |
| `make help` | List targets and supported overrides. |
| `make source` | Clone Kubernetes if the checkout is missing. |
| `make build` | Build both images without creating a cluster. |
| `make kubernetes-image` | Build the kind base node image from Kubernetes source. |
| `make node-image` | Build containerd and overlay it onto the existing base node image. |
| `make cluster` | Create a cluster using the existing node image. |
| `make up` | Build images, create the cluster, and deploy the driver. |
| `make driver-build` | Compile the Linux driver binary. |
| `make driver-image` | Compile and build the driver container image. |
| `make driver-load` | Build and load the driver image into an existing cluster. |
| `make driver-deploy` | Build, load, install, and restart the driver. |
| `make driver-test` | Run driver configuration and manifest tests. |
| `make driver-integration-test` | Run veth lifecycle tests in an isolated privileged Linux container. |
| `make example` | Create the demo claim and pod and wait for readiness. |
| `make check` | Inspect the demo interface and test pod-to-host connectivity. |
| `make test` | Run unit tests, deploy the driver, create the demo, and check connectivity. |
| `make status` | Show nodes and workloads. |
| `make delete` | Delete the kind cluster, retaining images and sources. |

`make test` requires an existing compatible cluster. Unit and isolated Linux
integration tests do not require Kubernetes. The integration tests use Docker
with `--privileged --network none`; they create namespaces only inside the test
container, not in the kind cluster.

To remove only the example pod and claim, run `make -C driver clean`.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `KUBERNETES_DIR` | Sibling `../kubernetes` checkout | Kubernetes source used for the node build. |
| `KUBERNETES_REPO` | `https://github.com/MikeZappa87/kubernetes.git` | Repository cloned when sources are missing. |
| `KUBERNETES_BRANCH` | `mzappa/nonnetworkpodsmain-nodeconfigurepoddrapoc` | Branch used for a new clone. |
| `KUBERNETES_IMAGE` | `dra-poc-kubernetes:dev` | Kubernetes base image built by kind. |
| `NODE_IMAGE` | `dra-poc-node:dev` | Custom kind node image. |
| `CONTAINERD_REPO` | `https://github.com/MikeZappa87/containerd.git` | Runtime fork cloned inside the build container. |
| `CONTAINERD_BRANCH` | `mzappa/nonnetworkpods` | Runtime branch to build. |
| `CONTAINERD_COMMIT` | Empty | Optional commit to pin instead of building the branch tip. |
| `GO_VERSION` | `1.27.1` | Go builder image version for containerd. |
| `BUILD_FLAGS` | Empty | Extra Docker build flags for the runtime overlay. |
| `DRIVER_IMAGE` | `veth-dra-driver:dev` | Driver image built and loaded into kind. |
| `CLUSTER_NAME` | `dra-poc` | Cluster name; kubectl uses context `kind-<name>`. |
| `ARCH` | Host architecture, normalized to `arm64` or `amd64` | Target Linux image and binary architecture. |
| `WAIT` | `180s` | Cluster creation and workload readiness timeout. |

For example, `make up CLUSTER_NAME=dra-dev DRIVER_IMAGE=veth-dra-driver:local`
uses another cluster name and driver tag. Use the same overrides for subsequent
commands. `GO`, `DOCKER`, `KIND`, and `KUBECTL` can override tool paths.

The node build uses [kubernetes-version.env](kubernetes-version.env) through
`KUBE_GIT_VERSION_FILE`. Keep that development-version metadata consistent with
the source you intend to build, or supply another file via `VERSION_FILE`.

## Project layout

- [Makefile](Makefile): source builds, cluster lifecycle, and driver orchestration.
- [kind.yaml](kind.yaml): two-node cluster with default CNI and PodDefaultNetwork enabled.
- [Containerfile](Containerfile): builds custom containerd and overlays its binaries.
- [kubernetes-version.env](kubernetes-version.env): Kubernetes build version metadata.
- [driver/README.md](driver/README.md): driver lifecycle, opaque configuration,
  runtime namespace requirements, and limitations.
- [driver/deploy.yaml](driver/deploy.yaml): RBAC, DaemonSet, and DeviceClass.
- [driver/example.yaml](driver/example.yaml): claim configuration and consuming pod.
- [driver/Containerfile](driver/Containerfile): packages the compiled driver.

## Custom containerd

kind builds `KUBERNETES_IMAGE` from your Kubernetes checkout. The root
[Containerfile](Containerfile) compiles the containerd fork inside Linux for
`ARCH`, then replaces `containerd`, `ctr`, and `containerd-shim-runc-v2` in the
final `NODE_IMAGE`. Kubernetes binaries, kind's entrypoint, runtime configuration,
CNI, and preloaded control-plane images are inherited from the base image.
The image names must differ. No local containerd checkout is required.

`make build` orders these stages correctly; `make up` then creates the cluster
from the final image. After changing only containerd, use `make node-image` to
reuse the existing Kubernetes base instead of rebuilding Kubernetes. Supply
`CONTAINERD_BRANCH` for another branch or `CONTAINERD_COMMIT` for reproducibility.
Docker may cache a branch clone even after its remote tip moves; use
`make node-image BUILD_FLAGS=--no-cache` to refresh it. Changing a pinned commit
invalidates the source build layer automatically.

Replacing the image does not update existing nodes. Recreate the kind cluster
to use the new runtime. The configured Go version must meet the selected
containerd branch's requirements, and its runtime configuration must remain
compatible with kind's base image.

## Scope and safety

This is a **privileged development proof of concept**, not a production network
driver. The driver uses `hostNetwork`, enters pod namespaces with `setns`, and
can assign claim-specified addresses in the node namespace. Restrict who can
create claims for this driver.

Each node publishes 32 exclusive veth slots by default. The driver supports one
veth per claim, without IPAM, bridges, NAT, or default-route installation. Use
unique address subnets for separate claims on the same node. It does not update
pod IP status; the example's secondary address is independent of the default
CNI network. Further details are in [driver/README.md](driver/README.md).