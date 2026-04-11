FROM golang:1.22-alpine AS build
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/ws-router ./cmd/router

FROM alpine:3.21
RUN apk upgrade --no-cache && adduser -D -u 10001 appuser
USER appuser
WORKDIR /app
COPY --from=build /bin/ws-router /usr/local/bin/ws-router
EXPOSE 8082
ENTRYPOINT ["ws-router"]
