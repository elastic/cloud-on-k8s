FROM docker.elastic.co/wolfi/go:v1.26.4-r0@sha256:c202f194a5e7a5c0ed5625b03c4a335dd2cc16cc51fe888c7d96a3cf99affd42 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
