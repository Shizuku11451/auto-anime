FROM alpine:3.21
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN apk add --no-cache ca-certificates qbittorrent-nox tini
WORKDIR /app
COPY auto-anime-${VERSION#v}-linux-${TARGETARCH} /app/auto-anime
COPY static /app/static
RUN chmod 755 /app/auto-anime \
    && chmod -R a+rX /app \
    && { echo "version=${VERSION}"; echo "commit=${COMMIT}"; } > /app/meta.env \
    && mkdir -p /data/downloads /data/qbittorrent
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 755 /usr/local/bin/docker-entrypoint.sh
ENV DOWNLOAD_MODE=builtin QBITTORRENT_DATA_DIR=/data/qbittorrent
EXPOSE 8000
EXPOSE 6881/tcp 6881/udp
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8000/api/health || exit 1
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["./auto-anime"]
