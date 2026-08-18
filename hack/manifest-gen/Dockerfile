FROM docker.elastic.co/wolfi/go:v1.26.6-r0@sha256:6f8f9a2cb0cae79af5ec66204a461e371600fa8cad338aa27eee5653e52281cd as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
