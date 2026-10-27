package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"auto-anime/internal/aggregate"
)

func main() {
	indexers := []string{}
	for _, id := range strings.Split(os.Getenv("JACKETT_INDEXERS"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			indexers = append(indexers, id)
		}
	}
	handler, err := aggregate.New(aggregate.Config{
		Token: os.Getenv("SEARCH_API_TOKEN"), JackettURL: os.Getenv("JACKETT_URL"),
		JackettKey: os.Getenv("JACKETT_API_KEY"), BangumiURL: os.Getenv("BANGUMI_API"), Indexers: indexers,
		Public: os.Getenv("SEARCH_PUBLIC") == "true",
	})
	if err != nil {
		log.Fatal(err)
	}
	address := os.Getenv("SEARCH_LISTEN")
	if address == "" {
		address = "127.0.0.1:8090"
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("聚合搜索服务启动，监听 %s，来源数量 %d", address, len(indexers))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("聚合搜索服务监听失败")
	}
}
