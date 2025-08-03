FROM docker.elastic.co/wolfi/go:1.24.5-r2@sha256:cf5f05d2109c9e865625a3de6ff9cd34c44afe42c9cb03481b6f60ebe9b6fc54 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
