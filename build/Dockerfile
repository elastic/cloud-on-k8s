# Build the operator binary
FROM docker.io/library/golang:1.20.5 as builder

ARG VERSION
ARG SHA1
ARG SNAPSHOT
ARG GO_TAGS
ARG LICENSE_PUBKEY_PATH

ENV LICENSE_PUBKEY=/license.key

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

ENV HELM_VERSION=3.10.1
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && ./get_helm.sh -v v${HELM_VERSION} --no-sudo && rm get_helm.sh

# Cache download dependencies go
COPY Makefile go.mod go.sum ./
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod download

# Generate default eck config (required Helm)
COPY deploy/ deploy/
RUN mkdir config && make generate-config-file

# Copy the sources
ENV LICENSE_PUBKEY=/license.key
COPY $LICENSE_PUBKEY_PATH /license.key
COPY pkg/ pkg/
COPY cmd/ cmd/

# Build
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod \
      CGO_ENABLED=0 GOOS=linux make go-build

# ---------------------------------------------
# Copy the operator binary into a lighter image
FROM gcr.io/distroless/static:nonroot

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
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/config/eck.yaml  /conf/eck.yaml

# Copy NOTICE.txt and LICENSE.txt into the image
COPY *.txt /licenses/

ENTRYPOINT ["/elastic-operator"]
CMD ["manager"]
