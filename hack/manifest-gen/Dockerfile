FROM docker.elastic.co/wolfi/go:v1.26.5-r2@sha256:e16da8fd50b0dd5c4c423594f42b8501241f2b878cdf4268bf42865a22d9a872 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
