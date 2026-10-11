FROM golang:1.23-alpine AS build
WORKDIR /src
# 版本号由构建参数注入；容器内一键更新依赖 /app/meta.env 与 /data 里的版本记录
# 判断镜像版本，因此这里必须写下真实版本号而不是默认的 dev。
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ENV CGO_ENABLED=0
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/*.go .
# web_embed.go uses //go:embed, so the generated frontend must be present at compile time.
COPY backend/static ./static
RUN go build -trimpath -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" -o /auto-anime .
# 直接编译整个包（含测试文件）并跑静态检查，避免发布一个连测试都编译不过的镜像
RUN go vet ./...
FROM alpine:3.21
RUN apk add --no-cache ca-certificates qbittorrent-nox tini
WORKDIR /app
ARG VERSION=dev
ARG COMMIT=unknown
COPY --from=build /auto-anime .
COPY backend/static ./static
RUN { echo "version=${VERSION}"; echo "commit=${COMMIT}"; } > /app/meta.env
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 755 /usr/local/bin/docker-entrypoint.sh \
    && chmod -R a+rX /app \
    && mkdir -p /data/downloads /data/qbittorrent
ENV DOWNLOAD_MODE=builtin QBITTORRENT_DATA_DIR=/data/qbittorrent
EXPOSE 8000
EXPOSE 6881/tcp 6881/udp
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8000/api/health || exit 1
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["./auto-anime"]
