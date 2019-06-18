# Run e2e tests
FROM golang:1.11

# If a restricted PSP is applied we can't run as root
USER 101
# Copy in the go src
WORKDIR /go/src/github.com/elastic/cloud-on-k8s/operators
COPY pkg/    pkg/
COPY config/ config/
COPY test/   test/
COPY vendor/ vendor/

# Run the tests
ENTRYPOINT ["go", "test", "-v", "-failfast", "-timeout", "2h", "-tags=e2e", "./test/e2e"]