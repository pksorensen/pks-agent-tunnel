FROM golang:1.24-alpine AS builder
WORKDIR /workspace
# Server depends on src/protocol via a `replace ../protocol` directive in go.mod.
# Don't copy go.work — it references src/agent-tunnel which we don't ship in
# this image. GOWORK=off forces the build to use go.mod's replace directive.
ENV GOWORK=off
COPY src/protocol ./src/protocol
COPY src/agent-tunnel-server ./src/agent-tunnel-server
WORKDIR /workspace/src/agent-tunnel-server
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/agent-tunnel-server .

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/agent-tunnel-server /app/agent-tunnel-server
ENV USER_DATA_DIR=/data
ENV LISTEN_HTTP=:8080
ENV LISTEN_CONTROL=:7080
ENV TCP_POOL_MIN=10000
ENV TCP_POOL_MAX=19999
ENV AUTH_MODE=anonymous
EXPOSE 80 443 7443 8080 7080 10000-19999
VOLUME /data
ENTRYPOINT ["/app/agent-tunnel-server"]
