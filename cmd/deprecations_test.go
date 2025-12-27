package cmd

import "testing"

func TestDeprecationsLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	deprecationsCache = nil

	store := newDeprecationStore()
	store.setCustomer("Acme")
	store.setProject("Beta", "Legacy")
	store.setProject("Beta", "Legacy") // idempotent

	if err := saveDeprecations(store); err != nil {
		t.Fatalf("save deprecations: %v", err)
	}

	loaded, err := loadDeprecations()
	if err != nil {
		t.Fatalf("load deprecations: %v", err)
	}

	if !loaded.isCustomerDeprecated("Acme") {
		t.Fatalf("expected Acme deprecated")
	}
	if loaded.isCustomerDeprecated("Beta") {
		t.Fatalf("did not expect Beta customer deprecated")
	}
	if !loaded.isProjectDeprecated("Beta", "Legacy") {
		t.Fatalf("expected Beta/Legacy deprecated")
	}
	if loaded.isProjectDeprecated("Beta", "Modern") {
		t.Fatalf("did not expect Beta/Modern deprecated")
	}

	loaded.removeCustomer("Acme")
	loaded.removeProject("Beta", "Legacy")
	if err := saveDeprecations(loaded); err != nil {
		t.Fatalf("save deprecations after remove: %v", err)
	}

	loaded, err = loadDeprecations()
	if err != nil {
		t.Fatalf("reload deprecations: %v", err)
	}
	if loaded.isCustomerDeprecated("Acme") {
		t.Fatalf("expected Acme active after removal")
	}
	if loaded.isProjectDeprecated("Beta", "Legacy") {
		t.Fatalf("expected Beta/Legacy active after removal")
	}
}
