FROM docker.elastic.co/wolfi/go:v1.26.3-r2@sha256:db78e76ef9276abcfde88f7faad8155904a732d204f9f4e83427b3077d8185d4 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
