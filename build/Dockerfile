# Build the operator binary
FROM docker.elastic.co/wolfi/go:1.25.6-r0@sha256:8dd06d6e5de640aac12a318c77e486381ee122a7a281887c6f7ddf4ff1b4d56f as builder

ARG VERSION
ARG SHA1
ARG SNAPSHOT
ARG GO_TAGS
ARG LICENSE_PUBKEY

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# Cache go dependencies
COPY Makefile go.mod go.sum ./
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod download

# Copy default eck config
COPY config/eck.yaml .

# Copy the sources
COPY pkg/ pkg/
COPY cmd/ cmd/
COPY build/$LICENSE_PUBKEY /$LICENSE_PUBKEY

# Build
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod \
      CGO_ENABLED=0 GOOS=linux LICENSE_PUBKEY=/$LICENSE_PUBKEY make go-build

# ---------------------------------------------
# Copy the operator binary into a lighter image
FROM docker.elastic.co/wolfi/static:latest@sha256:a301031ffd4ed67f35ca7fa6cf3dad9937b5fa47d7493955a18d9b4ca5412d1a

ARG VERSION

# Add common ECK labels and OCI annotations to image
LABEL io.k8s.description="Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, APM Server, Beats, and Enterprise Search on Kubernetes" \
      io.k8s.display-name="Elastic Cloud on Kubernetes" \
      org.opencontainers.image.authors="eck@elastic.co" \
      org.opencontainers.image.base.name="gcr.io/distroless/static:nonroot" \
      org.opencontainers.image.description="Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, APM Server, Beats, and Enterprise Search on Kubernetes" \
      org.opencontainers.image.documentation="https://www.elastic.co/guide/en/cloud-on-k8s/" \
      org.opencontainers.image.licenses="Elastic-License-2.0" \
      org.opencontainers.image.source="https://github.com/elastic/cloud-on-k8s/" \
      org.opencontainers.image.title="Elastic Cloud on Kubernetes" \
      org.opencontainers.image.vendor="Elastic" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.url="https://github.com/elastic/cloud-on-k8s/"

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/elastic-operator /elastic-operator
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/eck.yaml         /conf/eck.yaml

# Copy NOTICE.txt and LICENSE.txt into the image
COPY *.txt /licenses/

ENTRYPOINT ["/elastic-operator"]
CMD ["manager"]
