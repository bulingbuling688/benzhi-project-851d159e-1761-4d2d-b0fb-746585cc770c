package ledger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"surveyrelease/internal/domain"
)

func readSnapshot(path string) (*projectionSnapshot, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取投影快照: %w", err)
	}
	var snap projectionSnapshot
	if err = json.Unmarshal(b, &snap); err != nil {
		return nil, &CorruptionError{Path: path, Reason: "投影快照不是有效 JSON: " + err.Error()}
	}
	if snap.SchemaVersion != schemaVersion {
		return nil, &SchemaError{Found: snap.SchemaVersion, Supported: schemaVersion, Source: "投影快照"}
	}
	if snap.Cases == nil || snap.Idempotency == nil {
		return nil, &CorruptionError{Path: path, Reason: "投影快照缺少 cases 或 idempotency"}
	}
	return &snap, nil
}

func readEvents(path string) ([]eventRecord, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []eventRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("打开事件账本: %w", err)
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	records := []eventRecord{}
	var previous string
	var sequence int64
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				return nil, &CorruptionError{Path: path, Sequence: sequence + 1, Reason: "末尾事件被截断"}
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				return nil, &CorruptionError{Path: path, Sequence: sequence + 1, Reason: "出现空事件行"}
			}
			var rec eventRecord
			if err = json.Unmarshal(line, &rec); err != nil {
				return nil, &CorruptionError{Path: path, Sequence: sequence + 1, Reason: "事件 JSON 无效: " + err.Error()}
			}
			if rec.SchemaVersion != schemaVersion {
				return nil, &SchemaError{Found: rec.SchemaVersion, Supported: schemaVersion, Source: "事件账本"}
			}
			sequence++
			if rec.Sequence != sequence {
				return nil, &CorruptionError{Path: path, Sequence: rec.Sequence, Reason: fmt.Sprintf("期望序号 %d", sequence)}
			}
			if rec.PreviousHash != previous {
				return nil, &CorruptionError{Path: path, Sequence: rec.Sequence, Reason: "previousHash 不匹配"}
			}
			calculated, err := calculateEventHash(rec)
			if err != nil {
				return nil, err
			}
			if rec.EventHash != calculated {
				return nil, &CorruptionError{Path: path, Sequence: rec.Sequence, Reason: "eventHash 不匹配"}
			}
			if rec.Aggregate == nil || rec.Aggregate.ID != rec.CaseID {
				return nil, &CorruptionError{Path: path, Sequence: rec.Sequence, Reason: "事件缺少有效聚合投影"}
			}
			if err := rec.Aggregate.ValidateIntegrity(); err != nil {
				return nil, &CorruptionError{Path: path, Sequence: rec.Sequence, Reason: "聚合完整性检查失败: " + err.Error()}
			}
			previous = rec.EventHash
			records = append(records, rec)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取事件账本: %w", readErr)
		}
	}
	return records, nil
}

func replay(records []eventRecord) (map[string]*domain.ReleaseCase, map[string]storedIdempotency, int64, string) {
	cases := map[string]*domain.ReleaseCase{}
	idem := map[string]storedIdempotency{}
	var seq int64
	var hash string
	for _, rec := range records {
		cases[rec.CaseID] = rec.Aggregate
		idem[rec.IdempotencyKey] = storedIdempotency{CaseID: rec.CaseID, EventType: rec.EventType, Response: rec.Response, Version: rec.Aggregate.Version}
		seq = rec.Sequence
		hash = rec.EventHash
	}
	return cases, idem, seq, hash
}
