FROM docker.elastic.co/wolfi/go:1.25.3-r0@sha256:0bcdfb1d2aa9a3c5dcdcd0b825fce734c9287de4f785b5a29e285f7d098e8dab as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
