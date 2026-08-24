package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"surveyrelease/internal/domain"
)

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
	CaseID    string          `json:"caseId"`
	EventType string          `json:"eventType"`
	Response  json.RawMessage `json:"response"`
	Version   int64           `json:"version"`
}

type projectionSnapshot struct {
	SchemaVersion int                            `json:"schemaVersion"`
	LastSequence  int64                          `json:"lastSequence"`
	LastHash      string                         `json:"lastHash"`
	Cases         map[string]*domain.ReleaseCase `json:"cases"`
	Idempotency   map[string]storedIdempotency   `json:"idempotency"`
	WrittenAt     time.Time                      `json:"writtenAt"`
}
