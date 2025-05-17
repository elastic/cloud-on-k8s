FROM docker.elastic.co/wolfi/go:1.24.3@sha256:c274f71b2fb3282fc4c9e9b55f3a2e9d7cc0e8991094322a19f9f72042f3050a as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
