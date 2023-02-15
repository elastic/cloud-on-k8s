# Docker image for the E2E tests runner
FROM --platform=$TARGETPLATFORM docker.io/library/golang:1.19.5

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG E2E_JSON
ENV E2E_JSON $E2E_JSON
ARG E2E_TAGS
ENV E2E_TAGS $E2E_TAGS

ENV GCLOUD_VERSION=381.0.0
ENV ECK_DIAG_VERSION=1.4.0

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# gcloud sdk to use gsutil
ENV PATH=${PATH}:/usr/local/google-cloud-sdk/bin
RUN curl -fsSLO https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz && \
    mkdir -p /usr/local/gcloud && \
    tar -zxf google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz -C /usr/local && \
    /usr/local/google-cloud-sdk/install.sh --quiet && \
    gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    rm google-cloud-sdk-${GCLOUD_VERSION}-linux-x86_64.tar.gz

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
