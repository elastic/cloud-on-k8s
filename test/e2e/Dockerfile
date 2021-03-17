# Docker image for the E2E tests runner
FROM --platform=$TARGETPLATFORM golang:1.16.2 as builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG GO_TAGS

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

COPY ["go.mod", "go.sum", "./"]
RUN go mod download

COPY pkg/    pkg/
COPY cmd/    cmd/
COPY test/ test/

RUN CGO_ENABLED=0 GOOS=linux \
    test/e2e/compile.sh

RUN CGO_ENABLED=0 GOOS=linux \
    go build \
       -o e2e-runner github.com/elastic/cloud-on-k8s/test/e2e/runner

RUN CGO_ENABLED=0 GOOS=linux \
    go build \
       -o chaos-runner github.com/elastic/cloud-on-k8s/test/e2e/cmd


FROM scratch
# Copy the config dir
COPY config/ config/

# e2e-runner replaces a shell script to invoke the different tests
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/e2e-runner .

# chaos-runner is the e2e cli to run chaos monkey style tests
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/chaos-runner .

# test2json is needed to produce json output
COPY --from=builder /usr/local/go/pkg/tool/linux_*/test2json .

# the precompiled tests
COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/*.test .

#RUN chgrp -R 0 /go && chmod -R g=u /go

# If a restricted PSP is applied we can't run as root
USER 101

ENTRYPOINT ["/e2e-runner"]