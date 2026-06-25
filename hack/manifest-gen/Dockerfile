FROM docker.elastic.co/wolfi/go:v1.26.4-r1@sha256:1ee72a66d128eb44b2431e7730d60a50d578041f904ec9bd77b1dd94aa4c5f8d as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
