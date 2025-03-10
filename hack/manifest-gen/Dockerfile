FROM docker.elastic.co/wolfi/go:1.24.1@sha256:34b1aefdf2b7fb655f558ece4ba4d8193dc4f257fc175f18c752c76c110b2d30 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
