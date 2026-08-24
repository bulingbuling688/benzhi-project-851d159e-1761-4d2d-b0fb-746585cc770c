package ledger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func writeSnapshot(path string, snap projectionSnapshot) error {
	snap.SchemaVersion = schemaVersion
	snap.WrittenAt = time.Now().UTC()
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("编码投影快照: %w", err)
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "projection-*.tmp")
	if err != nil {
		return fmt.Errorf("创建投影临时文件: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(0o640); err != nil {
		return err
	}
	if _, err = tmp.Write(b); err != nil {
		return fmt.Errorf("写入投影临时文件: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("同步投影临时文件: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("关闭投影临时文件: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("原子替换投影快照: %w", err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err == nil {
		err = dir.Sync()
		dir.Close()
	}
	if err != nil {
		return fmt.Errorf("同步数据目录: %w", err)
	}
	ok = true
	return nil
}
