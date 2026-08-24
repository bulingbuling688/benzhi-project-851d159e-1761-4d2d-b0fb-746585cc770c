package httpapi

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"surveyrelease/internal/application"
	"surveyrelease/internal/ledger"
)

func TestRunSelfcheckOverActualListener(t *testing.T) {
	store, err := ledger.Open(ledger.Config{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server, err := Listen("127.0.0.1:0", New(application.NewService(store, nil, nil), logger))
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = RunSelfcheck(ctx, server.Address()); err != nil {
		t.Fatal(err)
	}
	if err = server.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-served; err != nil {
		t.Fatal(err)
	}
}
