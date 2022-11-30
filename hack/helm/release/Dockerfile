# Build the operator binary
FROM docker.io/library/golang:1.19.3 as builder

WORKDIR /usr/local/src/helm-releaser

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
COPY ["go.mod", "go.sum", "./"]
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY pkg/    pkg/
COPY cmd/    cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
    -mod readonly \
    -o helm-releaser main.go

# Copy the operator binary into a lighter image
# FROM gcr.io/distroless/static:nonroot
FROM alpine

COPY --from=builder /usr/local/src/helm-releaser/helm-releaser /usr/local/bin/helm-releaser

WORKDIR /app
ENTRYPOINT ["/usr/local/bin/helm-releaser"]
CMD ["release"]
