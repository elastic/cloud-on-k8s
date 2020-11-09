# Docker image for the E2E tests runner
FROM golang:1.15.4

ARG E2E_JSON
ENV E2E_JSON $E2E_JSON

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# Create the go test cache directory
RUN mkdir -p /.cache && chmod 777 /.cache

# Download dependencies
COPY ["go.mod", "go.sum","./"]
RUN go mod download

# Copy the source
COPY pkg/ pkg/
COPY config/ config/
COPY test/ test/

RUN chgrp -R 0 /go && chmod -R g=u /go

# If a restricted PSP is applied we can't run as root
USER 101

ENTRYPOINT ["test/e2e/run.sh"]
