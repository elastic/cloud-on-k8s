FROM docker.elastic.co/wolfi/go:1.25.0-r1@sha256:db1fe8921f0f251d32026d2f85ebd7ea0b239abe80d1062b9864c62016040a0a as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
