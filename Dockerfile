FROM golang:1.24-alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bot ./cmd/bot/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata py3-pip && \
    pip3 install --break-system-packages edge-tts && \
    apk del py3-pip && \
    rm -rf /root/.cache
WORKDIR /app
COPY --from=builder /bot .
COPY config.example.yaml ./config.yaml
VOLUME ["/app/data"]
ENV TZ=Europe/Moscow
EXPOSE 11112
RUN adduser -D -u 1001 botuser && mkdir -p /app/data && chown botuser:botuser /app/data
USER botuser
CMD ["./bot"]
