# Build the operator binary
FROM --platform=$TARGETPLATFORM golang:1.17.8 as builder

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
			-o elastic-operator github.com/elastic/cloud-on-k8s/cmd

# Copy the operator binary into a lighter image
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.5

ARG VERSION

# Add required ECK labels and override labels from base image
LABEL name="Elastic Cloud on Kubernetes" \
      io.k8s.display-name="Elastic Cloud on Kubernetes" \
      maintainer="eck@elastic.co" \
      vendor="Elastic" \
      version="$VERSION" \
      url="https://www.elastic.co/guide/en/cloud-on-k8s/" \
      summary="Run Elasticsearch, Kibana, APM Server, Enterprise Search, and Beats on Kubernetes and OpenShift" \
      description="Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, APM Server, Beats, and Enterprise Search on Kubernetes" \
      io.k8s.description="Elastic Cloud on Kubernetes automates the deployment, provisioning, management, and orchestration of Elasticsearch, Kibana, APM Server, Beats, and Enterprise Search on Kubernetes"

# Update the base image packages to the latest versions
RUN microdnf update --setopt=tsflags=nodocs && microdnf clean all

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/elastic-operator .
COPY config/eck.yaml /conf/eck.yaml

# Copy NOTICE.txt and LICENSE.txt into the image
COPY *.txt /licenses/

USER 1001

ENTRYPOINT ["./elastic-operator"]
CMD ["manager"]
