package ledger

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct{ Directory string }

func (c Config) validate() (Config, error) {
	if c.Directory == "" {
		return Config{}, fmt.Errorf("数据目录不能为空")
	}
	abs, err := filepath.Abs(c.Directory)
	if err != nil {
		return Config{}, fmt.Errorf("解析数据目录: %w", err)
	}
	if err = os.MkdirAll(abs, 0o750); err != nil {
		return Config{}, fmt.Errorf("创建数据目录: %w", err)
	}
	c.Directory = abs
	return c, nil
}

func (c Config) eventsPath() string   { return filepath.Join(c.Directory, "events.jsonl") }
func (c Config) snapshotPath() string { return filepath.Join(c.Directory, "projection.json") }
