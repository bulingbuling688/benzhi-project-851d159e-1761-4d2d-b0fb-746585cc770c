package main

import (
	"io"
	"testing"
)

func TestAddressPrecedence(t *testing.T) {
	cfg, err := parseConfig(nil, func(key string) string {
		if key == "PORT" {
			return "19444"
		}
		return ""
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:19444" {
		t.Fatalf("address=%s", cfg.Address)
	}
	cfg, err = parseConfig([]string{"-addr=127.0.0.1:19555"}, func(string) string { return "19444" }, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:19555" {
		t.Fatalf("-addr 未优先: %s", cfg.Address)
	}
}
func TestInvalidAddressAndPort(t *testing.T) {
	cases := [][]string{{"-addr=0.0.0.0:19081"}, {"-addr=127.0.0.1:8080x"}, {"-selfcheck-timeout=0s"}}
	for _, args := range cases {
		if _, err := parseConfig(args, func(string) string { return "" }, io.Discard); err == nil {
			t.Errorf("参数应失败: %v", args)
		}
	}
	if _, err := parseConfig(nil, func(string) string { return "70000" }, io.Discard); err == nil {
		t.Fatal("无效 PORT 应失败")
	}
}
