package timeline_cancellation_masked_test

import (
	"context"
	"errors"
	"testing"

	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
)

type cancelingRepository struct {
	cancel context.CancelFunc
}

func (r *cancelingRepository) Commit(context.Context, application.CommitRequest, application.Mutator) (application.CommitResult, error) {
	panic("unexpected Commit")
}

func (r *cancelingRepository) Get(ctx context.Context, _ string) (*domain.ReleaseCase, error) {
	return nil, ctx.Err()
}

func (r *cancelingRepository) Timeline(context.Context, string, ...application.TimelineQuery) (application.TimelinePage, error) {
	r.cancel()
	return application.TimelinePage{}, nil
}

func TestTimelinePreservesCancellationFromExistenceCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := application.NewService(&cancelingRepository{cancel: cancel}, nil, nil)
	_, err := svc.Timeline(ctx, "case-1", application.TimelineQuery{Limit: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("时间线存在性检查应传播 context.Canceled，实际为 %v", err)
	}
}
