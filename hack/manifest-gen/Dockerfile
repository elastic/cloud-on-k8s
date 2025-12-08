FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:0399daf4c6cc8ae155ba8b5dbe17af20361fd08fda6a84c500f9db24ce74cbcc as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
