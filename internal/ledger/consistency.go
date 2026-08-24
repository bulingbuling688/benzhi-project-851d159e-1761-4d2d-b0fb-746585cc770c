package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// validateSnapshotPrefix 将快照视作可重建的缓存，但仍校验其声明位置处的内容。
// 快照可以落后于事件账本；这代表事件已 Sync 而快照原子替换尚未完成的可恢复崩溃点。
func validateSnapshotPrefix(path string, snap *projectionSnapshot, records []eventRecord) error {
	if snap == nil {
		return nil
	}
	if snap.LastSequence < 0 || snap.LastSequence > int64(len(records)) {
		return &CorruptionError{Path: path, Sequence: snap.LastSequence, Reason: "快照序号超出事件账本范围"}
	}
	if snap.LastSequence == 0 {
		if snap.LastHash != "" || len(snap.Cases) != 0 || len(snap.Idempotency) != 0 {
			return &CorruptionError{Path: path, Reason: "空位置快照包含非空投影"}
		}
		return nil
	}
	if records[snap.LastSequence-1].EventHash != snap.LastHash {
		return &CorruptionError{Path: path, Sequence: snap.LastSequence, Reason: "快照哈希不对应事件账本前缀"}
	}
	prefixCases, prefixIdempotency, _, _ := replay(records[:snap.LastSequence])
	if err := compareJSONProjection(prefixCases, snap.Cases); err != nil {
		return &CorruptionError{Path: path, Sequence: snap.LastSequence, Reason: "cases 投影不一致: " + err.Error()}
	}
	if err := compareJSONProjection(prefixIdempotency, snap.Idempotency); err != nil {
		return &CorruptionError{Path: path, Sequence: snap.LastSequence, Reason: "idempotency 投影不一致: " + err.Error()}
	}
	return nil
}

func compareJSONProjection(expected, actual any) error {
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return fmt.Errorf("编码期望投影: %w", err)
	}
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		return fmt.Errorf("编码实际投影: %w", err)
	}
	if !bytes.Equal(expectedJSON, actualJSON) {
		return fmt.Errorf("内容与账本重放结果不同")
	}
	return nil
}
