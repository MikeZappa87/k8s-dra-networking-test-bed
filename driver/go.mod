module example.com/veth-dra-driver

go 1.27.0

require (
	github.com/containernetworking/plugins v1.8.0
	github.com/vishvananda/netlink v1.3.1
	golang.org/x/sys v0.48.0
	k8s.io/api v0.0.0
	k8s.io/apimachinery v0.0.0
	k8s.io/client-go v0.0.0
	k8s.io/dynamic-resource-allocation v0.0.0
	k8s.io/klog/v2 v2.140.0
	k8s.io/kubelet v0.0.0
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/containernetworking/cni v1.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fxamacker/cbor/v2 v2.9.4 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-openapi/jsonpointer v1.0.2 // indirect
	github.com/go-openapi/jsonreference v1.0.2 // indirect
	github.com/go-openapi/swag/conv v0.29.2 // indirect
	github.com/go-openapi/swag/jsonutils v0.29.2 // indirect
	github.com/go-openapi/swag/pools v0.29.2 // indirect
	github.com/go-openapi/swag/typeutils v0.29.2 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.etcd.io/etcd/client/pkg/v3 v3.7.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/kube-openapi v0.0.0-20261007072838-e2e80c32a35f // indirect
	k8s.io/utils v0.0.0-20260626114624-be93311217bd // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v7 v7.0.0 // indirect
	tags.cncf.io/container-device-interface/specs-go v1.1.1 // indirect
)

// Use the same fork as the kubelet: released modules lack ConfigurePodResources.
replace (
	k8s.io/api => ../../kubernetes/staging/src/k8s.io/api
	k8s.io/apimachinery => ../../kubernetes/staging/src/k8s.io/apimachinery
	k8s.io/apiserver => ../../kubernetes/staging/src/k8s.io/apiserver
	k8s.io/client-go => ../../kubernetes/staging/src/k8s.io/client-go
	k8s.io/component-base => ../../kubernetes/staging/src/k8s.io/component-base
	k8s.io/component-helpers => ../../kubernetes/staging/src/k8s.io/component-helpers
	k8s.io/dynamic-resource-allocation => ../../kubernetes/staging/src/k8s.io/dynamic-resource-allocation
	k8s.io/kms => ../../kubernetes/staging/src/k8s.io/kms
	k8s.io/ktesting => ../../kubernetes/staging/src/k8s.io/ktesting
	k8s.io/kubelet => ../../kubernetes/staging/src/k8s.io/kubelet
	k8s.io/streaming => ../../kubernetes/staging/src/k8s.io/streaming
)
