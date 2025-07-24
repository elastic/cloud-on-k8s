FROM docker.elastic.co/wolfi/go:1.24.5-r2@sha256:8b8d98d34d7c0892abead1f9be7cd3581587a556a26496553bb8b91e7520ae3e as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
