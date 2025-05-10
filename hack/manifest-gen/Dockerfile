FROM docker.elastic.co/wolfi/go:1.24.3@sha256:1f72b3107ebc956806f2b3d4916a926a08c9f97fa9459f2f3e0daf62e5ca5a73 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
