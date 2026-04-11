FROM golang:1.25-alpine AS build
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/gameserver ./cmd/gameserver

FROM alpine:3.21
RUN apk upgrade --no-cache && adduser -D -u 10001 appuser
USER appuser
WORKDIR /app
COPY --from=build /bin/gameserver /usr/local/bin/gameserver
EXPOSE 8080
ENTRYPOINT ["gameserver"]
