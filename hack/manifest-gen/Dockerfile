FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:0ea7bf3561c70122cd0fb30fafbce3cc38d1a185751e72f557b51eca9d10f3fd as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
