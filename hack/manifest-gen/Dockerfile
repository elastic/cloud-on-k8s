FROM docker.elastic.co/wolfi/go:v1.27.1-r0@sha256:3a96e56089a9fd563aa6b03cae24c8aaf909cc3f8f059e4faec20082bedc6e53 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
