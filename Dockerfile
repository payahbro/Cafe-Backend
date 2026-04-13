FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api ./cmd/api

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/api /api
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]

