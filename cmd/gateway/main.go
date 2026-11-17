// Copyright (c) 2026 BlueSkyXN
// SPDX-License-Identifier: GPL-3.0-only

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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
	version := flag.Bool("version", false, "输出版本、目录摘要与指南摘要")
	renderSkill := flag.Bool("render-skill", false, "输出随仓库发布的 feishu-gateway SKILL.md")
	explainPath := flag.String("explain-request", "", "从 JSON headers 文件解释身份与路由，不调用飞书或启动后端")
	flag.Parse()
	if *catalogFlag {
		fmt.Println(gateway.CatalogRevision())
		return nil
	}
	if *version {
		fmt.Printf("version=%s\ncatalog_revision=%s\nguidance_revision=%s\n", gateway.Version, gateway.CatalogRevision(), gateway.GuidanceRevision())
		return nil
	}
	if *renderSkill {
		fmt.Print(gateway.RenderSkill())
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
	if *explainPath != "" {
		return explainRequest(c, *explainPath)
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
		guidance, err := gateway.LoadGuidance(cat)
		if err != nil {
			return err
		}
		fmt.Printf("configuration valid; operations=%d; guided_operations=%d; guides=%d; source=%s\n", len(cat.Operations), len(guidance.Operations), len(guidance.Skills), cat.SourceCommit)
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

func explainRequest(c gateway.Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return err
	}
	if len(b) > 64<<10 {
		return errors.New("请求示例超过 64 KiB")
	}
	var input struct {
		Headers map[string]json.RawMessage `json:"headers"`
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return errors.New("请求示例需要 JSON 对象，headers 的值为字符串或字符串数组")
	}
	headers := http.Header{}
	for name, raw := range input.Headers {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			headers[name] = []string{value}
			continue
		}
		var values []string
		if err := json.Unmarshal(raw, &values); err != nil {
			return errors.New("headers 的值必须为字符串或字符串数组")
		}
		headers[name] = values
	}
	result, err := c.ExplainRequest(headers)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
