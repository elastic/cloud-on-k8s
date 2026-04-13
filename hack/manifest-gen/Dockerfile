FROM docker.elastic.co/wolfi/go:v1.26.2-r0@sha256:8ae760d4a7bb419d04944fe8c164de74a824e7374b359cd2f926716a6ebf9116 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
