package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func BenchmarkGetSettings(b *testing.B) {
	st, err := Open(filepath.Join(b.TempDir(), "bench_settings.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()

	b.ReportAllocs()
	for b.Loop() {
		_, err := st.GetSettings()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWorkingUsers(b *testing.B) {
	st, err := Open(filepath.Join(b.TempDir(), "bench_working.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	for i := 0; i < 200; i++ {
		_, err := st.CreateUser(fmt.Sprintf("user_%d", i), fmt.Sprintf("uuid_%d", i), "secret_pass", fmt.Sprintf("sub_%d", i), 100<<30, now+86400, 3)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		users, err := st.WorkingUsers(now)
		if err != nil {
			b.Fatal(err)
		}
		if len(users) != 200 {
			b.Fatalf("expected 200 users, got %d", len(users))
		}
	}
}

func BenchmarkListUsers_All(b *testing.B) {
	st, err := Open(filepath.Join(b.TempDir(), "bench_list_all.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	for i := 0; i < 500; i++ {
		_, err := st.CreateUser(fmt.Sprintf("user_%d", i), fmt.Sprintf("uuid_%d", i), "secret_pass", fmt.Sprintf("sub_%d", i), 100<<30, now+86400, 3)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		users, err := st.ListUsers()
		if err != nil {
			b.Fatal(err)
		}
		if len(users) != 500 {
			b.Fatalf("expected 500 users, got %d", len(users))
		}
	}
}

func BenchmarkListUsers_Paged(b *testing.B) {
	st, err := Open(filepath.Join(b.TempDir(), "bench_list_paged.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	for i := 0; i < 500; i++ {
		_, err := st.CreateUser(fmt.Sprintf("user_%d", i), fmt.Sprintf("uuid_%d", i), "secret_pass", fmt.Sprintf("sub_%d", i), 100<<30, now+86400, 3)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	for b.Loop() {
		users, total, err := st.ListUsersPaged(20, 0)
		if err != nil {
			b.Fatal(err)
		}
		if len(users) != 20 || total != 500 {
			b.Fatalf("expected 20 users and 500 total, got %d and %d", len(users), total)
		}
	}
}
