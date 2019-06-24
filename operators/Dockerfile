# Build the manager binary
FROM golang:1.11 as builder

# Copy in the go src
WORKDIR /go/src/github.com/elastic/cloud-on-k8s/operators
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

ARG GO_LDFLAGS
ARG GO_TAGS

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build \
			-ldflags "$GO_LDFLAGS" -tags="$GO_TAGS" -a \
			-o elastic-operator github.com/elastic/cloud-on-k8s/operators/cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags="$GO_TAGS" -a -o process-manager github.com/elastic/cloud-on-k8s/operators/cmd/process-manager

# Copy the controller-manager and the process-manager into a thin image
FROM centos:7

RUN set -x \
    && groupadd --system --gid 101 elastic \
    && useradd --system -g elastic -m --home /eck -c "eck user" --shell /bin/false --uid 101 elastic \
    && chmod 755 /eck

WORKDIR /eck
USER 101

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/operators/elastic-operator .
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/operators/process-manager .
ENTRYPOINT ["./elastic-operator"]
CMD ["manager"]
