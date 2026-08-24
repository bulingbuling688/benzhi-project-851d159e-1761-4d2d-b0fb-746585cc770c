package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"surveyrelease/internal/domain"
)

type Clock func() time.Time

type Service struct {
	repo             Repository
	clock            Clock
	ids              IDGenerator
	credentialSecret []byte
}

func NewService(repo Repository, secret []byte, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	if len(secret) < 16 {
		sum := sha256.Sum256([]byte("survey-release-local-credential-secret"))
		secret = sum[:]
	}
	return &Service{repo: repo, clock: clock, credentialSecret: append([]byte(nil), secret...)}
}

func validateMeta(meta CommandMeta, expectedRequired bool) error {
	if err := meta.Actor.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		return fail(KindValidation, "missing_idempotency_key", "Idempotency-Key 不能为空")
	}
	if len(meta.IdempotencyKey) > 128 {
		return fail(KindValidation, "invalid_idempotency_key", "Idempotency-Key 长度不能超过 128")
	}
	if expectedRequired && meta.ExpectedVersion == nil {
		return fail(KindValidation, "missing_expected_version", "If-Match 不能为空")
	}
	return nil
}

func cloneCase(c *domain.ReleaseCase) (*domain.ReleaseCase, error) {
	if c == nil {
		return nil, nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var copy domain.ReleaseCase
	if err := json.Unmarshal(b, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}

func (s *Service) commit(ctx context.Context, req CommitRequest, mutator Mutator) (json.RawMessage, int64, error) {
	r, err := s.repo.Commit(ctx, req, mutator)
	if err != nil {
		return nil, 0, normalizeError(err)
	}
	return r.Response, r.Version, nil
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*AppError); ok {
		return err
	}
	if d, ok := err.(*domain.RuleError); ok {
		kind := KindValidation
		if d.Code == "invalid_state" || d.Code == "open_blocker" || d.Code == "calibration_evidence_expired" || d.Code == "review_not_ready" {
			kind = KindState
		}
		if d.Code == "calibration_evidence_conflict" {
			kind = KindConflict
		}
		return &AppError{Kind: kind, Code: d.Code, Message: d.Message, Cause: err}
	}
	return &AppError{Kind: KindPersistence, Code: "persistence_error", Message: "持久化操作失败", Cause: err}
}

func (s *Service) CreateCase(ctx context.Context, meta CommandMeta, in CreateCaseInput) (json.RawMessage, int64, error) {
	if err := validateMeta(meta, false); err != nil {
		return nil, 0, err
	}
	if err := validateCreateInput(in); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleEngineer); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	caseID := s.ids.New("case", now)
	revisionID := s.ids.New("rev", now)
	req := CommitRequest{CaseID: caseID, EventType: "case.created", Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, CommandPayload: in, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		if current != nil {
			return nil, nil, fail(KindConflict, "case_exists", "档案已存在")
		}
		rev, err := domain.NewRevision(revisionID, caseID, "", meta.Actor.Name, 1, in.Artifacts, now)
		if err != nil {
			return nil, nil, err
		}
		c, err := domain.NewCase(caseID, in.Title, in.AcquisitionBatch, in.CoordinateReference, in.ScopeDescription, rev, now)
		if err != nil {
			return nil, nil, err
		}
		return c, CaseResponse{Case: c, Version: c.Version}, nil
	})
}

func (s *Service) SubmitCalibration(ctx context.Context, caseID string, meta CommandMeta, in CalibrationInput) (json.RawMessage, int64, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, 0, err
	}
	if err := validateMeta(meta, true); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleEngineer); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	if err := validateCalibrationInput(in, now); err != nil {
		return nil, 0, err
	}
	req := CommitRequest{CaseID: caseID, EventType: "calibration.submitted", Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, ExpectedVersion: meta.ExpectedVersion, CommandPayload: in, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		c, err := cloneCase(current)
		if err != nil || c == nil {
			if err == nil {
				err = fail(KindNotFound, "case_not_found", "档案不存在")
			}
			return nil, nil, err
		}
		if err := c.SubmitCalibration(in.CoveragePercent, in.HorizontalErrorCM, in.VerticalErrorCM, in.Evidence.DomainEvidence(), now); err != nil {
			return nil, nil, err
		}
		return c, CaseResponse{Case: c, Version: c.Version}, nil
	})
}

func (s *Service) Validate(ctx context.Context, caseID string, meta CommandMeta) (json.RawMessage, int64, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, 0, err
	}
	if err := validateMeta(meta, true); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleEngineer, RoleReviewer); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	batchID := s.ids.New("vbatch", now)
	req := CommitRequest{CaseID: caseID, EventType: "validation.completed", Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, ExpectedVersion: meta.ExpectedVersion, CommandPayload: map[string]string{"ruleVersion": RuleVersion}, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		c, err := cloneCase(current)
		if err != nil || c == nil {
			if err == nil {
				err = fail(KindNotFound, "case_not_found", "档案不存在")
			}
			return nil, nil, err
		}
		findings, err := EvaluateRevision(c, now)
		if err != nil {
			return nil, nil, err
		}
		revision, ok := c.CurrentRevision()
		if !ok {
			return nil, nil, fail(KindState, "missing_revision", "当前候选修订不存在")
		}
		batch, err := domain.BuildValidationBatch(batchID, *revision, findings, c.PreviousValidatedFindings(), now)
		if err != nil {
			return nil, nil, err
		}
		if err = c.ApplyValidation(batch, findings, now); err != nil {
			return nil, nil, err
		}
		return c, s.caseResponse(c, now), nil
	})
}

func (s *Service) Remediate(ctx context.Context, caseID string, meta CommandMeta, in RemediationInput) (json.RawMessage, int64, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, 0, err
	}
	if err := validateMeta(meta, true); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleEngineer); err != nil {
		return nil, 0, err
	}
	if err := validateRemediationInput(in); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	revisionID := s.ids.New("rev", now)
	req := CommitRequest{CaseID: caseID, EventType: "remediation.submitted", Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, ExpectedVersion: meta.ExpectedVersion, CommandPayload: in, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		c, err := cloneCase(current)
		if err != nil || c == nil {
			if err == nil {
				err = fail(KindNotFound, "case_not_found", "档案不存在")
			}
			return nil, nil, err
		}
		rev, err := domain.NewRevision(revisionID, c.ID, c.CurrentRevisionID, meta.Actor.Name, len(c.Revisions)+1, in.Artifacts, now)
		if err != nil {
			return nil, nil, err
		}
		if err = c.AddRemediation(rev, in.Resolutions, now); err != nil {
			return nil, nil, err
		}
		return c, CaseResponse{Case: c, Version: c.Version}, nil
	})
}

func (s *Service) Review(ctx context.Context, caseID string, meta CommandMeta, in ReviewInput) (json.RawMessage, int64, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, 0, err
	}
	if err := validateMeta(meta, true); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleReviewer); err != nil {
		return nil, 0, err
	}
	if err := validateReviewInput(in); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	decisionID := s.ids.New("decision", now)
	req := CommitRequest{CaseID: caseID, EventType: "review." + in.Decision, Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, ExpectedVersion: meta.ExpectedVersion, CommandPayload: in, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		c, err := cloneCase(current)
		if err != nil || c == nil {
			if err == nil {
				err = fail(KindNotFound, "case_not_found", "档案不存在")
			}
			return nil, nil, err
		}
		readiness := c.ReviewReadinessAt(now)
		if err = c.Review(decisionID, in.Decision == "approve", strings.TrimSpace(in.Reason), meta.Actor.Name, readiness, now); err != nil {
			return nil, nil, err
		}
		return c, CaseResponse{Case: c, Version: c.Version}, nil
	})
}

func (s *Service) Freeze(ctx context.Context, caseID string, meta CommandMeta) (json.RawMessage, int64, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, 0, err
	}
	if err := validateMeta(meta, true); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleReleaseManager); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	req := CommitRequest{CaseID: caseID, EventType: "case.frozen", Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, ExpectedVersion: meta.ExpectedVersion, CommandPayload: map[string]string{"caseId": caseID}, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		c, err := cloneCase(current)
		if err != nil || c == nil {
			if err == nil {
				err = fail(KindNotFound, "case_not_found", "档案不存在")
			}
			return nil, nil, err
		}
		if err = c.Freeze(meta.Actor.Name, now); err != nil {
			return nil, nil, err
		}
		return c, CaseResponse{Case: c, Version: c.Version}, nil
	})
}

func (s *Service) Issue(ctx context.Context, caseID string, meta CommandMeta) (json.RawMessage, int64, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, 0, err
	}
	if err := validateMeta(meta, true); err != nil {
		return nil, 0, err
	}
	if err := authorize(meta.Actor.Role, RoleReleaseManager); err != nil {
		return nil, 0, err
	}
	now := s.clock().UTC()
	credID := s.ids.New("cred", now)
	number := s.ids.CredentialNumber(now)
	req := CommitRequest{CaseID: caseID, EventType: "credential.issued", Actor: meta.Actor.Name, IdempotencyKey: meta.IdempotencyKey, ExpectedVersion: meta.ExpectedVersion, CommandPayload: map[string]string{"credentialNumber": number}, OccurredAt: now}
	return s.commit(ctx, req, func(current *domain.ReleaseCase) (*domain.ReleaseCase, any, error) {
		c, err := cloneCase(current)
		if err != nil || c == nil {
			if err == nil {
				err = fail(KindNotFound, "case_not_found", "档案不存在")
			}
			return nil, nil, err
		}
		cred, err := domain.NewCredential(credID, number, c, meta.Actor.Name, now, s.credentialSecret)
		if err != nil {
			return nil, nil, err
		}
		if err = c.Release(cred, now); err != nil {
			return nil, nil, err
		}
		return c, CredentialResponse{Credential: c.Credential, Version: c.Version}, nil
	})
}

func (s *Service) GetCase(ctx context.Context, caseID string) (*domain.ReleaseCase, error) {
	if err := validateCaseID(caseID); err != nil {
		return nil, err
	}
	c, err := s.repo.Get(ctx, caseID)
	if err != nil {
		return nil, normalizeError(err)
	}
	if c == nil {
		return nil, fail(KindNotFound, "case_not_found", "档案不存在")
	}
	return cloneCase(c)
}

func (s *Service) caseResponse(c *domain.ReleaseCase, now time.Time) CaseResponse {
	response := CaseResponse{Case: c, Version: c.Version}
	if c.Status == domain.StatusReviewing {
		readiness := c.ReviewReadinessAt(now)
		response.ReviewReadiness = &readiness
	}
	return response
}

func (s *Service) GetCaseResponse(ctx context.Context, caseID string) (CaseResponse, error) {
	c, err := s.GetCase(ctx, caseID)
	if err != nil {
		return CaseResponse{}, err
	}
	return s.caseResponse(c, s.clock().UTC()), nil
}
func (s *Service) Timeline(ctx context.Context, caseID string, query TimelineQuery) (TimelinePage, error) {
	if err := validateCaseID(caseID); err != nil {
		return TimelinePage{}, err
	}
	if query.Limit == 0 {
		query.Limit = DefaultTimelineLimit
	}
	if err := validateTimelineQuery(query); err != nil {
		return TimelinePage{}, err
	}
	page, err := s.repo.Timeline(ctx, caseID, query)
	if err != nil {
		return TimelinePage{}, normalizeError(err)
	}
	if page.ReturnedCount == 0 {
		if c, _ := s.repo.Get(ctx, caseID); c == nil {
			return TimelinePage{}, fail(KindNotFound, "case_not_found", "档案不存在")
		}
	}
	return page, nil
}
func (s *Service) Verify(ctx context.Context, caseID string, in VerifyInput) (VerificationResult, error) {
	if err := validateCaseID(caseID); err != nil {
		return VerificationResult{}, err
	}
	if err := validateVerifyInput(in); err != nil {
		return VerificationResult{}, err
	}
	c, err := s.GetCase(ctx, caseID)
	if err != nil {
		return VerificationResult{}, err
	}
	if c.Credential == nil {
		return VerificationResult{Valid: false, CaseID: caseID, Reason: "档案尚未签发凭据"}, nil
	}
	cred := c.Credential
	valid := cred.CredentialNumber == in.CredentialNumber && cred.VerificationCode == in.VerificationCode && cred.ManifestHash == in.ManifestHash && cred.Verify(s.credentialSecret, c.ManifestHash)
	result := VerificationResult{Valid: valid, CaseID: caseID, CredentialNumber: cred.CredentialNumber, ManifestHash: cred.ManifestHash, IssuedAt: cred.IssuedAt}
	if !valid {
		result.Reason = "凭据编号、清单哈希或校验码不匹配"
	}
	return result, nil
}

func DecodeResponse[T any](raw json.RawMessage) (T, error) {
	var value T
	err := json.Unmarshal(raw, &value)
	if err != nil {
		return value, fmt.Errorf("解析应用响应: %w", err)
	}
	return value, nil
}
