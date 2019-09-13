# Run e2e tests
FROM golang:1.11

ENV GOTESTSUM_VERSION=0.3.5

# Install gotestsum to generate xUnit compatible XML for tests
RUN curl -fsSLO https://github.com/gotestyourself/gotestsum/releases/download/v${GOTESTSUM_VERSION}/gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    tar xzf gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz && \
    mv gotestsum /usr/local/bin/gotestsum && chmod +x /usr/local/bin/gotestsum && \
    rm gotestsum_${GOTESTSUM_VERSION}_linux_amd64.tar.gz

# Create the go test cache directory
RUN mkdir -p /.cache && chmod 777 /.cache
# If a restricted PSP is applied we can't run as root
USER 101
# Copy in the go src
WORKDIR /go/src/github.com/elastic/cloud-on-k8s
COPY pkg/    pkg/
COPY config/ config/
COPY test/   test/
COPY vendor/ vendor/

# Run the tests
ENTRYPOINT ["test/e2e/run.sh"]
