FROM golang:1.11 as builder

# Build
WORKDIR /go/src/github.com/elastic/localvolume

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 && \
    go build -o bin/driverclient ./cmd/driverclient && \
    go build -o bin/driverdaemon ./cmd/driverdaemon && \
    go build -o bin/provisioner  ./cmd/provisioner

# Copy artefacts
WORKDIR /app/
RUN cp /go/src/github.com/elastic/localvolume/bin/* . && \
    cp /go/src/github.com/elastic/localvolume/scripts/* . && \
    rm -r /go/src/

# --

FROM centos:7
WORKDIR /app/
COPY --from=builder /app/ .
ENTRYPOINT ["/app/entrypoint.sh"]