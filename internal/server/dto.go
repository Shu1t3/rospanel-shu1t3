package server

import (
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/store"
)

// DTO layer for internal/server.
// Decouples HTTP transport serialization formats and API contracts from domain and storage entities.

// --- Group DTOs ---

type groupDTO struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	CreatedAt int64    `json:"created_at"`
	Grants    []string `json:"grants,omitempty"`
	Members   int      `json:"members"`
	MemberIDs []int64  `json:"member_ids"`
}

func toGroupDTO(g *model.Group) groupDTO {
	if g == nil {
		return groupDTO{}
	}
	return groupDTO{
		ID:        g.ID,
		Name:      g.Name,
		CreatedAt: g.CreatedAt,
		Grants:    g.Grants,
		Members:   g.Members,
		MemberIDs: g.MemberIDs,
	}
}

func toGroupDTOs(groups []model.Group) []groupDTO {
	if groups == nil {
		return []groupDTO{}
	}
	out := make([]groupDTO, len(groups))
	for i := range groups {
		out[i] = toGroupDTO(&groups[i])
	}
	return out
}

// --- Webhook DTOs ---

type webhookDTO struct {
	ID            int64    `json:"id"`
	URL           string   `json:"url"`
	Secret        string   `json:"secret"`
	Events        []string `json:"events"`
	Enabled       bool     `json:"enabled"`
	CreatedAt     int64    `json:"created_at"`
	LastStatus    int      `json:"last_status"`
	LastAttemptAt int64    `json:"last_attempt_at"`
	LastError     string   `json:"last_error"`
}

func toWebhookDTO(w *model.Webhook) webhookDTO {
	if w == nil {
		return webhookDTO{}
	}
	events := w.Events
	if events == nil {
		events = []string{}
	}
	return webhookDTO{
		ID:            w.ID,
		URL:           w.URL,
		Secret:        w.Secret,
		Events:        events,
		Enabled:       w.Enabled,
		CreatedAt:     w.CreatedAt,
		LastStatus:    w.LastStatus,
		LastAttemptAt: w.LastAttemptAt,
		LastError:     w.LastError,
	}
}

func toWebhookDTOs(webhooks []model.Webhook) []webhookDTO {
	if webhooks == nil {
		return []webhookDTO{}
	}
	out := make([]webhookDTO, len(webhooks))
	for i := range webhooks {
		out[i] = toWebhookDTO(&webhooks[i])
	}
	return out
}

// --- Admin & AdminSession DTOs ---

type adminDTO struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
	CreatedAt          int64  `json:"created_at"`
	LastLoginAt        int64  `json:"last_login_at"`
	TOTPEnabled        bool   `json:"totp_enabled"`
}

func toAdminDTO(a model.Admin) adminDTO {
	return adminDTO{
		ID:                 a.ID,
		Username:           a.Username,
		Role:               a.Role,
		MustChangePassword: a.MustChangePassword,
		CreatedAt:          a.CreatedAt,
		LastLoginAt:        a.LastLoginAt,
		TOTPEnabled:        a.TOTPEnabled,
	}
}

func toAdminDTOs(admins []model.Admin) []adminDTO {
	if admins == nil {
		return []adminDTO{}
	}
	out := make([]adminDTO, len(admins))
	for i := range admins {
		out[i] = toAdminDTO(admins[i])
	}
	return out
}

type adminSessionDTO struct {
	ID         int64  `json:"id"`
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	CreatedAt  int64  `json:"created_at"`
	LastSeenAt int64  `json:"last_seen_at"`
	ExpiresAt  int64  `json:"expires_at"`
	Current    bool   `json:"current"`
}

func toAdminSessionDTO(s *store.AdminSession) adminSessionDTO {
	if s == nil {
		return adminSessionDTO{}
	}
	return adminSessionDTO{
		ID:         s.ID,
		IP:         s.IP,
		UserAgent:  s.UserAgent,
		CreatedAt:  s.CreatedAt,
		LastSeenAt: s.LastSeenAt,
		ExpiresAt:  s.ExpiresAt,
		Current:    s.Current,
	}
}

func toAdminSessionDTOs(sessions []store.AdminSession) []adminSessionDTO {
	if sessions == nil {
		return []adminSessionDTO{}
	}
	out := make([]adminSessionDTO, len(sessions))
	for i := range sessions {
		out[i] = toAdminSessionDTO(&sessions[i])
	}
	return out
}

// --- Billing DTOs ---

type tariffPlanDTO struct {
	ID          int64   `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	PriceRub    int     `json:"price_rub"`
	PeriodDays  int     `json:"period_days"`
	DataLimit   int64   `json:"data_limit"`
	DeviceLimit int     `json:"device_limit"`
	SpeedLimit  int     `json:"speed_limit"`
	ResetPeriod string  `json:"reset_period"`
	SortOrder   int     `json:"sort_order"`
	Enabled     bool    `json:"enabled"`
	GroupIDs    []int64 `json:"group_ids"`
}

func toTariffPlanDTO(p *model.TariffPlan) tariffPlanDTO {
	if p == nil {
		return tariffPlanDTO{}
	}
	groupIDs := p.GroupIDs
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	return tariffPlanDTO{
		ID:          p.ID,
		Slug:        p.Slug,
		Name:        p.Name,
		PriceRub:    p.PriceRub,
		PeriodDays:  p.PeriodDays,
		DataLimit:   p.DataLimit,
		DeviceLimit: p.DeviceLimit,
		SpeedLimit:  p.SpeedLimit,
		ResetPeriod: p.ResetPeriod,
		SortOrder:   p.SortOrder,
		Enabled:     p.Enabled,
		GroupIDs:    groupIDs,
	}
}

func toTariffPlanDTOs(plans []model.TariffPlan) []tariffPlanDTO {
	if plans == nil {
		return []tariffPlanDTO{}
	}
	out := make([]tariffPlanDTO, len(plans))
	for i := range plans {
		out[i] = toTariffPlanDTO(&plans[i])
	}
	return out
}

func fromTariffPlanDTO(d tariffPlanDTO) model.TariffPlan {
	groupIDs := d.GroupIDs
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	return model.TariffPlan{
		ID:          d.ID,
		Slug:        d.Slug,
		Name:        d.Name,
		PriceRub:    d.PriceRub,
		PeriodDays:  d.PeriodDays,
		DataLimit:   d.DataLimit,
		DeviceLimit: d.DeviceLimit,
		SpeedLimit:  d.SpeedLimit,
		ResetPeriod: d.ResetPeriod,
		SortOrder:   d.SortOrder,
		Enabled:     d.Enabled,
		GroupIDs:    groupIDs,
	}
}

type paymentOrderDTO struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	UserName   string `json:"user_name,omitempty"`
	PlanID     int64  `json:"plan_id"`
	PlanName   string `json:"plan_name,omitempty"`
	AmountRub  int    `json:"amount_rub"`
	Status     string `json:"status"`
	Provider   string `json:"provider"`
	ProviderID string `json:"provider_id,omitempty"`
	PayURL     string `json:"pay_url,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	PaidAt     int64  `json:"paid_at"`
}

func toPaymentOrderDTO(o *model.PaymentOrder) paymentOrderDTO {
	if o == nil {
		return paymentOrderDTO{}
	}
	return paymentOrderDTO{
		ID:         o.ID,
		UserID:     o.UserID,
		UserName:   o.UserName,
		PlanID:     o.PlanID,
		PlanName:   o.PlanName,
		AmountRub:  o.AmountRub,
		Status:     o.Status,
		Provider:   o.Provider,
		ProviderID: o.ProviderID,
		PayURL:     o.PayURL,
		CreatedAt:  o.CreatedAt,
		PaidAt:     o.PaidAt,
	}
}

func toPaymentOrderDTOs(orders []model.PaymentOrder) []paymentOrderDTO {
	if orders == nil {
		return []paymentOrderDTO{}
	}
	out := make([]paymentOrderDTO, len(orders))
	for i := range orders {
		out[i] = toPaymentOrderDTO(&orders[i])
	}
	return out
}

type providerStatDTO struct {
	Provider string `json:"provider"`
	Count    int    `json:"count"`
	Sum      int    `json:"sum"`
}

func toProviderStatDTO(s model.ProviderStat) providerStatDTO {
	return providerStatDTO{
		Provider: s.Provider,
		Count:    s.Count,
		Sum:      s.Sum,
	}
}

func toProviderStatDTOs(stats []model.ProviderStat) []providerStatDTO {
	if stats == nil {
		return []providerStatDTO{}
	}
	out := make([]providerStatDTO, len(stats))
	for i := range stats {
		out[i] = toProviderStatDTO(stats[i])
	}
	return out
}

type paymentStatsDTO struct {
	TotalPaid    int               `json:"total_paid"`
	PaidCount    int               `json:"paid_count"`
	EarnedToday  int               `json:"earned_today"`
	EarnedMonth  int               `json:"earned_month"`
	PendingCount int               `json:"pending_count"`
	PendingSum   int               `json:"pending_sum"`
	ByProvider   []providerStatDTO `json:"by_provider"`
}

func toPaymentStatsDTO(s *model.PaymentStats) paymentStatsDTO {
	if s == nil {
		return paymentStatsDTO{ByProvider: []providerStatDTO{}}
	}
	return paymentStatsDTO{
		TotalPaid:    s.TotalPaid,
		PaidCount:    s.PaidCount,
		EarnedToday:  s.EarnedToday,
		EarnedMonth:  s.EarnedMonth,
		PendingCount: s.PendingCount,
		PendingSum:   s.PendingSum,
		ByProvider:   toProviderStatDTOs(s.ByProvider),
	}
}

// --- Broadcast DTOs ---

type broadcastButtonDTO struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

func toBroadcastButtonDTO(b model.BroadcastButton) broadcastButtonDTO {
	return broadcastButtonDTO{
		Text: b.Text,
		URL:  b.URL,
	}
}

func toBroadcastButtonDTOs(buttons []model.BroadcastButton) []broadcastButtonDTO {
	if buttons == nil {
		return []broadcastButtonDTO{}
	}
	out := make([]broadcastButtonDTO, len(buttons))
	for i := range buttons {
		out[i] = toBroadcastButtonDTO(buttons[i])
	}
	return out
}

func fromBroadcastButtonDTO(d broadcastButtonDTO) model.BroadcastButton {
	return model.BroadcastButton{
		Text: d.Text,
		URL:  d.URL,
	}
}

func fromBroadcastButtonDTOs(buttons []broadcastButtonDTO) []model.BroadcastButton {
	if buttons == nil {
		return []model.BroadcastButton{}
	}
	out := make([]model.BroadcastButton, len(buttons))
	for i := range buttons {
		out[i] = fromBroadcastButtonDTO(buttons[i])
	}
	return out
}

type broadcastDTO struct {
	ID         int64                `json:"id"`
	CreatedBy  string               `json:"created_by"`
	Text       string               `json:"text"`
	MediaKind  string               `json:"media_kind"`
	MediaName  string               `json:"media_name"`
	Buttons    []broadcastButtonDTO `json:"buttons"`
	Audience   string               `json:"audience"`
	Status     string               `json:"status"`
	CreatedAt  int64                `json:"created_at"`
	StartedAt  int64                `json:"started_at"`
	FinishedAt int64                `json:"finished_at"`
	Total      int                  `json:"total"`
	Sent       int                  `json:"sent"`
	Failed     int                  `json:"failed"`
	Blocked    int                  `json:"blocked"`
	Skipped    int                  `json:"skipped"`
}

func toBroadcastDTO(b *model.Broadcast) broadcastDTO {
	if b == nil {
		return broadcastDTO{Buttons: []broadcastButtonDTO{}}
	}
	return broadcastDTO{
		ID:         b.ID,
		CreatedBy:  b.CreatedBy,
		Text:       b.Text,
		MediaKind:  b.MediaKind,
		MediaName:  b.MediaName,
		Buttons:    toBroadcastButtonDTOs(b.Buttons),
		Audience:   b.Audience,
		Status:     b.Status,
		CreatedAt:  b.CreatedAt,
		StartedAt:  b.StartedAt,
		FinishedAt: b.FinishedAt,
		Total:      b.Total,
		Sent:       b.Sent,
		Failed:     b.Failed,
		Blocked:    b.Blocked,
		Skipped:    b.Skipped,
	}
}

func toBroadcastDTOs(broadcasts []model.Broadcast) []broadcastDTO {
	if broadcasts == nil {
		return []broadcastDTO{}
	}
	out := make([]broadcastDTO, len(broadcasts))
	for i := range broadcasts {
		out[i] = toBroadcastDTO(&broadcasts[i])
	}
	return out
}

// --- Events & Audit DTOs ---

type userEventDTO struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	UserName  string `json:"user_name"`
	Action    string `json:"action"`
	ActorKind string `json:"actor_kind"`
	ActorName string `json:"actor_name"`
	Details   any    `json:"details"`
	CreatedAt int64  `json:"created_at"`
}

func toUserEventDTO(e *model.UserEvent) userEventDTO {
	if e == nil {
		return userEventDTO{}
	}
	return userEventDTO{
		ID:        e.ID,
		UserID:    e.UserID,
		UserName:  e.UserName,
		Action:    e.Action,
		ActorKind: e.ActorKind,
		ActorName: e.ActorName,
		Details:   e.Details,
		CreatedAt: e.CreatedAt,
	}
}

func toUserEventDTOs(events []model.UserEvent) []userEventDTO {
	if events == nil {
		return []userEventDTO{}
	}
	out := make([]userEventDTO, len(events))
	for i := range events {
		out[i] = toUserEventDTO(&events[i])
	}
	return out
}

type adminAuditDTO struct {
	ID        int64  `json:"id"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	ActorKind string `json:"actor_kind"`
	ActorName string `json:"actor_name"`
	IP        string `json:"ip"`
	Details   any    `json:"details"`
	CreatedAt int64  `json:"created_at"`
}

func toAdminAuditDTO(a *model.AdminAudit) adminAuditDTO {
	if a == nil {
		return adminAuditDTO{}
	}
	return adminAuditDTO{
		ID:        a.ID,
		Action:    a.Action,
		Target:    a.Target,
		ActorKind: a.ActorKind,
		ActorName: a.ActorName,
		IP:        a.IP,
		Details:   a.Details,
		CreatedAt: a.CreatedAt,
	}
}

func toAdminAuditDTOs(audits []model.AdminAudit) []adminAuditDTO {
	if audits == nil {
		return []adminAuditDTO{}
	}
	out := make([]adminAuditDTO, len(audits))
	for i := range audits {
		out[i] = toAdminAuditDTO(&audits[i])
	}
	return out
}

type adminAuditEntryDTO struct {
	Key      string `json:"key"`
	Category string `json:"category"`
}

func toAdminAuditEntryDTO(e model.AdminAuditEntry) adminAuditEntryDTO {
	return adminAuditEntryDTO{
		Key:      e.Key,
		Category: e.Category,
	}
}

func toAdminAuditEntryDTOs(entries []model.AdminAuditEntry) []adminAuditEntryDTO {
	if entries == nil {
		return []adminAuditEntryDTO{}
	}
	out := make([]adminAuditEntryDTO, len(entries))
	for i := range entries {
		out[i] = toAdminAuditEntryDTO(entries[i])
	}
	return out
}

// --- External Subscription & Server DTOs ---

type extSubscriptionDTO struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
	LastFetchAt int64  `json:"last_fetch_at"`
	LastOKAt    int64  `json:"last_ok_at"`
	LastError   string `json:"last_error,omitempty"`
	ServerCount int    `json:"server_count"`
	CreatedAt   int64  `json:"created_at"`
}

func toExtSubscriptionDTO(s *model.ExtSubscription) extSubscriptionDTO {
	if s == nil {
		return extSubscriptionDTO{}
	}
	return extSubscriptionDTO{
		ID:          s.ID,
		Name:        s.Name,
		Source:      s.Source,
		Enabled:     s.Enabled,
		LastFetchAt: s.LastFetchAt,
		LastOKAt:    s.LastOKAt,
		LastError:   s.LastError,
		ServerCount: s.ServerCount,
		CreatedAt:   s.CreatedAt,
	}
}

func toExtSubscriptionDTOs(subs []model.ExtSubscription) []extSubscriptionDTO {
	if subs == nil {
		return []extSubscriptionDTO{}
	}
	out := make([]extSubscriptionDTO, len(subs))
	for i := range subs {
		out[i] = toExtSubscriptionDTO(&subs[i])
	}
	return out
}

type extServerDTO struct {
	ID       int64  `json:"id"`
	SubID    int64  `json:"sub_id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Enabled  bool   `json:"enabled"`
	SeenAt   int64  `json:"seen_at"`
}

func toExtServerDTO(s *model.ExtServer) extServerDTO {
	if s == nil {
		return extServerDTO{}
	}
	return extServerDTO{
		ID:       s.ID,
		SubID:    s.SubID,
		Name:     s.Name,
		Protocol: s.Protocol,
		Host:     s.Host,
		Port:     s.Port,
		Enabled:  s.Enabled,
		SeenAt:   s.SeenAt,
	}
}

func toExtServerDTOs(servers []model.ExtServer) []extServerDTO {
	if servers == nil {
		return []extServerDTO{}
	}
	out := make([]extServerDTO, len(servers))
	for i := range servers {
		out[i] = toExtServerDTO(&servers[i])
	}
	return out
}

// --- Config Snapshot DTOs ---

type configSnapshotDTO struct {
	ID        int64  `json:"id"`
	CreatedAt int64  `json:"created_at"`
	Label     string `json:"label"`
	Auto      bool   `json:"auto"`
}

func toConfigSnapshotDTO(s *model.ConfigSnapshot) configSnapshotDTO {
	if s == nil {
		return configSnapshotDTO{}
	}
	return configSnapshotDTO{
		ID:        s.ID,
		CreatedAt: s.CreatedAt,
		Label:     s.Label,
		Auto:      s.Auto,
	}
}

func toConfigSnapshotDTOs(snapshots []model.ConfigSnapshot) []configSnapshotDTO {
	if snapshots == nil {
		return []configSnapshotDTO{}
	}
	out := make([]configSnapshotDTO, len(snapshots))
	for i := range snapshots {
		out[i] = toConfigSnapshotDTO(&snapshots[i])
	}
	return out
}

// --- Support Group DTOs ---

type supportGroupDTO struct {
	ChatID  int64  `json:"chat_id"`
	Title   string `json:"title"`
	IsForum bool   `json:"is_forum"`
	IsAdmin bool   `json:"is_admin"`
}

func toSupportGroupDTO(g *model.SupportGroup) supportGroupDTO {
	if g == nil {
		return supportGroupDTO{}
	}
	return supportGroupDTO{
		ChatID:  g.ChatID,
		Title:   g.Title,
		IsForum: g.IsForum,
		IsAdmin: g.IsAdmin,
	}
}

func toSupportGroupDTOs(groups []model.SupportGroup) []supportGroupDTO {
	if groups == nil {
		return []supportGroupDTO{}
	}
	out := make([]supportGroupDTO, len(groups))
	for i := range groups {
		out[i] = toSupportGroupDTO(&groups[i])
	}
	return out
}

// --- SubRule & ProbeHit DTOs ---

type subRuleDTO struct {
	Field   string `json:"field"`
	Op      string `json:"op"`
	Value   string `json:"value"`
	Action  string `json:"action"`
	Enabled bool   `json:"enabled"`
}

func toSubRuleDTO(r *model.SubRule) subRuleDTO {
	if r == nil {
		return subRuleDTO{}
	}
	return subRuleDTO{
		Field:   r.Field,
		Op:      r.Op,
		Value:   r.Value,
		Action:  r.Action,
		Enabled: r.Enabled,
	}
}

func toSubRuleDTOs(rules []model.SubRule) []subRuleDTO {
	if rules == nil {
		return []subRuleDTO{}
	}
	out := make([]subRuleDTO, len(rules))
	for i := range rules {
		out[i] = toSubRuleDTO(&rules[i])
	}
	return out
}

func fromSubRuleDTO(d subRuleDTO) model.SubRule {
	return model.SubRule{
		Field:   d.Field,
		Op:      d.Op,
		Value:   d.Value,
		Action:  d.Action,
		Enabled: d.Enabled,
	}
}

func fromSubRuleDTOs(rules []subRuleDTO) []model.SubRule {
	if rules == nil {
		return []model.SubRule{}
	}
	out := make([]model.SubRule, len(rules))
	for i := range rules {
		out[i] = fromSubRuleDTO(rules[i])
	}
	return out
}

type probeHitDTO struct {
	IP        string `json:"ip"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
	Hits      int64  `json:"hits"`
	Paths     int64  `json:"paths"`
	Country   string `json:"country,omitempty"`
	ASN       uint32 `json:"asn,omitempty"`
	Org       string `json:"org,omitempty"`
}

func toProbeHitDTO(p *model.ProbeHit) probeHitDTO {
	if p == nil {
		return probeHitDTO{}
	}
	return probeHitDTO{
		IP:        p.IP,
		FirstSeen: p.FirstSeen,
		LastSeen:  p.LastSeen,
		Hits:      p.Hits,
		Paths:     p.Paths,
		Country:   p.Country,
		ASN:       p.ASN,
		Org:       p.Org,
	}
}

func toProbeHitDTOs(probes []model.ProbeHit) []probeHitDTO {
	if probes == nil {
		return []probeHitDTO{}
	}
	out := make([]probeHitDTO, len(probes))
	for i := range probes {
		out[i] = toProbeHitDTO(&probes[i])
	}
	return out
}

// --- Stats DTOs ---

type dailyPointDTO struct {
	Day  string `json:"day"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

func toDailyPointDTO(p model.DailyPoint) dailyPointDTO {
	return dailyPointDTO{
		Day:  p.Day,
		Up:   p.Up,
		Down: p.Down,
	}
}

func toDailyPointDTOs(pts []model.DailyPoint) []dailyPointDTO {
	if pts == nil {
		return []dailyPointDTO{}
	}
	out := make([]dailyPointDTO, len(pts))
	for i := range pts {
		out[i] = toDailyPointDTO(pts[i])
	}
	return out
}

type userTotalDTO struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Up     int64  `json:"up"`
	Down   int64  `json:"down"`
}

func toUserTotalDTO(u model.UserTotal) userTotalDTO {
	return userTotalDTO{
		UserID: u.UserID,
		Name:   u.Name,
		Up:     u.Up,
		Down:   u.Down,
	}
}

func toUserTotalDTOs(totals []model.UserTotal) []userTotalDTO {
	if totals == nil {
		return []userTotalDTO{}
	}
	out := make([]userTotalDTO, len(totals))
	for i := range totals {
		out[i] = toUserTotalDTO(totals[i])
	}
	return out
}

type countryStatDTO struct {
	Code string `json:"code"`
	IPs  int64  `json:"ips"`
	Hits int64  `json:"hits"`
}

func toCountryStatDTO(c model.CountryStat) countryStatDTO {
	return countryStatDTO{
		Code: c.Code,
		IPs:  c.IPs,
		Hits: c.Hits,
	}
}

func toCountryStatDTOs(stats []model.CountryStat) []countryStatDTO {
	if stats == nil {
		return []countryStatDTO{}
	}
	out := make([]countryStatDTO, len(stats))
	for i := range stats {
		out[i] = toCountryStatDTO(stats[i])
	}
	return out
}

type asnStatDTO struct {
	ASN  uint32 `json:"asn"`
	Org  string `json:"org"`
	IPs  int64  `json:"ips"`
	Hits int64  `json:"hits"`
}

func toASNStatDTO(a model.ASNStat) asnStatDTO {
	return asnStatDTO{
		ASN:  a.ASN,
		Org:  a.Org,
		IPs:  a.IPs,
		Hits: a.Hits,
	}
}

func toASNStatDTOs(stats []model.ASNStat) []asnStatDTO {
	if stats == nil {
		return []asnStatDTO{}
	}
	out := make([]asnStatDTO, len(stats))
	for i := range stats {
		out[i] = toASNStatDTO(stats[i])
	}
	return out
}

type connectionDTO struct {
	IP       string `json:"ip"`
	LastSeen int64  `json:"last_seen"`
	Count    int64  `json:"count"`
}

func toConnectionDTO(c model.Connection) connectionDTO {
	return connectionDTO{
		IP:       c.IP,
		LastSeen: c.LastSeen,
		Count:    c.Count,
	}
}

func toConnectionDTOs(conns []model.Connection) []connectionDTO {
	if conns == nil {
		return []connectionDTO{}
	}
	out := make([]connectionDTO, len(conns))
	for i := range conns {
		out[i] = toConnectionDTO(conns[i])
	}
	return out
}

// --- System Proxy DTOs ---

type systemProxyAccountDTO struct {
	User string `json:"user"`
	Pass string `json:"pass"`
}

func toSystemProxyAccountDTO(a model.SystemProxyAccount) systemProxyAccountDTO {
	return systemProxyAccountDTO{
		User: a.User,
		Pass: a.Pass,
	}
}

func toSystemProxyAccountDTOs(accounts []model.SystemProxyAccount) []systemProxyAccountDTO {
	if accounts == nil {
		return []systemProxyAccountDTO{}
	}
	out := make([]systemProxyAccountDTO, len(accounts))
	for i := range accounts {
		out[i] = toSystemProxyAccountDTO(accounts[i])
	}
	return out
}

func fromSystemProxyAccountDTO(d systemProxyAccountDTO) model.SystemProxyAccount {
	return model.SystemProxyAccount{
		User: d.User,
		Pass: d.Pass,
	}
}

func fromSystemProxyAccountDTOs(accounts []systemProxyAccountDTO) []model.SystemProxyAccount {
	if accounts == nil {
		return []model.SystemProxyAccount{}
	}
	out := make([]model.SystemProxyAccount, len(accounts))
	for i := range accounts {
		out[i] = fromSystemProxyAccountDTO(accounts[i])
	}
	return out
}

type systemProxyDTO struct {
	SocksEnabled bool                    `json:"socks_enabled"`
	SocksPort    int                     `json:"socks_port"`
	HTTPEnabled  bool                    `json:"http_enabled"`
	HTTPPort     int                     `json:"http_port"`
	Accounts     []systemProxyAccountDTO `json:"accounts"`
}

func toSystemProxyDTO(p *model.SystemProxy) systemProxyDTO {
	if p == nil {
		return systemProxyDTO{Accounts: []systemProxyAccountDTO{}}
	}
	return systemProxyDTO{
		SocksEnabled: p.SocksEnabled,
		SocksPort:    p.SocksPort,
		HTTPEnabled:  p.HTTPEnabled,
		HTTPPort:     p.HTTPPort,
		Accounts:     toSystemProxyAccountDTOs(p.Accounts),
	}
}

func fromSystemProxyDTO(d systemProxyDTO) model.SystemProxy {
	return model.SystemProxy{
		SocksEnabled: d.SocksEnabled,
		SocksPort:    d.SocksPort,
		HTTPEnabled:  d.HTTPEnabled,
		HTTPPort:     d.HTTPPort,
		Accounts:     fromSystemProxyAccountDTOs(d.Accounts),
	}
}
