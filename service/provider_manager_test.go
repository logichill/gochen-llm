package service

import "testing"

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
