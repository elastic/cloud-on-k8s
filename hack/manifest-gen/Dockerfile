FROM docker.elastic.co/wolfi/go:1.25.4-r0@sha256:0447bc63c01761fd14bd45576d6a502bd1030aea313ee69fe78370e8dcccef73 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
