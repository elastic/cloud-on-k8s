# Docker image for the E2E tests runner
FROM --platform=$TARGETPLATFORM golang:1.18.4

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG E2E_JSON
ENV E2E_JSON $E2E_JSON
ARG E2E_TAGS
ENV E2E_TAGS $E2E_TAGS

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# create the go test cache directory
RUN mkdir -p /.cache && chmod 777 /.cache

# non-root user to support restricted PSP
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
