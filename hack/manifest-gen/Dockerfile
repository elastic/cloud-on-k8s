FROM docker.elastic.co/wolfi/go:v1.27.1-r0@sha256:1fbf86a1ffffb302e4bcd5f1fbd49bf0570786755f35e74885acda1cb769e0d7 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
