package wrapped_app_error_misclassified_test

import (
	"context"
	"fmt"
	"testing"

	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
)

type wrappingRepository struct{}

func (*wrappingRepository) Commit(context.Context, application.CommitRequest, application.Mutator) (application.CommitResult, error) {
	panic("unexpected Commit")
}

func (*wrappingRepository) Get(context.Context, string) (*domain.ReleaseCase, error) {
	cause := &application.AppError{Kind: application.KindNotFound, Code: "case_not_found", Message: "档案不存在"}
	return nil, fmt.Errorf("读取投影: %w", cause)
}

func (*wrappingRepository) Timeline(context.Context, string, ...application.TimelineQuery) (application.TimelinePage, error) {
	panic("unexpected Timeline")
}

func TestWrappedAppErrorKeepsPublicClassification(t *testing.T) {
	svc := application.NewService(&wrappingRepository{}, nil, nil)
	_, err := svc.GetCase(context.Background(), "case-1")
	classified := application.Classify(err)
	if classified.Kind != application.KindNotFound || classified.Code != "case_not_found" {
		t.Fatalf("包装后的 AppError 应保留公开分类，实际 kind=%s code=%s", classified.Kind, classified.Code)
	}
}
