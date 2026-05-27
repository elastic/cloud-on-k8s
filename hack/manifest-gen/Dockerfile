FROM docker.elastic.co/wolfi/go:v1.26.3-r3@sha256:d1a5981d9071d7dd5594c36b79fd5f6d2ec5f1c75958ee9dcc9c8fc11aa9159b as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
