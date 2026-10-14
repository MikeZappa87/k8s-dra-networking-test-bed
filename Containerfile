# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Build inside Linux containers for the node image's target architecture.
ARG GO_VERSION=1.27.1
ARG BASE_IMAGE=dra-poc-kubernetes:dev

FROM golang:${GO_VERSION}-bookworm AS containerd-build
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates git make rsync \
	&& rm -rf /var/lib/apt/lists/*
ARG CONTAINERD_REPO=https://github.com/MikeZappa87/containerd.git
ARG CONTAINERD_BRANCH=mzappa/nonnetworkpods
# Optional immutable commit; otherwise build the branch tip when this layer runs.
ARG CONTAINERD_COMMIT=
RUN git clone --depth 1 --single-branch --branch "${CONTAINERD_BRANCH}" \
	"${CONTAINERD_REPO}" /src/containerd \
	&& if [ -n "${CONTAINERD_COMMIT}" ]; then \
		git -C /src/containerd fetch --depth 1 origin "${CONTAINERD_COMMIT}" \
		&& git -C /src/containerd checkout --detach "${CONTAINERD_COMMIT}"; \
	fi
WORKDIR /src/containerd
RUN CGO_ENABLED=0 make STATIC=1 BUILDTAGS=no_btrfs \
	bin/containerd bin/ctr bin/containerd-shim-runc-v2

# Preserve kind services, runtime configuration, CNI, Kubernetes binaries, and
# preloaded control-plane images. Replace only the containerd runtime binaries.
FROM ${BASE_IMAGE}
COPY --from=containerd-build /src/containerd/bin/containerd /usr/local/bin/containerd
COPY --from=containerd-build /src/containerd/bin/ctr /usr/local/bin/ctr
COPY --from=containerd-build /src/containerd/bin/containerd-shim-runc-v2 /usr/local/bin/containerd-shim-runc-v2

# first tell systemd that it is in docker (it will check for the container env)
# https://systemd.io/CONTAINER_INTERFACE/
ENV container=docker
# systemd exits on SIGRTMIN+3, not SIGTERM (which re-executes it)
# https://bugzilla.redhat.com/show_bug.cgi?id=1201657
STOPSIGNAL SIGRTMIN+3

# NOTE: this is *only* for documentation, the entrypoint is overridden later
ENTRYPOINT [ "/usr/local/bin/entrypoint", "/sbin/init" ]