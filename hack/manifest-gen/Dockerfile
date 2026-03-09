FROM docker.elastic.co/wolfi/go:1.26.1-r0@sha256:429c3e478fba2a1dd85a4648a7c67af2c42b41af7c8961591a4bed4c74235d9c as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
