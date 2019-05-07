FROM golang:1.11 as builder

# Build
WORKDIR /go/src/github.com/elastic/cloud-on-k8s/local-volume

COPY vendor/ vendor/
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY scripts/    scripts/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 && \
    go build -o bin/driverclient ./cmd/driverclient && \
    go build -o bin/driverdaemon ./cmd/driverdaemon && \
    go build -o bin/provisioner  ./cmd/provisioner

# Copy artefacts
WORKDIR /app/
RUN cp /go/src/github.com/elastic/cloud-on-k8s/local-volume/bin/* . && \
    cp /go/src/github.com/elastic/cloud-on-k8s/local-volume/scripts/* . && \
    rm -r /go/src/

# --

FROM centos:7 as base

RUN yum install -y lvm2 e2fsprogs && \
    # set udev_sync and udev_rules to 0 to let LVM handle volumes and device mounting itself without waiting for udev
    sed -i -e 's/udev_sync = 1/udev_sync = 0/g' -e 's/udev_rules = 1/udev_rules = 0/g' /etc/lvm/lvm.conf

# --

FROM base
WORKDIR /app/
COPY --from=builder /app/ .
ENTRYPOINT ["/app/entrypoint.sh"]
