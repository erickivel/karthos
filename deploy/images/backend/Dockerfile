FROM golang:1.26.3-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/backend ./cmd/backend
COPY pkg ./pkg
RUN go build -o /bin/backend ./cmd/backend

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
USER appuser
COPY --from=builder /bin/backend /backend
EXPOSE 4317 4318 8080 13133
ENTRYPOINT ["/backend"]
