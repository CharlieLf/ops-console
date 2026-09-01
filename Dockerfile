FROM node:22-alpine AS frontend
WORKDIR /fe
COPY frontend/package.json ./
RUN npm install --no-audit --no-fund
COPY frontend ./
RUN npm run build

FROM golang:1.23-alpine AS backend
WORKDIR /src
COPY backend ./
RUN go vet ./... && go test ./... \
  && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /ops-console .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget su-exec docker-cli docker-cli-compose \
  && adduser -D -H -u 1000 ops
WORKDIR /app
COPY --from=backend /ops-console /app/ops-console
COPY --from=frontend /fe/dist /app/static
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
RUN mkdir -p /app/data \
  && chown -R ops:ops /app \
  && sed -i 's/\r$//' /app/docker-entrypoint.sh \
  && chmod +x /app/docker-entrypoint.sh
ENV DATA_DIR=/app/data \
    STATIC_DIR=/app/static \
    HOST_STACKS_DIR=/opt/stacks \
    HOST_PROC=/host/proc \
    DOCKER_SOCKET=/var/run/docker.sock \
    LISTEN=:7070
EXPOSE 7070
HEALTHCHECK --interval=60s --timeout=5s --start-period=5s \
  CMD wget -qO- http://127.0.0.1:7070/api/health || exit 1
ENTRYPOINT ["/app/docker-entrypoint.sh"]
