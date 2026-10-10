//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	drapb "k8s.io/kubelet/pkg/apis/dra/v1"
)

// Run only in an isolated privileged Linux container, never on a real node.
func TestVethLifecycle(t *testing.T) {
	podNS, err := testutils.NewNS()
	if err != nil {
		t.Fatal(err)
	}
	defer podNS.Close()
	defer testutils.UnmountNS(podNS)
	ctx := context.Background()
	d := &driver{node: "node", count: 32, cancel: func() {}}
	claim := testClaim()
	claims := []*resourceapi.ResourceClaim{claim}
	objects := []kubeletplugin.NamespacedObject{{UID: claim.UID}}
	t.Cleanup(func() { _, _ = d.UnprepareResourceClaims(ctx, objects) })
	prepared, err := d.PrepareResourceClaims(ctx, claims)
	if err != nil || prepared[claim.UID].Err != nil {
		t.Fatalf("prepare: %v %+v", err, prepared)
	}
	sandbox := &drapb.Sandbox{NetnsPath: podNS.Path()}
	for attempt := 0; attempt < 2; attempt++ {
		// Fresh driver instance verifies retries do not depend on process state.
		d = &driver{node: "node", count: 32, cancel: func() {}}
		configured, err := d.ConfigurePodResources(ctx, claims, sandbox)
		if err != nil || configured[claim.UID].Err != nil {
			t.Fatalf("configure attempt %d: %v %+v", attempt, err, configured)
		}
	}
	hostName, _ := linkNames(claim.UID)
	host, err := netlink.NewHandle(unix.NETLINK_ROUTE)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	check := func(handle *netlink.Handle, name, address string) error {
		link, err := handle.LinkByName(name)
		if err != nil {
			return err
		}
		if link.Type() != "veth" || link.Attrs().MTU != 1500 || link.Attrs().Flags&1 == 0 {
			t.Errorf("incorrect link: %+v", link.Attrs())
		}
		addresses, err := handle.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		if len(addresses) != 1 || addresses[0].IPNet.String() != address {
			t.Errorf("addresses on %s: %+v, want %s", name, addresses, address)
		}
		return nil
	}
	if err := check(host, hostName, "192.0.2.1/30"); err != nil {
		t.Fatal(err)
	}
	if err := podNS.Do(func(_ ns.NetNS) error {
		pod, err := netlink.NewHandle(unix.NETLINK_ROUTE)
		if err != nil {
			return err
		}
		defer pod.Close()
		if _, err := pod.LinkByName(hostName); !isLinkNotFound(err) {
			t.Error("host end unexpectedly present in pod namespace")
		}
		return check(pod, "net1", "192.0.2.2/30")
	}); err != nil {
		t.Fatal(err)
	}
	// setns must have restored the calling thread's host namespace.
	if _, err := netlink.LinkByName(hostName); err != nil {
		t.Fatalf("namespace was not restored: %v", err)
	}
	otherNS, err := testutils.NewNS()
	if err != nil {
		t.Fatal(err)
	}
	defer otherNS.Close()
	defer testutils.UnmountNS(otherNS)
	result, err := d.ConfigurePodResources(ctx, claims, &drapb.Sandbox{NetnsPath: otherNS.Path()})
	if err == nil && result[claim.UID].Err == nil {
		t.Fatal("shared live claim should not be moved to a second sandbox")
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := d.UnprepareResourceClaims(ctx, objects)
		if err != nil || result[claim.UID] != nil {
			t.Fatalf("unprepare: %v %+v", err, result)
		}
	}
	if _, err := host.LinkByName(hostName); !isLinkNotFound(err) {
		t.Fatalf("host end still exists: %v", err)
	}
	if err := podNS.Do(func(_ ns.NetNS) error {
		_, err := netlink.LinkByName("net1")
		if !isLinkNotFound(err) {
			t.Errorf("pod end still exists: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Once unprepared, the same claim can be configured in a new sandbox.
	result, err = d.ConfigurePodResources(ctx, claims, &drapb.Sandbox{NetnsPath: otherNS.Path()})
	if err != nil || result[claim.UID].Err != nil {
		t.Fatalf("replacement sandbox: %v %+v", err, result)
	}
}
