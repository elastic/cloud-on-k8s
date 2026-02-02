FROM docker.elastic.co/wolfi/go:1.25.6-r1@sha256:a9ca01fe231ee4f3720eb23f1e03662a04b0e5eaedd56651dcb9b3f8cb8078ac as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
