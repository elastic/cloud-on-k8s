# Build the operator binary
FROM --platform=$TARGETPLATFORM golang:1.18.4 as builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG GO_LDFLAGS
ARG GO_TAGS
WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

# Copy the go source
COPY pkg/    pkg/
COPY cmd/    cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux \
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

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/elastic-operator .
COPY config/eck.yaml /conf/eck.yaml

# Copy NOTICE.txt and LICENSE.txt into the image
COPY *.txt /licenses/

ENTRYPOINT ["./elastic-operator"]
CMD ["manager"]
