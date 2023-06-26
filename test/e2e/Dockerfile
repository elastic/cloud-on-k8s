# Docker image for the E2E tests runner
FROM --platform=$TARGETPLATFORM docker.io/library/golang:1.20.5

ARG TARGETPLATFORM
ARG BUILDPLATFORM

ENV ECK_DIAG_VERSION=1.4.0

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

RUN curl -fsSLO https://github.com/elastic/eck-diagnostics/releases/download/${ECK_DIAG_VERSION}/eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    tar xzf eck-diagnostics_${ECK_DIAG_VERSION}_Linux_x86_64.tar.gz && \
    mv eck-diagnostics /usr/local/bin/eck-diagnostics

# create the go test cache directory
RUN mkdir -p /.cache && chmod 777 /.cache

# non-root user to support restricted security policies
RUN chown -R 1001 /go/src/github.com/elastic/cloud-on-k8s
USER 1001

# download go dependencies
COPY ["go.mod", "go.sum","./"]
RUN go mod download

# copy sources
COPY pkg/ pkg/
COPY config/ config/
COPY test/ test/

ENTRYPOINT ["test/e2e/run.sh"]
