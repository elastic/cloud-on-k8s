FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:95efe4c938d5a2081c41fc3500f6f2daa3a1efd837bc7e37610adeac5779e637 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
