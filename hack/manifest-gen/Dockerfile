FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:80b1be6ff729f3ce108b7cfb9082e63bf7bede471a40c12c011e3c528ead000a as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
