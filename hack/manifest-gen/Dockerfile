FROM docker.elastic.co/wolfi/go:1.25.5-r0@sha256:f81844bc5528ac4701e6c0713d7f3b3c004d18d5e5640d77501bcde30b4e0561 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
