# Docker image for the E2E tests runner
FROM docker.io/library/golang:1.21.6

ARG GO_TAGS

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

ENV ECK_DIAG_VERSION=1.5.2
RUN curl -fsSLO https://github.com/elastic/eck-diagnostics/releases/download/${ECK_DIAG_VERSION}/eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    tar xzf eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
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
COPY deploy/ deploy/

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
