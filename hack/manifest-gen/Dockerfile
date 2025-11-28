FROM docker.elastic.co/wolfi/go:1.25.4-r0@sha256:e1a50918f981ff742f11120a027ed50410bfcaaa39ac0a1a9dec55bd60b52036 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
