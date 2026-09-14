FROM docker.elastic.co/wolfi/go:v1.27.1-r0@sha256:9757715fe63f51fdf6799ccb4417c305c504d7b6ddc6ca1eb12061dd92bfebff as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
