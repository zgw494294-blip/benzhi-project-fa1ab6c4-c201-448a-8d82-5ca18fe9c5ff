package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/store"
	"benzhi-project-fa1ab6c4-c201-448a-8d82-5ca18fe9c5ff/internal/transport"
)

func main() {
	addr := flag.String("addr", "", "HTTP 监听地址")
	selfcheck := flag.Bool("selfcheck", false, "执行端到端自检后退出")
	dataDir := flag.String("data-dir", ".data", "本地事件仓储目录")
	flag.Parse()
	listen := *addr
	if listen == "" {
		if port := os.Getenv("PORT"); port != "" {
			listen = "127.0.0.1:" + port
		} else {
			listen = "127.0.0.1:19081"
		}
	}
	if *selfcheck {
		if err := runSelfcheck(listen); err != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", err)
			os.Exit(1)
		}
		fmt.Println("自检通过：校准任务、测量判定、偏差整改、同行复核、证书签发和校验链路正常")
		return
	}
	s, err := store.Open(*dataDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	server := &http.Server{Addr: listen, Handler: transport.NewServer(s).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Printf("校准证书治理服务监听 %s，数据目录 %s\n", listen, filepath.Clean(*dataDir))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "服务退出:", err)
		os.Exit(1)
	}
}
