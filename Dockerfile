FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN go mod download && CGO_ENABLED=0 go build -o /out/autoscaler ./...

# smol runtime
FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/autoscaler /usr/local/bin/autoscaler
ENTRYPOINT ["autoscaler"]