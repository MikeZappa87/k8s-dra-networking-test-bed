SHELL := /bin/bash
.DEFAULT_GOAL := help
ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
GO ?= go
ARCH ?= $(shell uname -m | sed 's/aarch64/arm64/;s/x86_64/amd64/')
IMAGE ?= veth-dra-driver:dev
DOCKER ?= docker
KIND ?= $(shell command -v kind 2>/dev/null || printf '%s/bin/kind' "$$(go env GOPATH)")
KUBECTL ?= kubectl
CLUSTER_NAME ?= dra-poc
CONTEXT := kind-$(CLUSTER_NAME)
WAIT ?= 180s
# Use an existing Linux image with a shell for privileged, isolated netns tests.
TEST_IMAGE ?= mcr.microsoft.com/devcontainers/base:1-ubuntu-22.04

.PHONY: help build test integration-test image load deploy example check clean
help:
	@printf '%s\n' 'make -C driver build             Cross-compile the Linux driver' \
	  'make -C driver test              Run configuration unit tests' \
	  'make -C driver integration-test  Test veth lifecycle in an isolated privileged container' \
	  'make -C driver image             Build the driver container' \
	  'make -C driver deploy            Build, load into kind, deploy driver and DeviceClass' \
	  'make -C driver example           Create an example claim and Pod' \
	  'make -C driver check             Check net1 and ping the host end' \
	  'make -C driver clean             Remove example Pod and claim'

build:
	cd "$(ROOT)" && GOWORK=off GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 $(GO) build -trimpath -o bin/veth-dra-driver .

test:
	cd "$(ROOT)" && GOWORK=off $(GO) test ./...

integration-test:
	cd "$(ROOT)" && GOWORK=off GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 $(GO) test -tags=integration -c -o bin/network.test .
	$(DOCKER) run --rm --privileged --network none --user 0 --platform linux/$(ARCH) \
	  --mount type=bind,src="$(ROOT)/bin",dst=/tests,readonly \
	  --mount type=bind,src="$(ROOT)",dst=/driver,readonly --workdir /driver \
	  --entrypoint /tests/network.test $(TEST_IMAGE) -test.v

image: build
	$(DOCKER) build --platform linux/$(ARCH) -f "$(ROOT)/Containerfile" -t "$(IMAGE)" "$(ROOT)"

load: image
	"$(KIND)" load docker-image --name "$(CLUSTER_NAME)" "$(IMAGE)"

deploy: load
	$(KUBECTL) --context "$(CONTEXT)" apply -f "$(ROOT)/deploy.yaml"
	$(KUBECTL) --context "$(CONTEXT)" -n kube-system set image daemonset/veth-dra-driver driver="$(IMAGE)"
	$(KUBECTL) --context "$(CONTEXT)" -n kube-system rollout status daemonset/veth-dra-driver --timeout="$(WAIT)"

example:
	$(KUBECTL) --context "$(CONTEXT)" apply -f "$(ROOT)/example.yaml"
	$(KUBECTL) --context "$(CONTEXT)" wait --for=condition=Ready pod/veth-demo --timeout="$(WAIT)"

check:
	$(KUBECTL) --context "$(CONTEXT)" exec veth-demo -- sh -ec \
	  'ip link show net1; ip -4 addr show net1 | grep "192.0.2.2/30"; ping -c 3 -I net1 192.0.2.1'

clean:
	$(KUBECTL) --context "$(CONTEXT)" delete -f "$(ROOT)/example.yaml" --ignore-not-found