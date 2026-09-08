package service

import (
	"context"
	"testing"
	"time"

	"gochen/errors"
	runtime "gochen/process/task"
)

func TestProviderManager_runHealthCheckOnce_nilCtx_doesNotPanic(t *testing.T) {
	pm, err := NewProviderManager(nil, nil)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}
	impl, ok := pm.(*providerManagerImpl)
	if !ok || impl == nil {
		t.Fatalf("unexpected provider manager type: %T", pm)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("runHealthCheckOnce panicked: %v", r)
		}
	}()

	impl.runHealthCheckOnce(nil)
}

func TestProviderManagerStopNilCtxReturnsInvalidInput(t *testing.T) {
	pm, err := NewProviderManager(nil, nil)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}
	err = pm.Stop(nil)
	if err == nil || !errors.Is(err, errors.InvalidInput) {
		t.Fatalf("expected invalid input for nil stop ctx, got %v", err)
	}
}

func TestProviderManagerStopTimeoutCleansStateForRepeatedStop(t *testing.T) {
	pm, err := NewProviderManager(nil, nil)
	if err != nil {
		t.Fatalf("NewProviderManager: %v", err)
	}
	impl := pm.(*providerManagerImpl)
	release := make(chan struct{})
	super := runtime.NewTaskSupervisor("test.provider_manager")
	if err := super.Go(context.Background(), "blocked", func(ctx context.Context) {
		<-release
	}); err != nil {
		t.Fatalf("start blocked task: %v", err)
	}
	defer close(release)

	impl.lifecycleMu.Lock()
	impl.started = true
	impl.stopped = false
	impl.super = super
	impl.cancel = func() {}
	impl.lifecycleMu.Unlock()

	stopCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := pm.Stop(stopCtx); err == nil || !errors.Is(err, errors.Timeout) {
		t.Fatalf("expected timeout on first stop, got %v", err)
	}
	if err := pm.Stop(context.Background()); err != nil {
		t.Fatalf("expected repeated stop after timeout to be idempotent, got %v", err)
	}
}
