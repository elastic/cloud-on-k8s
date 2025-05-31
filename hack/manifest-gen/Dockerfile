FROM docker.elastic.co/wolfi/go:1.24.3-r1@sha256:18ee0bedf15f2c4cdc7f82ff30f260d9173abc2433680a2debfa69d77b7e804a as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
