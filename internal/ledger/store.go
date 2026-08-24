package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
)

type Store struct {
	mu                   sync.RWMutex
	config               Config
	file                 *os.File
	cases                map[string]*domain.ReleaseCase
	idempotency          map[string]storedIdempotency
	events               []eventRecord
	lastSequence         int64
	lastHash             string
	verifiedHeadSequence int64
	verifiedHeadHash     string
}

func Open(config Config) (*Store, error) {
	config, err := config.validate()
	if err != nil {
		return nil, err
	}
	snap, err := readSnapshot(config.snapshotPath())
	if err != nil {
		return nil, err
	}
	records, err := readEvents(config.eventsPath())
	if err != nil {
		return nil, err
	}
	cases, idem, seq, hash := replay(records)
	if err := validateSnapshotPrefix(config.snapshotPath(), snap, records); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(config.eventsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("打开事件账本用于追加: %w", err)
	}
	return &Store{config: config, file: f, cases: cases, idempotency: idem, events: records, lastSequence: seq, lastHash: hash, verifiedHeadSequence: seq, verifiedHeadHash: hash}, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

func randomEventID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("evt_%x", b)
	}
	return "evt_" + hex.EncodeToString(b)
}

func clone(c *domain.ReleaseCase) (*domain.ReleaseCase, error) {
	if c == nil {
		return nil, nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var out domain.ReleaseCase
	if err = json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) Commit(ctx context.Context, req application.CommitRequest, mutate application.Mutator) (application.CommitResult, error) {
	select {
	case <-ctx.Done():
		return application.CommitResult{}, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return application.CommitResult{}, errors.New("账本已关闭")
	}
	if prior, ok := s.idempotency[req.IdempotencyKey]; ok {
		return application.CommitResult{Response: append(json.RawMessage(nil), prior.Response...), Version: prior.Version, Replayed: true}, nil
	}
	current := s.cases[req.CaseID]
	if current == nil && req.EventType != "case.created" {
		return application.CommitResult{}, &application.AppError{Kind: application.KindNotFound, Code: "case_not_found", Message: "档案不存在"}
	}
	if req.ExpectedVersion != nil {
		actual := int64(0)
		if current != nil {
			actual = current.Version
		}
		if *req.ExpectedVersion != actual {
			return application.CommitResult{}, &application.AppError{Kind: application.KindConflict, Code: "version_conflict", Message: fmt.Sprintf("版本冲突：expected=%d actual=%d", *req.ExpectedVersion, actual)}
		}
	}
	next, responseValue, err := mutate(current)
	if err != nil {
		return application.CommitResult{}, err
	}
	if next == nil {
		return application.CommitResult{}, errors.New("提交未产生聚合")
	}
	if err := next.ValidateIntegrity(); err != nil {
		return application.CommitResult{}, fmt.Errorf("聚合完整性检查失败: %w", err)
	}
	response, err := json.Marshal(responseValue)
	if err != nil {
		return application.CommitResult{}, fmt.Errorf("编码命令响应: %w", err)
	}
	command, err := json.Marshal(req.CommandPayload)
	if err != nil {
		return application.CommitResult{}, fmt.Errorf("编码命令载荷: %w", err)
	}
	nextCopy, err := clone(next)
	if err != nil {
		return application.CommitResult{}, err
	}
	rec := eventRecord{SchemaVersion: schemaVersion, Sequence: s.lastSequence + 1, EventID: randomEventID(), CaseID: req.CaseID, EventType: req.EventType, Actor: req.Actor, IdempotencyKey: req.IdempotencyKey, ExpectedVersion: req.ExpectedVersion, CommandPayload: command, Aggregate: nextCopy, Response: response, PreviousHash: s.lastHash, OccurredAt: req.OccurredAt.UTC()}
	rec.EventHash, err = calculateEventHash(rec)
	if err != nil {
		return application.CommitResult{}, err
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return application.CommitResult{}, err
	}
	line = append(line, '\n')
	if _, err = s.file.Write(line); err != nil {
		return application.CommitResult{}, fmt.Errorf("追加事件: %w", err)
	}
	if err = s.file.Sync(); err != nil {
		return application.CommitResult{}, fmt.Errorf("同步事件账本: %w", err)
	}
	s.cases[req.CaseID] = nextCopy
	stored := storedIdempotency{CaseID: req.CaseID, EventType: req.EventType, Response: response, Version: nextCopy.Version}
	s.idempotency[req.IdempotencyKey] = stored
	s.events = append(s.events, rec)
	s.lastSequence = rec.Sequence
	s.lastHash = rec.EventHash
	s.verifiedHeadSequence = rec.Sequence
	s.verifiedHeadHash = rec.EventHash
	snap := projectionSnapshot{LastSequence: s.lastSequence, LastHash: s.lastHash, Cases: s.cases, Idempotency: s.idempotency}
	if err = writeSnapshot(s.config.snapshotPath(), snap); err != nil {
		return application.CommitResult{}, err
	}
	return application.CommitResult{Response: response, Version: nextCopy.Version}, nil
}

func (s *Store) Get(ctx context.Context, id string) (*domain.ReleaseCase, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cases[id])
}

func (s *Store) Timeline(ctx context.Context, caseID string, queries ...application.TimelineQuery) (application.TimelinePage, error) {
	select {
	case <-ctx.Done():
		return application.TimelinePage{}, ctx.Err()
	default:
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	query := application.TimelineQuery{Limit: application.DefaultTimelineLimit}
	if len(queries) > 0 {
		query = queries[0]
	}
	if query.Limit == 0 {
		query.Limit = application.DefaultTimelineLimit
	}
	if err := s.verifyMemoryLedger(); err != nil {
		return application.TimelinePage{}, err
	}
	if query.AfterSequence > s.lastSequence {
		return application.TimelinePage{}, &application.AppError{Kind: application.KindValidation, Code: "invalid_after_sequence", Message: "afterSequence 不能超过当前账本头序号"}
	}
	page := application.TimelinePage{CaseID: caseID, Events: []application.AuditRecord{}, NextAfterSequence: query.AfterSequence, LedgerHeadSequence: s.lastSequence, LedgerHeadHash: s.lastHash}
	for _, r := range s.events {
		if r.Sequence <= query.AfterSequence || r.CaseID != caseID || (query.EventType != "" && r.EventType != query.EventType) {
			continue
		}
		if len(page.Events) == query.Limit {
			page.HasMore = true
			break
		}
		payload, _ := json.Marshal(map[string]any{"command": json.RawMessage(r.CommandPayload), "version": r.Aggregate.Version, "eventSummary": auditEventSummary(r)})
		page.Events = append(page.Events, application.AuditRecord{Sequence: r.Sequence, EventID: r.EventID, CaseID: r.CaseID, EventType: r.EventType, Actor: r.Actor, IdempotencyKey: r.IdempotencyKey, ExpectedVersion: r.ExpectedVersion, Payload: payload, PreviousHash: r.PreviousHash, EventHash: r.EventHash, OccurredAt: r.OccurredAt, HashVerified: true})
	}
	page.ReturnedCount = len(page.Events)
	if page.ReturnedCount > 0 {
		page.NextAfterSequence = page.Events[page.ReturnedCount-1].Sequence
	}
	return page, nil
}

func (s *Store) verifyMemoryLedger() error {
	if s.lastSequence != int64(len(s.events)) || s.verifiedHeadSequence != s.lastSequence || s.verifiedHeadHash != s.lastHash {
		return errors.New("内存账本头与已验证账本头不一致")
	}
	previous := ""
	for index, record := range s.events {
		if record.Sequence != int64(index+1) || record.PreviousHash != previous {
			return errors.New("内存账本序号或哈希链不一致")
		}
		calculated, err := calculateEventHash(record)
		if err != nil || calculated != record.EventHash {
			return errors.New("内存账本事件哈希校验失败")
		}
		previous = record.EventHash
	}
	if previous != s.lastHash {
		return errors.New("内存账本最终哈希不一致")
	}
	return nil
}

func auditEventSummary(record eventRecord) any {
	c := record.Aggregate
	switch record.EventType {
	case "case.created":
		return map[string]any{"revisionId": c.Revisions[0].ID, "revisionContentHash": c.Revisions[0].RevisionContentHash, "registrationSummary": c.Revisions[0].RegistrationSummary}
	case "calibration.submitted":
		revision, _ := c.CurrentRevision()
		return map[string]any{"revisionId": revision.ID, "calibrationEvidence": revision.CalibrationEvidence}
	case "validation.completed":
		batch, _ := c.LatestValidationBatch()
		return batch
	case "remediation.submitted":
		revision, _ := c.CurrentRevision()
		return map[string]any{"revisionId": revision.ID, "revisionDiff": revision.RevisionDiff, "blockerResolutionLinks": revision.BlockerResolutionLinks}
	case "review.approve", "review.reject":
		if len(c.ReviewDecisions) > 0 {
			return c.ReviewDecisions[len(c.ReviewDecisions)-1]
		}
	}
	return nil
}
