package main

import (
	"os"
	"strings"
	"testing"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"
)

func testClaim() *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default", UID: "test-claim-uid"},
		Status: resourceapi.ResourceClaimStatus{Allocation: &resourceapi.AllocationResult{
			Devices: resourceapi.DeviceAllocationResult{
				Results: []resourceapi.DeviceRequestAllocationResult{{Driver: driverName, Pool: "node", Device: "veth-000", Request: "network"}},
				Config: []resourceapi.DeviceAllocationConfiguration{{
					Source: resourceapi.AllocationConfigSourceClaim, Requests: []string{"network"},
					DeviceConfiguration: resourceapi.DeviceConfiguration{Opaque: &resourceapi.OpaqueDeviceConfiguration{
						Driver: driverName, Parameters: runtime.RawExtension{Raw: []byte(`{"interfaceName":"net1","podAddress":"192.0.2.2/30","hostAddress":"192.0.2.1/30"}`)},
					}},
				}},
			},
		}},
	}
}

func TestDecodeConfig(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{"minimal", `{"interfaceName":"net1"}`, true},
		{"ipv6", `{"interfaceName":"net1","podAddress":"2001:db8::2/64"}`, true},
		{"missing interface", `{}`, false},
		{"loopback", `{"interfaceName":"lo"}`, false},
		{"too long", `{"interfaceName":"abcdefghijklmnop"}`, false},
		{"bad name", `{"interfaceName":"net/1"}`, false},
		{"unknown field", `{"interfaceName":"net1","bridge":"br0"}`, false},
		{"bad address", `{"interfaceName":"net1","podAddress":"192.0.2.2"}`, false},
		{"multicast", `{"interfaceName":"net1","podAddress":"224.0.0.1/24"}`, false},
		{"bad mtu", `{"interfaceName":"net1","mtu":67}`, false},
		{"ipv6 mtu", `{"interfaceName":"net1","mtu":1000,"podAddress":"2001:db8::2/64"}`, false},
		{"trailing object", `{"interfaceName":"net1"} {}`, false},
		{"null", `null`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := decodeConfig([]byte(test.raw))
			if (err == nil) != test.ok {
				t.Fatalf("config = %+v, err = %v, want success %v", config, err, test.ok)
			}
			if err == nil && config.MTU != 1500 {
				t.Fatalf("default MTU = %d", config.MTU)
			}
		})
	}
}

func TestClaimConfig(t *testing.T) {
	c, devices, err := claimConfig(testClaim(), "node", 32)
	if err != nil || c.InterfaceName != "net1" || len(devices) != 1 {
		t.Fatalf("config = %+v, devices = %+v, err = %v", c, devices, err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*resourceapi.ResourceClaim)
	}{
		{"unallocated", func(c *resourceapi.ResourceClaim) { c.Status.Allocation = nil }},
		{"wrong node", func(c *resourceapi.ResourceClaim) { c.Status.Allocation.Devices.Results[0].Pool = "other" }},
		{"unknown device", func(c *resourceapi.ResourceClaim) { c.Status.Allocation.Devices.Results[0].Device = "veth-999" }},
		{"multiple devices", func(c *resourceapi.ResourceClaim) {
			c.Status.Allocation.Devices.Results = append(c.Status.Allocation.Devices.Results, c.Status.Allocation.Devices.Results[0])
		}},
		{"wrong request", func(c *resourceapi.ResourceClaim) { c.Status.Allocation.Devices.Config[0].Requests = []string{"other"} }},
		{"class only", func(c *resourceapi.ResourceClaim) {
			c.Status.Allocation.Devices.Config[0].Source = resourceapi.AllocationConfigSourceClass
		}},
		{"duplicate config", func(c *resourceapi.ResourceClaim) {
			c.Status.Allocation.Devices.Config = append(c.Status.Allocation.Devices.Config, c.Status.Allocation.Devices.Config[0])
		}},
		{"shared claim", func(c *resourceapi.ResourceClaim) {
			c.Status.ReservedFor = make([]resourceapi.ResourceClaimConsumerReference, 2)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			claim := testClaim()
			test.mutate(claim)
			if _, _, err := claimConfig(claim, "node", 32); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestRequestMatching(t *testing.T) {
	if !matchesRequest(nil, "network/sub") || !matchesRequest([]string{"network"}, "network/sub") ||
		!matchesRequest([]string{"network/sub"}, "network/sub") || matchesRequest([]string{"network/other"}, "network/sub") {
		t.Fatal("incorrect request matching")
	}
}

func TestLinkNames(t *testing.T) {
	host, peer := linkNames("claim-a")
	other, _ := linkNames("claim-b")
	retry, _ := linkNames("claim-a")
	if len(host) > 15 || len(peer) > 15 || host == peer || host == other || host != retry {
		t.Fatalf("invalid names: %s %s %s %s", host, peer, other, retry)
	}
}

func TestManifests(t *testing.T) {
	decoder := serializer.NewCodecFactory(scheme.Scheme, serializer.EnableStrict).UniversalDeserializer()
	for _, name := range []string{"deploy.yaml", "example.yaml"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, document := range strings.Split(string(raw), "\n---\n") {
			jsonData, err := yaml.YAMLToJSONStrict([]byte(document))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if _, _, err := decoder.Decode(jsonData, nil, nil); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
}
