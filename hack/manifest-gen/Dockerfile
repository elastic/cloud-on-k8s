FROM docker.elastic.co/wolfi/go:1.26.0-r0@sha256:c27ce38190ffb76f410d7c67a28862a4d207d082b3c85047485428cbcb7dde2e as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
