package credential_number_restart_collision_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
)

type persistentRepository struct {
	cases map[string]*domain.ReleaseCase
}

func (r *persistentRepository) Commit(_ context.Context, req application.CommitRequest, mutate application.Mutator) (application.CommitResult, error) {
	current := r.cases[req.CaseID]
	if req.ExpectedVersion != nil && current.Version != *req.ExpectedVersion {
		return application.CommitResult{}, fmt.Errorf("version mismatch")
	}
	next, response, err := mutate(current)
	if err != nil {
		return application.CommitResult{}, err
	}
	r.cases[req.CaseID] = next
	raw, err := json.Marshal(response)
	return application.CommitResult{Response: raw, Version: next.Version}, err
}

func (r *persistentRepository) Get(_ context.Context, id string) (*domain.ReleaseCase, error) {
	return r.cases[id], nil
}

func (*persistentRepository) Timeline(context.Context, string, ...application.TimelineQuery) (application.TimelinePage, error) {
	return application.TimelinePage{}, nil
}

func frozenCase(t *testing.T, caseID string, now time.Time) *domain.ReleaseCase {
	t.Helper()
	revision, err := domain.NewRevision("rev-"+caseID, caseID, "", "engineer", 1, []domain.Artifact{{Path: "result.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 10}}, now)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.NewCase(caseID, "待签发档案", "B-1", "CGCS2000", "范围", revision, now)
	if err != nil {
		t.Fatal(err)
	}
	evidence := domain.CalibrationEvidence{Reference: "CAL-1", Instrument: "GNSS", CalibratedAt: now, CertificateHash: strings.Repeat("b", 64)}
	if err = c.SubmitCalibration(99, 10, 10, evidence, now); err != nil {
		t.Fatal(err)
	}
	current, _ := c.CurrentRevision()
	batch, err := domain.BuildValidationBatch("batch-"+caseID, *current, nil, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyValidation(batch, nil, now); err != nil {
		t.Fatal(err)
	}
	readiness := c.ReviewReadinessAt(now)
	if err = c.Review("decision-"+caseID, true, "批准", "reviewer", readiness, now); err != nil {
		t.Fatal(err)
	}
	if err = c.Freeze("manager", now); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCredentialNumberRemainsUniqueAcrossServiceRestart(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	repo := &persistentRepository{cases: map[string]*domain.ReleaseCase{
		"case-1": frozenCase(t, "case-1", now),
		"case-2": frozenCase(t, "case-2", now),
	}}
	issue := func(svc *application.Service, caseID, key string) *domain.ReleaseCredential {
		version := repo.cases[caseID].Version
		raw, _, err := svc.Issue(context.Background(), caseID, application.CommandMeta{
			Actor: application.Actor{Name: "manager", Role: application.RoleReleaseManager}, IdempotencyKey: key, ExpectedVersion: &version,
		})
		if err != nil {
			t.Fatal(err)
		}
		response, err := application.DecodeResponse[application.CredentialResponse](raw)
		if err != nil {
			t.Fatal(err)
		}
		return response.Credential
	}
	firstService := application.NewService(repo, []byte("0123456789abcdef"), func() time.Time { return now })
	first := issue(firstService, "case-1", "issue-1")
	secondService := application.NewService(repo, []byte("0123456789abcdef"), func() time.Time { return now })
	second := issue(secondService, "case-2", "issue-2")
	if first.CredentialNumber == second.CredentialNumber {
		t.Fatalf("服务重启后签发了重复 credentialNumber %q", first.CredentialNumber)
	}
}
