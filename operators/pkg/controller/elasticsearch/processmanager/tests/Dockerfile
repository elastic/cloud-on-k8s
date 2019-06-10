# Build the manager binary
FROM golang:1.11 as builder

# Copy in the go src
WORKDIR /go/src/github.com/elastic/cloud-on-k8s/operators
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o process-manager github.com/elastic/cloud-on-k8s/operators/cmd/process-manager

# Copy the controller-manager into a thin image
FROM docker.elastic.co/elasticsearch/elasticsearch:6.7.0

VOLUME ["/mnt/elastic-internal/process-manager"]

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/operators/process-manager /process-manager
ENTRYPOINT ["/process-manager"]
