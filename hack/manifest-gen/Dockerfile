FROM docker.elastic.co/wolfi/go:1.23.5-r1@sha256:ee60d538d2c3d849afcf1c00310cfd9d9409ffe2098d9da1e8db467cdc0ff199 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
