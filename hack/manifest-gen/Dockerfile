FROM docker.elastic.co/wolfi/go:v1.26.3-r1@sha256:0597d5d3e2d6e0ae95f03336b8f59e3a3b562a2b374433fa9ebf1a1a6224e4f7 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
