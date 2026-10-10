SHELL := /bin/bash
.DEFAULT_GOAL := help

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
KUBERNETES_DIR ?= $(ROOT)/../kubernetes
KUBERNETES_REPO ?= https://github.com/MikeZappa87/kubernetes.git
KUBERNETES_BRANCH ?= mzappa/nodeconfigurepoddrapoc
VERSION_FILE ?= $(ROOT)/kubernetes-version.env
ARCH ?= $(shell uname -m | sed 's/aarch64/arm64/;s/x86_64/amd64/')
KIND ?= $(shell command -v kind 2>/dev/null || printf '%s/bin/kind' "$$(go env GOPATH)")
DOCKER ?= docker
KUBECTL ?= kubectl
NODE_IMAGE ?= dra-poc-node:dev
DRIVER_IMAGE ?= veth-dra-driver:dev
GO ?= go
CLUSTER_NAME ?= dra-poc
CONTEXT := kind-$(CLUSTER_NAME)
WAIT ?= 180s
DRIVER_MAKE = $(MAKE) -C "$(ROOT)/driver" ARCH="$(ARCH)" GO="$(GO)" \
	DOCKER="$(DOCKER)" KIND="$(KIND)" KUBECTL="$(KUBECTL)" \
	CLUSTER_NAME="$(CLUSTER_NAME)" WAIT="$(WAIT)" IMAGE="$(DRIVER_IMAGE)"

.PHONY: help source kubernetes-image node-image build cluster up \
	driver-build driver-image driver-load driver-deploy driver-test \
	driver-integration-test example check test status delete

help:
	@printf '%s\n' \
	  'make build             Build the custom kind node and DRA driver images' \
	  'make node-image        Build a kind node directly from the Kubernetes checkout' \
	  'make kubernetes-image  Alias for node-image (clones sources if missing)' \
	  'make cluster           Create a cluster using the existing node image' \
	  'make up                Build images, create cluster, load and deploy DRA driver' \
	  'make driver-build      Compile the Linux DRA driver binary' \
	  'make driver-image      Build the DRA driver container image' \
	  'make driver-load       Build and load the DRA driver image into an existing cluster' \
	  'make driver-deploy     Build, load and deploy driver into an existing cluster' \
	  'make driver-test       Run driver unit and manifest tests' \
	  'make driver-integration-test  Run isolated privileged Linux namespace tests' \
	  'make example           Create the example ResourceClaim and Pod' \
	  'make check             Check the example veth and ping its host end' \
	  'make test              Deploy driver, create example and check connectivity' \
	  'make status            Show nodes and cluster workloads' \
	  'make delete            Delete the cluster (keep sources and images)' \
	  '' \
	  'Overrides: ARCH, KIND, KUBERNETES_DIR, VERSION_FILE, CLUSTER_NAME,' \
	  '           NODE_IMAGE, DRIVER_IMAGE, GO, DOCKER, KUBECTL, WAIT'

# Never reset, pull, or overwrite an existing source checkout.
source:
	@if [[ ! -e "$(KUBERNETES_DIR)" ]]; then \
	  git clone --single-branch --branch "$(KUBERNETES_BRANCH)" \
	    "$(KUBERNETES_REPO)" "$(KUBERNETES_DIR)"; \
	else \
	  git -C "$(KUBERNETES_DIR)" rev-parse --is-inside-work-tree >/dev/null; \
	fi

node-image: source
	KUBE_GIT_VERSION_FILE="$(abspath $(VERSION_FILE))" "$(KIND)" build node-image \
	  "$(KUBERNETES_DIR)" --type source --arch "$(ARCH)" --image "$(NODE_IMAGE)"

kubernetes-image: node-image

build: node-image driver-image

driver-build: source
	+$(DRIVER_MAKE) build

driver-image: source
	+$(DRIVER_MAKE) image

driver-load: driver-image
	"$(KIND)" load docker-image --name "$(CLUSTER_NAME)" "$(DRIVER_IMAGE)"

driver-deploy: driver-load
	$(KUBECTL) --context "$(CONTEXT)" apply -f "$(ROOT)/driver/deploy.yaml"
	$(KUBECTL) --context "$(CONTEXT)" -n kube-system set image daemonset/veth-dra-driver driver="$(DRIVER_IMAGE)"
	# Restart even when the image tag is unchanged, so rebuilt dev images are used.
	$(KUBECTL) --context "$(CONTEXT)" -n kube-system rollout restart daemonset/veth-dra-driver
	$(KUBECTL) --context "$(CONTEXT)" -n kube-system rollout status daemonset/veth-dra-driver --timeout="$(WAIT)"

driver-test: source
	+$(DRIVER_MAKE) test

driver-integration-test: source
	+$(DRIVER_MAKE) integration-test

cluster:
	"$(KIND)" create cluster --name "$(CLUSTER_NAME)" \
	  --config "$(ROOT)/kind.yaml" --image "$(NODE_IMAGE)" --wait "$(WAIT)"

up: build
	$(MAKE) cluster
	# Both images were already built; skip rebuilding the driver during deploy.
	$(MAKE) driver-deploy DRIVER_IMAGE="$(DRIVER_IMAGE)" -o driver-image

example:
	+$(DRIVER_MAKE) example

check:
	+$(DRIVER_MAKE) check

# Recursive calls serialize these operations even with make -j.
test: driver-test driver-deploy
	$(MAKE) example
	$(MAKE) check

status:
	$(KUBECTL) --context "$(CONTEXT)" get nodes -o wide
	$(KUBECTL) --context "$(CONTEXT)" get pods --all-namespaces -o wide

delete:
	"$(KIND)" delete cluster --name "$(CLUSTER_NAME)"