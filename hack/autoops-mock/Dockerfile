# Build the autoops-mock binary
FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:65dbe65914525e55521cbad942c13a87037f2837b5d44605a4b116e6e9d70693 as builder

WORKDIR /go/src/github.com/elastic/cloud-on-k8s

# Copy the source
COPY main.go .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -o autoops-mock main.go

# ---------------------------------------------
# Copy the binary into a lighter image
FROM docker.elastic.co/wolfi/static:latest@sha256:7bdd9720cefba78e8133c4d03eaaf3f18a25d147d2c8803cc830061e46b6b474

COPY --from=builder /go/src/github.com/elastic/cloud-on-k8s/autoops-mock /autoops-mock

ENTRYPOINT ["/autoops-mock"]
