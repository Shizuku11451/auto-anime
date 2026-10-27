FROM golang:1.23-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY backend/go.mod ./go.mod
COPY backend/cmd/search-server ./cmd/search-server
COPY backend/internal/aggregate ./internal/aggregate
COPY backend/internal/title ./internal/title
# This command and its shared parser use only the Go standard library.
RUN go test ./internal/aggregate ./internal/title && go build -trimpath -ldflags="-s -w" -o /search-server ./cmd/search-server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && addgroup -S search && adduser -S -G search search
COPY --from=build /search-server /usr/local/bin/search-server
USER search
ENV SEARCH_LISTEN=0.0.0.0:8090
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8090/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/search-server"]
