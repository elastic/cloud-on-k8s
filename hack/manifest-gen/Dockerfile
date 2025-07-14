FROM docker.elastic.co/wolfi/go:1.24.5-r1@sha256:69572d92fa88dabee21e7527b7f100af34e93740cfbce67a2d2d410540d7849d as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
