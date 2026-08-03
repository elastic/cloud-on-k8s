FROM docker.elastic.co/wolfi/go:v1.26.5-r2@sha256:ea003ef5b4f713892c01e0b2fcab924bd3fdb70d44277cd62932459e60cd273a as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
