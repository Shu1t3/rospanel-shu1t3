package store

import "testing"

// Disable the revision trigger after warming the cache so these checks exercise
// the new setters' own invalidation, rather than the trigger's fallback.
func TestEnsureLegalPathInvalidatesSettingsCache(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	before, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if before.LegalPath != "" {
		t.Fatalf("initial legal path = %q", before.LegalPath)
	}
	if _, err := st.db.Exec(`DROP TRIGGER settings_rev_on_update`); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureLegalPath(func() string { return "legal-address" }); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if after.LegalPath != "legal-address" {
		t.Fatalf("legal path after save = %q, want legal-address", after.LegalPath)
	}
	if err := st.EnsureLegalPath(func() string { return "replacement" }); err != nil {
		t.Fatal(err)
	}
	again, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if again.LegalPath != after.LegalPath {
		t.Fatalf("existing legal path changed to %q", again.LegalPath)
	}
}

func TestTGMailingSwitchInvalidatesSettingsCache(t *testing.T) {
	t.Parallel()
	st := newStore(t)
	before, err := st.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`DROP TRIGGER settings_rev_on_update`); err != nil {
		t.Fatal(err)
	}
	for _, on := range []bool{!before.TGMailingSwitch, before.TGMailingSwitch} {
		if err := st.SetTGMailingSwitch(on); err != nil {
			t.Fatal(err)
		}
		after, err := st.GetSettings()
		if err != nil {
			t.Fatal(err)
		}
		if after.TGMailingSwitch != on {
			t.Fatalf("mailing switch after save = %v, want %v", after.TGMailingSwitch, on)
		}
	}
}
