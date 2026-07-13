FROM docker.elastic.co/wolfi/go:v1.26.5-r0@sha256:96cbc5f88a7bce36a04774edfc1ee0f9041166cd6981b928b5f25a42b1ffb56b as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
