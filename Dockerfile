# ЭТАП 1: Сборка (Build)
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Копируем go.mod и go.sum из корня
COPY go.mod go.sum ./
RUN go mod download

# Копируем весь остальной проект
COPY . .

# Собираем бинарник. Указываем путь к папке bot относительно WORKDIR (/app)
RUN mkdir -p /build && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /build/bot ./bot


# ЭТАП 2: Финальный образ (Run)
FROM alpine:latest

RUN apk --no-cache add ca-certificates bash gnupg

WORKDIR /app

# Копируем скомпилированный бинарник
COPY --from=builder /build/bot /app/bot

# (Опционально) Если вашему боту нужен файл конфигурации .env, 
# вы можете скопировать его сюда или передавать через Docker volumes/environment variables:
# COPY bot/.env /app/.env

# Запускаем приложение
CMD ["/app/bot", "--skip"]