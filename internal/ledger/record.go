package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"surveyrelease/internal/domain"
)

// normalizePayload 将命令载荷的原始 JSON 归一化为稳定的字节表示。
// 通过解码为通用值再重新编码，使 map 键按字典序排列、空白保持一致，
// 从而确保语义相同的载荷在不同提交间产生相同的归一化字节序列。
func normalizePayload(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("null"), nil
	}
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("解码命令载荷: %w", err)
	}
	out, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("编码归一化载荷: %w", err)
	}
	return out, nil
}

// payloadHash 返回归一化命令载荷的稳定摘要，用于幂等冲突判断。
func payloadHash(raw json.RawMessage) (string, error) {
	normalized, err := normalizePayload(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(normalized)
	return hex.EncodeToString(sum[:]), nil
}

const schemaVersion = 1

type eventRecord struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Sequence        int64               `json:"sequence"`
	EventID         string              `json:"eventId"`
	CaseID          string              `json:"caseId"`
	EventType       string              `json:"eventType"`
	Actor           string              `json:"actor"`
	IdempotencyKey  string              `json:"idempotencyKey"`
	ExpectedVersion *int64              `json:"expectedVersion,omitempty"`
	CommandPayload  json.RawMessage     `json:"commandPayload"`
	Aggregate       *domain.ReleaseCase `json:"aggregate"`
	Response        json.RawMessage     `json:"response"`
	PreviousHash    string              `json:"previousHash"`
	EventHash       string              `json:"eventHash"`
	OccurredAt      time.Time           `json:"occurredAt"`
}

type hashMaterial struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	Sequence        int64               `json:"sequence"`
	EventID         string              `json:"eventId"`
	CaseID          string              `json:"caseId"`
	EventType       string              `json:"eventType"`
	Actor           string              `json:"actor"`
	IdempotencyKey  string              `json:"idempotencyKey"`
	ExpectedVersion *int64              `json:"expectedVersion,omitempty"`
	CommandPayload  json.RawMessage     `json:"commandPayload"`
	Aggregate       *domain.ReleaseCase `json:"aggregate"`
	Response        json.RawMessage     `json:"response"`
	PreviousHash    string              `json:"previousHash"`
	OccurredAt      time.Time           `json:"occurredAt"`
}

func calculateEventHash(r eventRecord) (string, error) {
	m := hashMaterial{r.SchemaVersion, r.Sequence, r.EventID, r.CaseID, r.EventType, r.Actor, r.IdempotencyKey, r.ExpectedVersion, r.CommandPayload, r.Aggregate, r.Response, r.PreviousHash, r.OccurredAt}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

type storedIdempotency struct {
	CaseID      string          `json:"caseId"`
	EventType   string          `json:"eventType"`
	PayloadHash string          `json:"payloadHash"`
	Response    json.RawMessage `json:"response"`
	Version     int64           `json:"version"`
}

type projectionSnapshot struct {
	SchemaVersion int                            `json:"schemaVersion"`
	LastSequence  int64                          `json:"lastSequence"`
	LastHash      string                         `json:"lastHash"`
	Cases         map[string]*domain.ReleaseCase `json:"cases"`
	Idempotency   map[string]storedIdempotency   `json:"idempotency"`
	WrittenAt     time.Time                      `json:"writtenAt"`
}
