# Build the operator binary
FROM docker.io/library/golang:1.20.5 as builder

ARG GO_LDFLAGS
ARG GO_TAGS
ARG LICENSE_PUBKEY_PATH

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# ENV KUBECTL_VERSION=1.26.3
# RUN curl -fsSLO https://dl.k8s.io/v${KUBECTL_VERSION}/bin/linux/$(uname -m | sed -e "s|x86_|amd|" -e "s|aarch|arm|")/kubectl && \
#     mv kubectl /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl
ENV HELM_VERSION=3.10.1
RUN curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 && \
    chmod 700 get_helm.sh && ./get_helm.sh -v v${HELM_VERSION} --no-sudo && rm get_helm.sh

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY go.mod go.sum ./
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod go mod download

# # Copy the sources
COPY pkg/ pkg/
COPY cmd/ cmd/

# Generate pkg/controller/common/license/zz_generated.pubkey.go and config/eck.yaml
ENV LICENSE_PUBKEY=/license.key
COPY ${LICENSE_PUBKEY_PATH} /license.key
COPY Makefile VERSION ./
# COPY .git .git
RUN make go-generate
# COPY config/ config/
# COPY deploy/ deploy/
# COPY hack/ hack/
# RUN make generate-crds-v1 generate-config-file
COPY deploy/ deploy/
RUN helm template deploy/eck-operator -s templates/configmap.yaml \
      -f deploy/eck-operator/values.yaml --set=webhook.enabled=false --set=telemetry.distributionChannel=image \
      > eck.yaml

# Build
RUN --mount=type=cache,mode=0755,target=/go/pkg/mod CGO_ENABLED=0 GOOS=linux \
      go build \
      -mod readonly \
      -ldflags "$GO_LDFLAGS" -tags="$GO_TAGS" -a \
      -o elastic-operator github.com/elastic/cloud-on-k8s/v2/cmd

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
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/eck.yaml         /conf/eck.yaml

# Copy NOTICE.txt and LICENSE.txt into the image
COPY *.txt /licenses/

ENTRYPOINT ["/elastic-operator"]
CMD ["manager"]
