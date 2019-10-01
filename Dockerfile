# Build the manager binary
FROM golang:1.13 as builder

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build \
            -mod readonly \
			-ldflags "$GO_LDFLAGS" -tags="$GO_TAGS" -a \
			-o elastic-operator github.com/elastic/cloud-on-k8s/cmd

# Copy the controller-manager into a thin image
FROM centos:7

RUN set -x \
    && groupadd --system --gid 101 elastic \
    && useradd --system -g elastic -m --home /eck -c "eck user" --shell /bin/false --uid 101 elastic \
    && chmod 755 /eck

WORKDIR /eck
USER 101

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/elastic-operator .
ENTRYPOINT ["./elastic-operator"]
CMD ["manager"]
