package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func deviceStore(t *testing.T) (*Store, int64) {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "dev.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.CreateUser("u1", "uuid-1", "pw", "tok", 0, 0, 0)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return st, u.ID
}

func dev(hwid string) model.Device {
	now := time.Now().Unix()
	return model.Device{HWID: hwid, OS: "android", LastSeen: now, IP: "1.2.3.4"}
}

func TestRegisterDeviceEnforcesCap(t *testing.T) {
	st, uid := deviceStore(t)

	for i, hwid := range []string{"a", "b"} {
		adm, err := st.RegisterDevice(uid, dev(hwid), 2)
		if err != nil {
			t.Fatalf("register %s: %v", hwid, err)
		}
		if !adm.Allowed || !adm.New || adm.Count != i+1 {
			t.Fatalf("register %s: %+v, want allowed new count=%d", hwid, adm, i+1)
		}
	}

	adm, err := st.RegisterDevice(uid, dev("c"), 2)
	if err != nil {
		t.Fatalf("register c: %v", err)
	}
	if adm.Allowed || adm.New {
		t.Errorf("third device admitted against a cap of 2: %+v", adm)
	}
	if adm.Count != 2 {
		t.Errorf("count = %d after refusal, want 2", adm.Count)
	}

	// A known device keeps refreshing even though the roster is full — the cap is on
	// how many devices exist, not on how often they fetch.
	adm, err = st.RegisterDevice(uid, dev("a"), 2)
	if err != nil {
		t.Fatalf("refresh a: %v", err)
	}
	if !adm.Allowed || adm.New {
		t.Errorf("refresh of a bound device: %+v, want allowed and not new", adm)
	}
}

// A refresh that carries only the id must not blank what the device already told
// us about itself: only x-hwid is required by the convention, so plenty of fetches
// arrive with nothing else, and a row that forgets it was an iPhone is a row the
// operator cannot act on.
func TestRegisterDeviceKeepsDescriptionOnBareRefresh(t *testing.T) {
	st, uid := deviceStore(t)

	full := dev("a")
	full.OS, full.OSVersion, full.Model, full.App = "iOS", "26.5.2", "iPhone 15 Pro Max", "Happ/1.0"
	if _, err := st.RegisterDevice(uid, full, 0); err != nil {
		t.Fatalf("register: %v", err)
	}

	bare := model.Device{HWID: "a", IP: "9.9.9.9", LastSeen: full.LastSeen + 60}
	if _, err := st.RegisterDevice(uid, bare, 0); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	list, err := st.ListDevices(uid)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, err %v", list, err)
	}
	got := list[0]
	if got.Model != "iPhone 15 Pro Max" || got.OS != "iOS" || got.OSVersion != "26.5.2" {
		t.Errorf("a bare refresh blanked the description: %+v", got)
	}
	// The address is the exception — it means "where it fetched from last".
	if got.IP != "9.9.9.9" {
		t.Errorf("ip = %q, want the address of the latest fetch", got.IP)
	}
}

func TestRegisterDeviceUnlimited(t *testing.T) {
	st, uid := deviceStore(t)
	for _, hwid := range []string{"a", "b", "c", "d"} {
		adm, err := st.RegisterDevice(uid, dev(hwid), 0)
		if err != nil {
			t.Fatalf("register %s: %v", hwid, err)
		}
		if !adm.Allowed {
			t.Fatalf("cap 0 refused %s: %+v", hwid, adm)
		}
	}
}

// The race this guards is the one Remnawave shipped (GHSA-985p-44h5-v3pq): a check
// and an insert in separate statements let two simultaneous fetches both take the
// last slot. The cap lives inside the INSERT here, so exactly cap devices survive
// however many clients arrive at once.
func TestRegisterDeviceConcurrentRespectsCap(t *testing.T) {
	st, uid := deviceStore(t)

	const cap, clients = 3, 12
	var wg sync.WaitGroup
	admitted := make([]bool, clients)
	start := make(chan struct{})
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			adm, err := st.RegisterDevice(uid, dev(string(rune('a'+i))), cap)
			if err != nil {
				t.Errorf("register %d: %v", i, err)
				return
			}
			admitted[i] = adm.Allowed
		}(i)
	}
	close(start)
	wg.Wait()

	got := 0
	for _, ok := range admitted {
		if ok {
			got++
		}
	}
	if got != cap {
		t.Errorf("%d clients admitted against a cap of %d", got, cap)
	}
	n, err := st.CountDevices(uid)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != cap {
		t.Errorf("%d rows stored, want %d", n, cap)
	}
}

func TestDeleteDeviceFreesSlot(t *testing.T) {
	st, uid := deviceStore(t)
	if _, err := st.RegisterDevice(uid, dev("a"), 1); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if adm, _ := st.RegisterDevice(uid, dev("b"), 1); adm.Allowed {
		t.Fatal("second device admitted against a cap of 1")
	}
	ok, err := st.DeleteDevice(uid, "a")
	if err != nil || !ok {
		t.Fatalf("delete a: ok=%v err=%v", ok, err)
	}
	adm, err := st.RegisterDevice(uid, dev("b"), 1)
	if err != nil {
		t.Fatalf("register b: %v", err)
	}
	if !adm.Allowed {
		t.Error("slot not freed by unbinding")
	}
	if ok, _ := st.DeleteDevice(uid, "nope"); ok {
		t.Error("deleting an unknown device reported a removal")
	}
}

func TestListAndPurgeDevices(t *testing.T) {
	st, uid := deviceStore(t)
	now := time.Now().Unix()
	fresh := dev("fresh")
	old := dev("old")
	old.LastSeen = now - 40*86400
	for _, d := range []model.Device{fresh, old} {
		if _, err := st.RegisterDevice(uid, d, 0); err != nil {
			t.Fatalf("register %s: %v", d.HWID, err)
		}
	}
	list, err := st.ListDevices(uid)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].HWID != "fresh" {
		t.Fatalf("list = %+v, want fresh first", list)
	}

	n, err := st.PurgeDevices(now - 30*86400)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d, want 1", n)
	}
	counts, err := st.DeviceCounts([]int64{uid})
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts[uid] != 1 {
		t.Errorf("counts = %v, want 1 device left", counts)
	}
}

func TestDeleteUserDropsDevices(t *testing.T) {
	st, uid := deviceStore(t)
	if _, err := st.RegisterDevice(uid, dev("a"), 0); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := st.DeleteUser(uid); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	n, err := st.CountDevices(uid)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("%d device rows outlived their user", n)
	}
}

// The device roster is written from an UNAUTHENTICATED subscription fetch carrying a
// client-supplied x-hwid, and "no limit" is the shipped default (hwid_fallback_limit
// starts at 0). Without a ceiling one token could insert a row per request forever.
func TestUnlimitedDeviceRosterStillHasACeiling(t *testing.T) {
	st, uid := deviceStore(t)
	for i := 0; i < maxDevicesPerUser+25; i++ {
		d := model.Device{HWID: fmt.Sprintf("hw-%d", i), LastSeen: 1700000000}
		if _, err := st.RegisterDevice(uid, d, 0); err != nil { // 0 = "unlimited"
			t.Fatalf("register %d: %v", i, err)
		}
	}
	n, err := st.CountDevices(uid)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n > maxDevicesPerUser {
		t.Errorf("roster grew to %d devices with no per-user limit, want at most %d", n, maxDevicesPerUser)
	}
}

func TestStampDeviceOverLimitChunking(t *testing.T) {
	st, _ := deviceStore(t)
	now := time.Now().Unix()

	// Insert 1050 users with device_limit = 1
	err := st.withTx(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`INSERT INTO users (name, uuid, password, sub_token, enabled, device_limit)
			VALUES (?, ?, ?, ?, 1, 1)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for i := 0; i < 1050; i++ {
			if _, err := stmt.Exec(fmt.Sprintf("user-%d", i), fmt.Sprintf("bulk-uuid-%d", i), "pw", fmt.Sprintf("bulk-tok-%d", i)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("bulk insert: %v", err)
	}

	var hits []ConnectionHit
	rows, err := st.db.Query(`SELECT id FROM users WHERE name LIKE 'user-%'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		hits = append(hits,
			ConnectionHit{UserID: id, IP: "198.51.100.1", SeenAt: now, Hits: 1},
			ConnectionHit{UserID: id, IP: "198.51.100.2", SeenAt: now, Hits: 1},
		)
	}
	if err := st.AddConnections(hits); err != nil {
		t.Fatalf("add connections: %v", err)
	}

	// StampDeviceOverLimit: should arm 1050 users across multiple chunks (>999 SQLite variable limit)
	if err := st.StampDeviceOverLimit(now); err != nil {
		t.Fatalf("StampDeviceOverLimit arming: %v", err)
	}

	var armedCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM users WHERE device_over_since = ?`, now).Scan(&armedCount); err != nil {
		t.Fatal(err)
	}
	if armedCount != 1050 {
		t.Fatalf("expected 1050 users armed, got %d", armedCount)
	}

	// Disarm: advance time so sightings age out of the online window
	if err := st.StampDeviceOverLimit(now + model.DeviceOnlineWindow + 10); err != nil {
		t.Fatalf("StampDeviceOverLimit disarming: %v", err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM users WHERE device_over_since <> 0`).Scan(&armedCount); err != nil {
		t.Fatal(err)
	}
	if armedCount != 0 {
		t.Fatalf("expected 0 users armed after disarm, got %d", armedCount)
	}
}

