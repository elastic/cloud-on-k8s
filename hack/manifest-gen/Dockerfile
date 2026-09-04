FROM docker.elastic.co/wolfi/go:v1.27.1-r0@sha256:89065a5c117d1d2bdb065ac461d1cd775dff8a4f7e905bfafc4e2c4682738670 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
