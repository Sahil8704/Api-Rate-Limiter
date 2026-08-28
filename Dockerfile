# recipe file jo btati hai container kaise banana hai
# Step 1: Use official lightweight Go 1.26 compiler image (Updated Version)
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum main.go ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o rate-limiter-engine .

# Step 2: Use a clean, ultra-small alpine runtime for final delivery
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/rate-limiter-engine .
EXPOSE 8080
CMD ["./rate-limiter-engine"]