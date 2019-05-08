# Run e2e tests
FROM golang:1.11

# Copy in the go src
WORKDIR /go/src/github.com/elastic/k8s-operators/operators
COPY pkg/    pkg/
COPY config/ config/
COPY test/   test/
COPY vendor/ vendor/

# Run the tests
ENTRYPOINT ["go", "test", "-v", "-failfast", "-timeout", "2h", "-tags=e2e", "./test/e2e"]