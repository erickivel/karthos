FROM golang:1.26.3-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN go build -o /bin/mesh-protocol-go main.go

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
USER appuser
COPY --from=builder /bin/mesh-protocol-go /mesh-protocol-go
EXPOSE 50051 9000
ENTRYPOINT ["/mesh-protocol-go"]
