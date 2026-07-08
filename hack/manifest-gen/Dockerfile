FROM docker.elastic.co/wolfi/go:v1.26.4-r1@sha256:8099808770c3bb4d32cc4e1be8b72e2db479c7ddad5ec55af85f84a5a793eb64 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
