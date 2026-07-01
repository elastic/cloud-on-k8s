FROM docker.elastic.co/wolfi/go:v1.26.4-r1@sha256:d0a950f03ac0ea044c44d97bcb6c48aab20ced88cb9a4ba3a19dc814798b1a55 as builder
ADD . /manifest-gen
WORKDIR /manifest-gen
ENV GO111MODULE=on CGO_ENABLED=0 GOOS=linux 
RUN go build -a -ldflags '-w -s' -o manifest-gen .

FROM scratch
COPY --from=builder /manifest-gen/manifest-gen /manifest-gen
COPY --from=builder /manifest-gen/assets /assets
ENTRYPOINT ["/manifest-gen"]
