FROM docker.elastic.co/wolfi/go:v1.26.5-r0@sha256:83ee7a170a530a0bed804f6e451fc41d51ea1887be3d57e5c1218e89a52d0f4e as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
