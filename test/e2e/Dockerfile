# Run e2e tests
FROM golang:1.11

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
