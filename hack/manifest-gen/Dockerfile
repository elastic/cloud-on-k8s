FROM docker.elastic.co/wolfi/go:v1.26.4-r0@sha256:eed8a0de25557e9d7f5c89f1b80d41d64eec4ef509339ff22f95b74192440778 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
