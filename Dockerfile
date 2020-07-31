# Build the operator binary
FROM golang:1.14.6 as builder

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

# Copy the operator binary into a lighter image
FROM registry.access.redhat.com/ubi8/ubi-minimal:8.2
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/elastic-operator .

USER 1001

ENTRYPOINT ["./elastic-operator"]
CMD ["manager"]
