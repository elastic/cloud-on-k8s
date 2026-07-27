FROM docker.elastic.co/wolfi/go:v1.26.5-r1@sha256:f002c11166f4ca63eaaaa3479f5e42175083c7a63f0349ab09986ab8a5c1e8ba as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
