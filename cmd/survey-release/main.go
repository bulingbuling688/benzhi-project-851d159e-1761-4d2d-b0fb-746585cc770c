package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"surveyrelease/internal/application"
	"surveyrelease/internal/httpapi"
	"surveyrelease/internal/ledger"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cfg, err := parseConfig(args, environment, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置错误：%v\n", err)
		return 2
	}
	cleanup := func() {}
	if cfg.Selfcheck && !cfg.dataDirectoryExplicit {
		dir, err := os.MkdirTemp("", "survey-release-selfcheck-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "自检数据目录创建失败：%v\n", err)
			return 3
		}
		cfg.DataDirectory = dir
		cleanup = func() { _ = os.RemoveAll(dir) }
	}
	defer cleanup()
	store, err := ledger.Open(ledger.Config{Directory: cfg.DataDirectory})
	if err != nil {
		fmt.Fprintf(os.Stderr, "账本恢复失败：%v\n", err)
		return 3
	}
	defer store.Close()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	service := application.NewService(store, nil, nil)
	handler := httpapi.New(service, logger)
	server, err := httpapi.Listen(cfg.Address, handler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "监听失败：%v\n", err)
		return 4
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve() }()
	logger.Info("服务已启动", "address", server.Address(), "dataDirectory", cfg.DataDirectory, "selfcheck", cfg.Selfcheck)
	if cfg.Selfcheck {
		return runSelfcheck(cfg, server, serveResult, logger)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case sig := <-signals:
		logger.Info("收到关闭信号", "signal", sig.String())
	case err := <-serveResult:
		if err != nil {
			fmt.Fprintf(os.Stderr, "HTTP 服务失败：%v\n", err)
			return 5
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "优雅关闭失败：%v\n", err)
		return 5
	}
	if err := <-serveResult; err != nil {
		fmt.Fprintf(os.Stderr, "HTTP 服务关闭失败：%v\n", err)
		return 5
	}
	return 0
}

func runSelfcheck(cfg config, server *httpapi.Server, serveResult <-chan error, logger *slog.Logger) int {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.SelfcheckTimeout)
	defer cancel()
	checkResult := make(chan error, 1)
	go func() { checkResult <- httpapi.RunSelfcheck(ctx, server.Address()) }()
	var checkErr error
	select {
	case checkErr = <-checkResult:
	case err := <-serveResult:
		if err == nil {
			err = errors.New("HTTP 服务在自检前意外停止")
		}
		checkErr = err
	case <-ctx.Done():
		checkErr = fmt.Errorf("自检超时: %w", ctx.Err())
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	if checkErr != nil {
		fmt.Fprintf(os.Stderr, "自检失败：%v\n", checkErr)
		return 6
	}
	if shutdownErr != nil {
		fmt.Fprintf(os.Stderr, "自检关闭失败：%v\n", shutdownErr)
		return 6
	}
	select {
	case err := <-serveResult:
		if err != nil {
			fmt.Fprintf(os.Stderr, "自检服务退出失败：%v\n", err)
			return 6
		}
	case <-shutdownCtx.Done():
		fmt.Fprintln(os.Stderr, "自检服务未在关闭时限内退出")
		return 6
	}
	logger.Info("完整公开放行链路自检通过")
	return 0
}
