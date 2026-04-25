FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY detector ./detector
RUN go test ./detector && mkdir -p /out && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/hng-detector ./detector

FROM alpine:3.20
RUN apk add --no-cache ca-certificates iptables && \
    addgroup -S detector && adduser -S detector -G detector && \
    mkdir -p /app /var/log/detector && chown -R detector:detector /app /var/log/detector
WORKDIR /app
COPY --from=build /out/hng-detector /app/hng-detector
COPY detector/config.yaml /app/config.yaml
EXPOSE 8081
ENTRYPOINT ["/app/hng-detector", "-config", "/app/config.yaml"]
