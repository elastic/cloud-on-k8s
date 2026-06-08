FROM docker.elastic.co/wolfi/go:v1.26.4-r0@sha256:e91fba4f431cef46c8db5f3a74974029abea8df6f3fbf4bbccc9e8a71eaddec7 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
