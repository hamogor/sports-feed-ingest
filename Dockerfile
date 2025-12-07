# Dockerfile at: /home/oref/GolandProjects/cortex-task/Dockerfile

# 1. Build the Go binary
FROM golang:1.24 AS builder

WORKDIR /app

# Copy go module files and download deps first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# TODO: change ./cmd/server to the actual path of your main package
# e.g. ./cmd/api or ./cmd/news-sync or ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/news-sync ./cmd/news-sync

# 2. Small runtime image
FROM gcr.io/distroless/base-debian12

WORKDIR /app

COPY --from=builder /app/news-sync /app/news-sync

# Expose health endpoint port (matches your compose: 8080:8080)
EXPOSE 8080

ENTRYPOINT ["/app/news-sync"]