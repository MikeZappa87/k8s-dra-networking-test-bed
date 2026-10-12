package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
	drapb "k8s.io/kubelet/pkg/apis/dra/v1"
)

type driver struct {
	node   string
	count  int
	cancel context.CancelFunc
}

var _ kubeletplugin.DRAPlugin = (*driver)(nil)

// PodResourceConfigurer is an optional extension to DRAPlugin. Drivers which
// omit it retain the base DRA contract; the helper reports this RPC as
// Unimplemented, which the kubelet treats as unsupported rather than fatal.
var _ kubeletplugin.PodResourceConfigurer = (*driver)(nil)

// PodResourceUnconfigurer is an optional extension to DRAPlugin. Drivers which
// omit it retain the base DRA contract; the helper reports this RPC as
// Unimplemented, which the kubelet treats as unsupported rather than fatal.
//var _ kubeletplugin.PodResourceUnconfigurer = (*driver)(nil)

func (d *driver) PrepareResourceClaims(ctx context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	results := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))
	for _, claim := range claims {
		_, devices, err := claimConfig(claim, d.node, d.count)
		results[claim.UID] = kubeletplugin.PrepareResult{Devices: devices, Err: err}
	}
	return results, nil
}

func (d *driver) ConfigurePodResources(ctx context.Context, claims []*resourceapi.ResourceClaim, pod *drapb.PodMetadata, sandbox *drapb.Sandbox) (map[types.UID]kubeletplugin.ConfigureResult, error) {
	if sandbox.GetNetnsPath() == "" {
		return nil, fmt.Errorf("sandbox netns_path is required")
	}
	podNS, err := ns.GetNS(sandbox.GetNetnsPath())
	if err != nil {
		return nil, fmt.Errorf("open pod namespace %q: %w", sandbox.GetNetnsPath(), err)
	}
	defer podNS.Close()
	var podStat, hostStat unix.Stat_t
	if err := unix.Fstat(int(podNS.Fd()), &podStat); err != nil {
		return nil, err
	}
	if err := unix.Stat("/proc/self/ns/net", &hostStat); err != nil {
		return nil, err
	}
	if podStat.Ino == hostStat.Ino && podStat.Dev == hostStat.Dev {
		return nil, fmt.Errorf("pod namespace must differ from the host namespace")
	}
	host, err := netlink.NewHandle(unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer host.Close()
	results := make(map[types.UID]kubeletplugin.ConfigureResult, len(claims))
	for _, claim := range claims {
		c, devices, err := claimConfig(claim, d.node, d.count)
		if err == nil {
			if err = ctx.Err(); err == nil {
				alias := fmt.Sprintf("%s%d:%d", ownerPrefix(claim.UID), podStat.Dev, podStat.Ino)
				err = configureVeth(host, podNS, claim.UID, alias, c)
			}
		}
		if err != nil {
			devices = nil
		}
		results[claim.UID] = kubeletplugin.ConfigureResult{Devices: devices, Err: err}
		if err == nil {
			klog.InfoS("Configured pod veth", "claim", klog.KObj(claim), "netns", sandbox.GetNetnsPath(), "interface", c.InterfaceName)
		}
	}
	return results, nil
}

func configureVeth(host *netlink.Handle, podNS ns.NetNS, uid types.UID, alias string, c Config) (err error) {
	hostName, peerName := linkNames(uid)
	link, err := host.LinkByName(hostName)
	created := false
	if isLinkNotFound(err) {
		// Place the peer directly into the target namespace when creating the pair.
		veth := &netlink.Veth{
			LinkAttrs: netlink.LinkAttrs{Name: hostName, MTU: c.MTU},
			PeerName:  peerName, PeerNamespace: netlink.NsFd(podNS.Fd()),
		}
		if err = host.LinkAdd(veth); err != nil {
			return fmt.Errorf("create veth pair: %w", err)
		}
		created = true
		link, err = host.LinkByName(hostName)
	}
	if err != nil {
		return err
	}
	defer func() {
		if err != nil && created {
			err = errors.Join(err, host.LinkDel(link))
		}
	}()
	if created {
		// Some kernels do not retain IFLA_IFALIAS during veth creation.
		// Set it explicitly before doing further work on the pair.
		if err = host.LinkSetAlias(link, alias); err != nil {
			return fmt.Errorf("set host ownership alias: %w", err)
		}
		link.Attrs().Alias = alias
	}
	if link.Type() != "veth" || link.Attrs().Alias != alias {
		return fmt.Errorf("host link %s belongs to another claim or sandbox (type %q, alias %q, expected %q); refusing to replace it", hostName, link.Type(), link.Attrs().Alias, alias)
	}
	if err = configureLink(host, link, c.MTU, c.HostAddress); err != nil {
		return fmt.Errorf("configure host veth: %w", err)
	}
	// Do locks an OS thread, calls setns, and restores the original namespace.
	// Open the netlink socket AFTER setns, not in the host namespace.
	err = podNS.Do(func(_ ns.NetNS) error {
		pod, err := netlink.NewHandle(unix.NETLINK_ROUTE)
		if err != nil {
			return err
		}
		defer pod.Close()
		peer, err := pod.LinkByName(peerName)
		if isLinkNotFound(err) {
			peer, err = pod.LinkByName(c.InterfaceName)
			if err == nil && (peer.Type() != "veth" || peer.Attrs().Alias != alias) {
				return fmt.Errorf("pod interface %s is not owned by this claim", c.InterfaceName)
			}
		} else if err == nil {
			if c.InterfaceName != peerName {
				if _, lookupErr := pod.LinkByName(c.InterfaceName); !isLinkNotFound(lookupErr) {
					return fmt.Errorf("pod interface %s already exists or cannot be inspected", c.InterfaceName)
				}
			}
			// Alias before rename makes a crash between these steps retryable.
			if err = pod.LinkSetAlias(peer, alias); err != nil {
				return err
			}
			if c.InterfaceName != peerName {
				if err = pod.LinkSetName(peer, c.InterfaceName); err != nil {
					return err
				}
			}
		}
		if err != nil {
			return fmt.Errorf("find pod peer: %w", err)
		}
		return configureLink(pod, peer, c.MTU, c.PodAddress)
	})
	if err != nil {
		return fmt.Errorf("configure pod veth: %w", err)
	}
	return nil
}

func configureLink(handle *netlink.Handle, link netlink.Link, mtu int, address string) error {
	if err := handle.LinkSetMTU(link, mtu); err != nil {
		return err
	}
	if address != "" {
		addr, err := netlink.ParseAddr(address)
		if err != nil {
			return err
		}
		if err := handle.AddrReplace(link, addr); err != nil {
			return err
		}
	}
	return handle.LinkSetUp(link)
}

func isLinkNotFound(err error) bool {
	var notFound netlink.LinkNotFoundError
	return errors.As(err, &notFound)
}

func (d *driver) UnconfigurePodResources(ctx context.Context, claims []kubeletplugin.NamespacedObject, sandbox *drapb.Sandbox) (result map[types.UID]error, err error) {
	return d.UnprepareResourceClaims(ctx, claims)
}

func (d *driver) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	host, err := netlink.NewHandle(unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	defer host.Close()
	results := make(map[types.UID]error, len(claims))
	for _, claim := range claims {
		hostName, _ := linkNames(claim.UID)
		link, err := host.LinkByName(hostName)
		if isLinkNotFound(err) {
			err = nil
		} else if err == nil {
			if link.Type() != "veth" || !strings.HasPrefix(link.Attrs().Alias, ownerPrefix(claim.UID)) {
				err = fmt.Errorf("refusing to delete unowned host link %s", hostName)
			} else {
				// Deleting either end removes the pair, including the pod peer.
				err = host.LinkDel(link)
			}
		}
		results[claim.UID] = err
	}
	return results, nil
}

func (d *driver) HandleError(ctx context.Context, err error, message string) {
	klog.FromContext(ctx).Error(err, message)
	if !errors.Is(err, kubeletplugin.ErrRecoverable) {
		d.cancel()
	}
}

func (d *driver) WatchHealthStatus(context.Context, chan<- kubeletplugin.DeviceHealthReport) error {
	return kubeletplugin.ErrHealthNotSupported
}
