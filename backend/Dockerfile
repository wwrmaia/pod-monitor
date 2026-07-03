FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -o api main.go

FROM alpine:3.19
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
COPY --from=builder /app/api .
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s CMD wget -qO- http://localhost:8080/healthz || exit 1
CMD ["./api"]
