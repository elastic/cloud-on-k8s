FROM docker.elastic.co/wolfi/go:1.25.2-r0@sha256:e7ae2baca94c7724545f8a9d2e1cdf2ddbcfa78871f26a7e96bb385f95f97a6d as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
