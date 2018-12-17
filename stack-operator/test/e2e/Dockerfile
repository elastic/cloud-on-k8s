# Run e2e tests
FROM golang:1.11

# Copy in the go src
WORKDIR /go/src/github.com/elastic/stack-operators/stack-operator
COPY pkg/    pkg/
COPY cmd/    cmd/
COPY test/   test/
COPY vendor/ vendor/

# Run the tests
ENTRYPOINT ["go", "test", "-v", "-failfast", "-timeout", "1h", "-tags=e2e", "./test/e2e"]