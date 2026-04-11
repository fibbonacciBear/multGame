FROM golang:1.25-alpine AS build
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/apiserver ./cmd/api

FROM alpine:3.21
RUN apk upgrade --no-cache && adduser -D -u 10001 appuser
USER appuser
WORKDIR /app
COPY --from=build /bin/apiserver /usr/local/bin/apiserver
EXPOSE 8081
ENTRYPOINT ["apiserver"]
