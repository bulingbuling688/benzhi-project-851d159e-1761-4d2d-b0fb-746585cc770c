package application

import (
	"context"
	"encoding/json"
	"time"

	"surveyrelease/internal/domain"
)

type CommitRequest struct {
	CaseID          string
	EventType       string
	Actor           string
	IdempotencyKey  string
	ExpectedVersion *int64
	CommandPayload  any
	OccurredAt      time.Time
}

type CommitResult struct {
	Response json.RawMessage
	Version  int64
	Replayed bool
}

type Mutator func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error)

type AuditRecord struct {
	Sequence        int64           `json:"sequence"`
	EventID         string          `json:"eventId"`
	CaseID          string          `json:"caseId"`
	EventType       string          `json:"eventType"`
	Actor           string          `json:"actor"`
	IdempotencyKey  string          `json:"idempotencyKey"`
	ExpectedVersion *int64          `json:"expectedVersion,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	PreviousHash    string          `json:"previousHash"`
	EventHash       string          `json:"eventHash"`
	OccurredAt      time.Time       `json:"occurredAt"`
	HashVerified    bool            `json:"hashVerified"`
}

type TimelineQuery struct {
	AfterSequence int64
	Limit         int
	EventType     string
}

type TimelinePage struct {
	CaseID             string        `json:"caseId"`
	Events             []AuditRecord `json:"events"`
	ReturnedCount      int           `json:"returnedCount"`
	NextAfterSequence  int64         `json:"nextAfterSequence"`
	HasMore            bool          `json:"hasMore"`
	LedgerHeadSequence int64         `json:"ledgerHeadSequence"`
	LedgerHeadHash     string        `json:"ledgerHeadHash"`
}

type Repository interface {
	Commit(context.Context, CommitRequest, Mutator) (CommitResult, error)
	Get(context.Context, string) (*domain.ReleaseCase, error)
	Timeline(context.Context, string, ...TimelineQuery) (TimelinePage, error)
}
