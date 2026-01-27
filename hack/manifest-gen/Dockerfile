FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:8dd06d6e5de640aac12a318c77e486381ee122a7a281887c6f7ddf4ff1b4d56f as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
