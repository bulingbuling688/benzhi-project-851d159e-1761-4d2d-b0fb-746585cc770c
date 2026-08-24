package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	Address               string
	DataDirectory         string
	Selfcheck             bool
	SelfcheckTimeout      time.Duration
	dataDirectoryExplicit bool
}

func parseConfig(args []string, getenv func(string) string, output io.Writer) (config, error) {
	fs := flag.NewFlagSet("survey-release", flag.ContinueOnError)
	fs.SetOutput(output)
	var cfg config
	fs.StringVar(&cfg.Address, "addr", defaultAddress, "HTTP 回环监听地址")
	fs.StringVar(&cfg.DataDirectory, "data-dir", "./survey-release-data", "事件账本与投影目录")
	fs.BoolVar(&cfg.Selfcheck, "selfcheck", false, "启动后执行完整 HTTP 自检并退出")
	fs.DurationVar(&cfg.SelfcheckTimeout, "selfcheck-timeout", 10*time.Second, "自检与关闭总超时")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("不接受位置参数: %s", strings.Join(fs.Args(), " "))
	}
	addrExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "addr" {
			addrExplicit = true
		}
		if f.Name == "data-dir" {
			cfg.dataDirectoryExplicit = true
		}
	})
	if !addrExplicit {
		if raw := strings.TrimSpace(getenv("PORT")); raw != "" {
			port, err := strconv.Atoi(raw)
			if err != nil || port < 1 || port > 65535 {
				return config{}, fmt.Errorf("PORT 必须是 1 到 65535 的十进制端口号")
			}
			cfg.Address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}
	if cfg.SelfcheckTimeout <= 0 {
		return config{}, fmt.Errorf("selfcheck-timeout 必须大于 0")
	}
	if err := validateAddress(cfg.Address); err != nil {
		return config{}, err
	}
	return cfg, nil
}

func validateAddress(addr string) error {
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("addr 必须是 host:port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("addr 端口必须在 1 到 65535 之间")
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("addr 只允许回环地址，收到 %q", host)
	}
	return nil
}

func environment(key string) string { return os.Getenv(key) }
