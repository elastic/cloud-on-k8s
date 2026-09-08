FROM docker.elastic.co/wolfi/go:v1.27.1-r0@sha256:6c1f925fc75e0e67396a0019b3410c7b64f6a82f16acaad4df8033f8aaf79532 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
