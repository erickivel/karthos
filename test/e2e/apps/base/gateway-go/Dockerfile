FROM golang:1.26.0-alpine AS builder

WORKDIR /src
COPY main.go .
RUN go build -o /bin/gateway-go main.go

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
USER appuser
COPY --from=builder /bin/gateway-go /gateway-go
EXPOSE 8080
ENTRYPOINT ["/gateway-go"]
