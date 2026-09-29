package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise a real upgrade from the shipped fork schema, including its own
// migrations whose ordinals overlap with the incoming upstream migrations.
func TestUpgradeFromFork403PreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatal(err)
	}
	names, err := os.ReadFile("testdata/migrations_v4.0.3.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Fields(string(names)) {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("baseline %s: %v", name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`UPDATE settings SET host='vpn.example', billing_enabled=1`,
		`INSERT INTO users(name, uuid, password, sub_token) VALUES ('Existing', 'old-uuid', 'password', 'old-token')`,
		`INSERT INTO tariff_plans(id,slug,name,price_rub,period_days,device_limit) VALUES (101,'existing','Existing plan',199,30,3)`,
		`INSERT INTO payment_orders(user_id,plan_id,amount_rub,status,created_at) VALUES (1,101,199,'paid',unixepoch())`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Opening twice also verifies migration idempotency for an upgraded installation.
	for i := range 2 {
		st, err := Open(path)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		set, err := st.GetSettings()
		if err != nil {
			st.Close()
			t.Fatal(err)
		}
		if set.Host != "vpn.example" || !set.BillingEnabled || set.AutoUpdateCron != "" {
			t.Errorf("settings changed: %+v", set)
		}
		u, err := st.GetUser(1)
		if err != nil {
			st.Close()
			t.Fatal(err)
		}
		if u.Name != "Existing" || u.SubToken != "old-token" || u.ExtraDevices != 0 {
			t.Errorf("user changed: %+v", u)
		}
		p, err := st.GetTariffPlan(101)
		if err != nil {
			st.Close()
			t.Fatal(err)
		}
		if p.PriceRub != 199 || p.DeviceLimit != 3 || p.DevicePrice != 0 {
			t.Errorf("plan changed: %+v", p)
		}
		o, err := st.GetPaymentOrder(1)
		if err != nil {
			st.Close()
			t.Fatal(err)
		}
		if o.Status != "paid" || o.AmountRub != 199 || o.Kind != "plan" {
			t.Errorf("order changed: %+v", o)
		}
		st.Close()
	}
}
