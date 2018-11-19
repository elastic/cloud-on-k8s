# Build the manager binary
FROM golang:1.11 as builder

# Copy in the go src
WORKDIR /go/src/github.com/elastic/stack-operators
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o stack-operator github.com/elastic/stack-operators/cmd

# Copy the controller-manager into a thin image
FROM ubuntu:latest
WORKDIR /root/
COPY --from=builder /go/src/github.com/elastic/stack-operators .
ENTRYPOINT ["./stack-operator"]
CMD ["manager"]
