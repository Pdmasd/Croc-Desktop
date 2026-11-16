package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schollz/croc/v11/src/croc"
)

func TestValidateSourcePathsRejectsMissingAndSpecialPaths(t *testing.T) {
	if _, err := validateSourcePaths([]string{filepath.Join(t.TempDir(), "missing.txt")}); err == nil {
		t.Fatal("missing source path was accepted")
	}
	if _, err := validateSourcePaths([]string{""}); err == nil {
		t.Fatal("empty source path was accepted")
	}
}

func TestValidateSourcePathsNormalizesAndDeduplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := validateSourcePaths([]string{path, path})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != path {
		t.Fatalf("validated paths = %v", got)
	}
}

func TestTransferManagerAllowsOnlyOneActiveSessionAndCancels(t *testing.T) {
	manager := NewTransferManager(nil)
	finished := make(chan error, 1)
	_, err := manager.start("send", croc.Options{}, "secret-that-is-not-persisted", func(ctx context.Context, _ croc.Options) error {
		<-ctx.Done()
		finished <- ctx.Err()
		return ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.start("receive", croc.Options{}, "", func(context.Context, croc.Options) error { return nil }); err == nil {
		t.Fatal("second concurrent transfer was accepted")
	}
	if err := manager.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel result = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("transfer context did not cancel")
	}
	if got := manager.Snapshot().State; got != StateCancelled {
		t.Fatalf("state = %q, want cancelled", got)
	}
	if err := manager.Cancel(); err == nil {
		t.Fatal("cancel without an active transfer succeeded")
	}
}

func TestTransferProgressSnapshotAndRate(t *testing.T) {
	now := time.Now()
	manager := NewTransferManager(nil)
	manager.now = func() time.Time { return now }
	manager.active = &activeTransfer{event: TransferEvent{ID: "one", State: StateTransferring}, lastAt: now}
	manager.setProgress("one", 100, 1000)
	now = now.Add(2 * time.Second)
	manager.setProgress("one", 300, 1000)
	got := manager.Snapshot()
	if got.Transferred != 300 || got.Total != 1000 || got.BytesPerSecond != 100 {
		t.Fatalf("progress snapshot = %+v", got)
	}
}
