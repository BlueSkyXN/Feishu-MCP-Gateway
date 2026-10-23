// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BlueSkyXN/Feishu-MCP-Gateway/internal/gateway"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "config.local.json", "JSON 配置路径")
	check := flag.Bool("check", false, "验证配置、目录及固定 CLI 文件后退出")
	catalogFlag := flag.Bool("catalog-revision", false, "输出当前操作目录摘要")
	keygen := flag.Bool("generate-key", false, "仅向 stdout 输出新接入 key 和校验摘要，不写文件")
	flag.Parse()
	if *catalogFlag {
		fmt.Println(gateway.CatalogRevision())
		return nil
	}
	if *keygen {
		key := rand.Text()
		hash := sha256.Sum256([]byte(key))
		fmt.Printf("key=%s\nkey_sha256=%s\n", key, hex.EncodeToString(hash[:]))
		return nil
	}
	c, err := gateway.LoadConfig(*path)
	if err != nil {
		return err
	}
	executor, err := gateway.NewCLIExecutor(c)
	if err != nil {
		return err
	}
	if *check {
		cat, err := gateway.LoadCatalog()
		if err != nil {
			return err
		}
		fmt.Printf("configuration valid; operations=%d; source=%s\n", len(cat.Operations), cat.SourceCommit)
		return nil
	}
	service, err := gateway.NewService(c, executor)
	if err != nil {
		return err
	}
	defer service.Close()
	service.ConfigPath = *path
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	defer signal.Stop(reload)
	server := &http.Server{Addr: c.Listen, Handler: service.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: time.Duration(c.TimeoutSeconds+30) * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	result := make(chan error, 1)
	go func() {
		slog.Info("gateway listening", "address", c.Listen, "operations", len(service.Catalog.Operations))
		result <- server.ListenAndServe()
	}()
	cleanup := time.NewTicker(time.Minute)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdown); err != nil {
				_ = server.Close()
				return err
			}
			return nil
		case err := <-result:
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		case <-reload:
			next, err := gateway.LoadConfig(*path)
			if err == nil {
				err = service.Reload(next)
			}
			if err != nil {
				slog.Error("configuration reload rejected", "error", err)
			} else {
				slog.Info("configuration reloaded")
			}
		case <-cleanup.C:
			if err := service.Store.Collect(); err != nil {
				slog.Error("artifact cleanup failed", "error", err)
			}
		}
	}
}
