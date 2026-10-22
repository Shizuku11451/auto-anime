FROM alpine:3.21
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
# 镜像不再随包 qBittorrent：BT 由程序内置的纯 Go 引擎（DOWNLOAD_MODE=native）处理。
RUN apk add --no-cache ca-certificates tini
WORKDIR /app
COPY auto-anime-${VERSION#v}-linux-${TARGETARCH} /app/auto-anime
COPY static /app/static
RUN chmod 755 /app/auto-anime \
    && chmod -R a+rX /app \
    && { echo "version=${VERSION}"; echo "commit=${COMMIT}"; } > /app/meta.env \
    && mkdir -p /data/downloads /data/bt
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 755 /usr/local/bin/docker-entrypoint.sh
ENV DOWNLOAD_MODE=native NATIVE_BT_PORT=6881
EXPOSE 8000
# BT 引擎默认使用 10 个连续端口（6881-6890），实际映射由部署配置的 ports 决定。
EXPOSE 6881/tcp 6881/udp
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8000/api/health || exit 1
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["./auto-anime"]
