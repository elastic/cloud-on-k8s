# Docker image for the E2E tests runner
FROM docker.io/library/golang:1.26.0

ARG GO_TAGS
ARG ARCH

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

ENV ECK_DIAG_VERSION=1.9.0
# Download and extract the correct eck-diagnostics binary based on the image architecture
# ARCH is passed from drivah.toml.sh (amd64 or arm64)
# Map to eck-diagnostics naming: x86_64 for amd64, arm64 for arm64
RUN case "${ARCH}" in \
      amd64) ECK_DIAG_ARCH="x86_64" ;; \
      arm64) ECK_DIAG_ARCH="arm64" ;; \
      *) echo "Unsupported architecture: ${ARCH}"; exit 1 ;; \
    esac; \
    curl -fsSLO "https://github.com/elastic/eck-diagnostics/releases/download/${ECK_DIAG_VERSION}/eck-diagnostics_${ECK_DIAG_VERSION}_Linux_${ECK_DIAG_ARCH}.tar.gz" && \
    tar xzf "eck-diagnostics_${ECK_DIAG_VERSION}_Linux_${ECK_DIAG_ARCH}.tar.gz" && \
    mv eck-diagnostics /usr/local/bin/eck-diagnostics

# create the go test cache directory
RUN mkdir -p /.cache && chmod 777 /.cache

# cache go dependencies
COPY Makefile go.mod go.sum ./
RUN go mod download

# copy sources
COPY pkg/ pkg/
COPY cmd/ cmd/
COPY config/ config/
COPY test/ test/

# non-root user to support restricted security policies
RUN chown -R 1001 /go/src/github.com/elastic/cloud-on-k8s
USER 1001

# go generate pub key
COPY test/e2e/license.key /license.key
RUN LICENSE_PUBKEY=/license.key go generate -tags="${GO_TAGS}" ./pkg/... ./cmd/...

# Set r/w permissions for user members of root group (GID=0) to support OpenShift Container Platform running containers
# using arbitrarily assigned user ID, which is always member of the root group.
RUN chgrp -R 0 /.cache/go-build/ /go/src/github.com/elastic/cloud-on-k8s && \
    chmod -R g=u /.cache/go-build/ /go/src/github.com/elastic/cloud-on-k8s

ENTRYPOINT ["test/e2e/run.sh"]
