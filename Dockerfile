FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /auto-anime .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates aria2
WORKDIR /app
COPY --from=build /auto-anime .
COPY static ./static
RUN mkdir -p /data/downloads
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8000/api/health || exit 1
CMD ["./auto-anime"]
