package server

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

func TestNilSlicesReturnEmptyArray(t *testing.T) {
	if s := toGroupDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toGroupDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toWebhookDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toWebhookDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toAdminDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toAdminDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toTariffPlanDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toTariffPlanDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toPaymentOrderDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toPaymentOrderDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toBroadcastDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toBroadcastDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toUserEventDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toUserEventDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toAdminAuditEntryDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toAdminAuditEntryDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toExtSubscriptionDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toExtSubscriptionDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toExtServerDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toExtServerDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toConfigSnapshotDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toConfigSnapshotDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toDailyPointDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toDailyPointDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toUserTotalDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toUserTotalDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toCountryStatDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toCountryStatDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toASNStatDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toASNStatDTOs(nil) = %v, want empty non-nil slice", s)
	}
	if s := toConnectionDTOs(nil); s == nil || len(s) != 0 {
		t.Errorf("toConnectionDTOs(nil) = %v, want empty non-nil slice", s)
	}
}

func TestGroupDTOMapping(t *testing.T) {
	orig := model.Group{
		ID:        42,
		Name:      "VIP Users",
		CreatedAt: time.Now().Unix(),
		Grants:    []string{"vless", "shadowsocks"},
		Members:   10,
		MemberIDs: []int64{101, 102},
	}

	dto := toGroupDTO(&orig)
	if dto.ID != orig.ID || dto.Name != orig.Name || dto.Members != orig.Members || len(dto.Grants) != len(orig.Grants) {
		t.Fatalf("toGroupDTO mismatch: got %+v, want %+v", dto, orig)
	}

	// Verify JSON wire keys
	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal groupDTO: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal wire map: %v", err)
	}
	for _, key := range []string{"id", "name", "grants", "members", "member_ids", "created_at"} {
		if _, ok := wire[key]; !ok {
			t.Errorf("missing expected JSON key %q in groupDTO serialization", key)
		}
	}
}

func TestWebhookDTOMapping(t *testing.T) {
	orig := model.Webhook{
		ID:            7,
		URL:           "https://example.com/hook",
		Events:        []string{"user.created", "user.deleted"},
		Secret:        "whsec_123",
		Enabled:       true,
		CreatedAt:     1234500,
		LastStatus:    200,
		LastAttemptAt: 1234567,
		LastError:     "none",
	}

	dto := toWebhookDTO(&orig)
	if dto.ID != orig.ID || dto.URL != orig.URL || dto.Secret != orig.Secret || dto.LastStatus != orig.LastStatus {
		t.Fatalf("toWebhookDTO mismatch: got %+v, want %+v", dto, orig)
	}

	raw, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal webhookDTO: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("unmarshal wire map: %v", err)
	}
	for _, key := range []string{"id", "url", "secret", "events", "enabled", "created_at", "last_status", "last_attempt_at", "last_error"} {
		if _, ok := wire[key]; !ok {
			t.Errorf("missing expected JSON key %q in webhookDTO serialization", key)
		}
	}
}

func TestTariffPlanDTORoundTrip(t *testing.T) {
	orig := model.TariffPlan{
		ID:          101,
		Slug:        "standard-monthly",
		Name:        "Standard Monthly",
		PriceRub:    500,
		PeriodDays:  30,
		DataLimit:   100,
		DeviceLimit: 3,
		SpeedLimit:  50,
		SortOrder:   1,
		Enabled:     true,
		GroupIDs:    []int64{1, 2},
	}

	dto := toTariffPlanDTO(&orig)
	if dto.ID != orig.ID || dto.Name != orig.Name || dto.PriceRub != orig.PriceRub || dto.PeriodDays != orig.PeriodDays {
		t.Fatalf("toTariffPlanDTO mismatch: got %+v, want %+v", dto, orig)
	}

	back := fromTariffPlanDTO(dto)
	if back.ID != orig.ID || back.Name != orig.Name || len(back.GroupIDs) != len(orig.GroupIDs) {
		t.Fatalf("fromTariffPlanDTO mismatch: got %+v, want %+v", back, orig)
	}
}

func TestPaymentOrderDTOMapper(t *testing.T) {
	orig := &model.PaymentOrder{
		ID:         888,
		UserID:     12,
		PlanID:     101,
		AmountRub:  500,
		Provider:   "yookassa",
		ProviderID: "yoopid_999",
		Status:     "paid",
		CreatedAt:  time.Now().Unix() - 100,
		PaidAt:     time.Now().Unix(),
	}

	dto := toPaymentOrderDTO(orig)
	if dto.ID != orig.ID || dto.UserID != orig.UserID || dto.Status != orig.Status || dto.PaidAt != orig.PaidAt {
		t.Fatalf("toPaymentOrderDTO mismatch: got %+v, want %+v", dto, orig)
	}

	// Nil check returns empty struct
	nilDTO := toPaymentOrderDTO(nil)
	if nilDTO.ID != 0 {
		t.Errorf("toPaymentOrderDTO(nil).ID = %d, want 0", nilDTO.ID)
	}
}

func TestSystemProxyDTORoundTrip(t *testing.T) {
	orig := model.SystemProxy{
		SocksEnabled: true,
		SocksPort:    1080,
		HTTPEnabled:  true,
		HTTPPort:     8080,
		Accounts:     []model.SystemProxyAccount{{User: "admin", Pass: "secretpassword"}},
	}

	dto := toSystemProxyDTO(&orig)
	if !dto.SocksEnabled || dto.SocksPort != 1080 || len(dto.Accounts) != 1 {
		t.Fatalf("toSystemProxyDTO mismatch: got %+v, want %+v", dto, orig)
	}

	back := fromSystemProxyDTO(dto)
	if !back.SocksEnabled || back.SocksPort != 1080 || len(back.Accounts) != 1 || back.Accounts[0].Pass != "secretpassword" {
		t.Fatalf("fromSystemProxyDTO mismatch: got %+v, want %+v", back, orig)
	}
}

func TestStatsDTOFieldFidelity(t *testing.T) {
	dp := model.DailyPoint{Day: "2026-09-04", Up: 1000, Down: 5000}
	dpDTO := toDailyPointDTO(dp)
	if dpDTO.Day != dp.Day || dpDTO.Up != dp.Up || dpDTO.Down != dp.Down {
		t.Errorf("dailyPointDTO mismatch: got %+v, want %+v", dpDTO, dp)
	}

	ut := model.UserTotal{UserID: 42, Name: "bob", Up: 200, Down: 800}
	utDTO := toUserTotalDTO(ut)
	if utDTO.UserID != ut.UserID || utDTO.Name != ut.Name || utDTO.Up != ut.Up || utDTO.Down != ut.Down {
		t.Errorf("userTotalDTO mismatch: got %+v, want %+v", utDTO, ut)
	}

	cs := model.CountryStat{Code: "DE", IPs: 15, Hits: 300}
	csDTO := toCountryStatDTO(cs)
	if csDTO.Code != cs.Code || csDTO.IPs != cs.IPs || csDTO.Hits != cs.Hits {
		t.Errorf("countryStatDTO mismatch: got %+v, want %+v", csDTO, cs)
	}

	as := model.ASNStat{ASN: 12345, Org: "Telekom", IPs: 10, Hits: 250}
	asDTO := toASNStatDTO(as)
	if asDTO.ASN != as.ASN || asDTO.Org != as.Org || asDTO.IPs != as.IPs || asDTO.Hits != as.Hits {
		t.Errorf("asnStatDTO mismatch: got %+v, want %+v", asDTO, as)
	}
}
