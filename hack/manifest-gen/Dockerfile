FROM docker.elastic.co/wolfi/go:v1.26.2-r0@sha256:f35caccd7bcd5d324f28c9ab94146914e68c887bb1916cdc654a241c6bf2a481 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
