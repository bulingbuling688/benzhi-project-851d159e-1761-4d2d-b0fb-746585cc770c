package idempotency_command_collision_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
	"surveyrelease/internal/ledger"
)

func TestIdempotencyKeyCannotReplayDifferentCommandPayload(t *testing.T) {
	now := time.Date(2026, 8, 25, 8, 0, 0, 0, time.UTC)
	store, err := ledger.Open(ledger.Config{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store, []byte("0123456789abcdef"), func() time.Time { return now })
	meta := application.CommandMeta{Actor: application.Actor{Name: "engineer", Role: application.RoleEngineer}, IdempotencyKey: "create-1"}
	raw, version, err := svc.CreateCase(context.Background(), meta, application.CreateCaseInput{
		Title: "幂等冲突检查", AcquisitionBatch: "B-1", CoordinateReference: "CGCS2000", ScopeDescription: "测试范围",
		Artifacts: []domain.Artifact{{Path: "result.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 10}},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := application.DecodeResponse[application.CaseResponse](raw)
	if err != nil {
		t.Fatal(err)
	}
	meta.IdempotencyKey = "calibration-reused"
	meta.ExpectedVersion = &version
	calibration := application.CalibrationInput{
		CoveragePercent: 99, HorizontalErrorCM: 10, VerticalErrorCM: 10,
		Evidence: application.CalibrationEvidenceInput{Reference: "CAL-1", Instrument: "GNSS", CalibratedAt: now, CertificateHash: strings.Repeat("b", 64)},
	}
	_, _, err = svc.SubmitCalibration(context.Background(), created.Case.ID, meta, calibration)
	if err != nil {
		t.Fatal(err)
	}
	calibration.HorizontalErrorCM = 19
	_, _, err = svc.SubmitCalibration(context.Background(), created.Case.ID, meta, calibration)
	if err == nil {
		t.Fatal("相同 Idempotency-Key 携带不同命令载荷时不应重放首次响应")
	}
}
