# Build the manager binary
FROM golang:1.11 as builder

# Copy in the go src
WORKDIR /go/src/github.com/elastic/cloud-on-k8s/operators
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

ARG GO_TAGS

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags="$GO_TAGS" -a -o elastic-operator github.com/elastic/cloud-on-k8s/operators/cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags="$GO_TAGS" -a -o process-manager github.com/elastic/cloud-on-k8s/operators/cmd/process-manager

# Copy the controller-manager into a thin image
FROM centos:7
WORKDIR /root/
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/operators/elastic-operator .
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/operators/process-manager .
ENTRYPOINT ["./elastic-operator"]
CMD ["manager"]
