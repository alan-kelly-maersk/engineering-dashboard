FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o engineering-dashboard

FROM alpine:3.20

RUN addgroup -S nonroot \
    && adduser -S nonroot -G nonroot

USER nonroot

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/engineering-dashboard .
COPY --from=builder /app/templates/ ./templates/
COPY --from=builder /app/repos.yaml .

EXPOSE 8080

ENTRYPOINT ["./engineering-dashboard"]
