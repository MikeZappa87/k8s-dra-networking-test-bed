# Minimal veth DRA driver

Uses the **local Kubernetes fork** on `mzappa/nodeconfigurepoddrapoc`, not a
released kubeletplugin package. The fork must be checked out alongside this
repository as `../kubernetes`. Go 1.27 or newer is required by that checkout.
The module's relative replacements ensure the driver uses the same custom API.

## Lifecycle

- `PrepareResourceClaims`: validates the allocated device and claim's opaque
  configuration; returns its pool/device/request without CDI devices.
- `ConfigurePodResources`: opens `sandbox.GetNetnsPath()`, creates a veth with
  its peer in that namespace, and uses the CNI namespace library's `Do` method
  to **setns on a locked OS thread**, configure the pod end, and restore the
  original namespace. Netlink sockets are opened in their respective namespaces.
- `UnprepareResourceClaims`: deletes the deterministic host interface, which
  also deletes the pod end. Names and ownership aliases survive driver restarts;
  no in-memory prepared-claim cache is needed.

Each node publishes 32 exclusive virtual device slots (`--devices` changes this,
up to 64). One allocated device and one matching claim-sourced opaque config
are required per claim. Config from unrelated drivers/requests or DeviceClasses
is ignored. Claims cannot be shared by multiple live pod sandboxes. Retrying the
same sandbox is idempotent; another live sandbox is rejected rather than having
its interface stolen. When the old sandbox disappears, the kernel removes its
veth pair, allowing the next sandbox to recreate it.

## Build and use

From the repository root, `make up` builds a kind node directly from your
Kubernetes checkout, builds the driver image, creates the cluster, loads the
driver image onto its nodes, and deploys the driver and DeviceClass. The root
node Containerfile is no longer used; kind supplies the unmodified containerd.
`make build` only builds the two images. For an existing cluster, use
`make driver-deploy` to rebuild, load, and restart the driver, including when
reusing its development image tag. Override `NODE_IMAGE`, `DRIVER_IMAGE`,
`CLUSTER_NAME`, or `ARCH` at the root as needed. `make test` runs unit tests,
deploys the driver, creates the example claim/pod, and checks connectivity.

From the repository root, run `make -C driver test`, then
`make -C driver integration-test` to test namespace entry, addresses, retries,
driver restart, live-sandbox conflicts, and cleanup in an isolated privileged
Linux container. Override `TEST_IMAGE` if the default Ubuntu image is unavailable.
These tests do not connect to Kubernetes.

After creating the custom kind cluster, run `make -C driver deploy`,
`make -C driver example`, and `make -C driver check`. Override `CLUSTER_NAME`,
`ARCH`, or `IMAGE` as needed. `make -C driver clean` removes the example pod and
claim. The deployment deliberately uses no surge, because registration sockets
are fixed and only one driver process should run per node.

See [deploy.yaml](deploy.yaml) for RBAC, DaemonSet, and DeviceClass, and
[example.yaml](example.yaml) for a ResourceClaim with opaque parameters and a pod
that consumes it. The example retains the default CNI interface and adds `net1`;
this driver does not depend on default CNI being enabled.

## Opaque parameters

| Field | Meaning |
| --- | --- |
| `interfaceName` | Required pod-side Linux interface name (1–15 characters, not `lo`). |
| `mtu` | Optional MTU; defaults to 1500. IPv6 requires at least 1280. |
| `podAddress` | Optional pod-side unicast CIDR address. |
| `hostAddress` | Optional host-side unicast CIDR address. |

Host names are `vh` plus 12 hex characters derived from the claim UID; the
temporary peer name starts with `vp`. Both ends are brought up. Unknown
parameters are rejected to catch typos. Address configuration uses `AddrReplace`
so retries do not add duplicates. Connected routes are created by the kernel;
the driver installs no default routes, bridges, IP forwarding, or NAT.

**Test-bed only:** the driver runs privileged in the node network namespace.
Claim authors can select host addresses, so restrict who can create these
claims. Use unique address subnets per claim on a node; there is no IPAM or
cross-claim address conflict detection. The example addresses are only for one
demo pod. This is not a production network-isolation mechanism.

The namespace path sent by the runtime must be visible at the **same absolute
path** inside the driver. The DaemonSet mounts `/var/run/netns` with
`HostToContainer` propagation for containerd's usual bind-mounted namespaces.
If your runtime returns paths elsewhere (such as `/proc/<pid>/ns/net`), adjust
the mounts and PID-namespace access accordingly. A host-network workload is
rejected because its namespace is the driver's host namespace.

The veth is managed independently from CNI and does not update `Pod.status.podIPs`
or ResourceClaim device network status. Removing the driver deployment alone
does not clean up active claims; delete consuming pods/claims before uninstalling.

If the driver is forcibly killed in the brief gap between creating a veth and
setting its host ownership alias, a retry deliberately refuses that unmarked
link. Deleting the old sandbox removes the pair; otherwise inspect and remove
the stale link manually. This avoids guessing ownership and deleting unrelated
interfaces.