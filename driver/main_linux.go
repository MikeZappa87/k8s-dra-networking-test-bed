package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	node := flag.String("node-name", os.Getenv("NODE_NAME"), "Kubernetes node name")
	count := flag.Int("devices", 32, "Number of exclusive veth slots per node (1-64)")
	kubeconfig := flag.String("kubeconfig", "", "Kubeconfig; empty uses in-cluster authentication")
	kubeletDir := flag.String("kubelet-dir", "/var/lib/kubelet", "Kubelet data directory, mounted at the same path")
	flag.Parse()
	defer klog.Flush()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, cancel, *node, *count, *kubeconfig, *kubeletDir); err != nil {
		klog.ErrorS(err, "Driver stopped")
		os.Exit(1)
	}
}

func run(ctx context.Context, cancel context.CancelFunc, node string, count int, kubeconfig, kubeletDir string) error {
	if node == "" || count < 1 || count > 64 {
		return fmt.Errorf("node-name is required and devices must be between 1 and 64")
	}
	var config *rest.Config
	var err error
	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		return err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}
	pluginDir := filepath.Join(kubeletDir, "plugins", driverName)
	registryDir := filepath.Join(kubeletDir, "plugins_registry")
	for _, dir := range []string{pluginDir, registryDir} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	d := &driver{node: node, count: count, cancel: cancel}
	helper, err := kubeletplugin.Start(ctx, d,
		kubeletplugin.DriverName(driverName), kubeletplugin.NodeName(node),
		kubeletplugin.KubeClient(client), kubeletplugin.PluginDataDirectoryPath(pluginDir),
		kubeletplugin.RegistrarDirectoryPath(registryDir),
		kubeletplugin.NodeV1(true), kubeletplugin.HealthService(false),
	)
	if err != nil {
		return err
	}
	defer helper.Stop()
	devices := make([]resourceapi.Device, count)
	for i := range devices {
		devices[i] = resourceapi.Device{Name: fmt.Sprintf("veth-%03d", i)}
	}
	if err := helper.PublishResources(ctx, resourceslice.DriverResources{Pools: map[string]resourceslice.Pool{
		node: {Slices: []resourceslice.Slice{{Devices: devices}}},
	}}); err != nil {
		return err
	}
	klog.InfoS("Veth DRA driver started", "driver", driverName, "node", node, "devices", count)
	<-ctx.Done()
	return nil
}
