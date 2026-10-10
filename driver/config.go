package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

const driverName = "veth.dra.example.com"

// Config is supplied in ResourceClaim.spec.devices.config[].opaque.parameters.
// Addresses are optional; this driver never installs routes, NAT, or a bridge.
type Config struct {
	InterfaceName string `json:"interfaceName"`
	MTU           int    `json:"mtu,omitempty"`
	PodAddress    string `json:"podAddress,omitempty"`
	HostAddress   string `json:"hostAddress,omitempty"`
}

var interfacePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)

func decodeConfig(raw []byte) (Config, error) {
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("decode opaque parameters: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("opaque parameters must contain one JSON object")
	}
	if !interfacePattern.MatchString(c.InterfaceName) || c.InterfaceName == "lo" {
		return c, fmt.Errorf("interfaceName must be 1-15 alphanumeric, underscore or hyphen characters, and not lo")
	}
	if c.MTU == 0 {
		c.MTU = 1500
	}
	if c.MTU < 68 || c.MTU > 65535 {
		return c, fmt.Errorf("mtu must be between 68 and 65535")
	}
	for _, address := range []string{c.PodAddress, c.HostAddress} {
		if address == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(address)
		if err != nil || prefix.Addr().IsUnspecified() || prefix.Addr().IsMulticast() {
			return c, fmt.Errorf("invalid unicast CIDR address %q", address)
		}
		if prefix.Addr().Is6() && c.MTU < 1280 {
			return c, fmt.Errorf("IPv6 requires mtu >= 1280")
		}
	}
	return c, nil
}

func matchesRequest(requests []string, allocated string) bool {
	if len(requests) == 0 {
		return true
	}
	for _, request := range requests {
		if request == allocated || request == strings.SplitN(allocated, "/", 2)[0] {
			return true
		}
	}
	return false
}

// A minimal driver supports exactly one allocated veth per claim. Read the
// allocation's effective configuration, not unfiltered claim spec entries.
func claimConfig(claim *resourceapi.ResourceClaim, node string, count int) (Config, []kubeletplugin.Device, error) {
	if claim.Status.Allocation == nil {
		return Config{}, nil, fmt.Errorf("claim is not allocated")
	}
	if len(claim.Status.ReservedFor) > 1 {
		return Config{}, nil, fmt.Errorf("sharing a veth claim between pods is not supported")
	}
	var allocated []resourceapi.DeviceRequestAllocationResult
	for _, result := range claim.Status.Allocation.Devices.Results {
		if result.Driver == driverName {
			allocated = append(allocated, result)
		}
	}
	if len(allocated) != 1 {
		return Config{}, nil, fmt.Errorf("expected exactly one allocated device for %s, got %d", driverName, len(allocated))
	}
	device := allocated[0]
	validDevice := false
	for i := 0; i < count; i++ {
		if device.Device == fmt.Sprintf("veth-%03d", i) {
			validDevice = true
			break
		}
	}
	if device.Pool != node || !validDevice || device.ShareID != nil {
		return Config{}, nil, fmt.Errorf("device %s/%s is not an exclusive device on node %s", device.Pool, device.Device, node)
	}
	var configs [][]byte
	for _, config := range claim.Status.Allocation.Devices.Config {
		if config.Source != resourceapi.AllocationConfigSourceClaim || config.Opaque == nil || config.Opaque.Driver != driverName || !matchesRequest(config.Requests, device.Request) {
			continue
		}
		configs = append(configs, config.Opaque.Parameters.Raw)
	}
	if len(configs) != 1 {
		return Config{}, nil, fmt.Errorf("expected exactly one matching claim opaque configuration, got %d", len(configs))
	}
	c, err := decodeConfig(configs[0])
	if err != nil {
		return Config{}, nil, err
	}
	return c, []kubeletplugin.Device{{Requests: []string{device.Request}, PoolName: device.Pool, DeviceName: device.Device}}, nil
}

// Deterministic names allow cleanup after restart without retaining claim data.
func linkNames(uid types.UID) (host, peer string) {
	hash := sha256.Sum256([]byte(driverName + ":" + string(uid)))
	return fmt.Sprintf("vh%x", hash[:6]), fmt.Sprintf("vp%x", hash[:6])
}

func ownerPrefix(uid types.UID) string { return driverName + ":" + string(uid) + ":" }
