FROM golang:1.23-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/*.go .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /auto-anime .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates qbittorrent-nox tini
WORKDIR /app
COPY --from=build /auto-anime .
COPY backend/static ./static
RUN mkdir -p /data/downloads /data/qbittorrent
ENV DOWNLOAD_MODE=builtin QBITTORRENT_DATA_DIR=/data/qbittorrent
EXPOSE 8000
EXPOSE 6881/tcp 6881/udp
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8000/api/health || exit 1
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./auto-anime"]
