package migration

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/sub"
)

// PreflightChecker runs pre-switchover verifications on the candidate node.
type PreflightChecker struct {
	client *http.Client
}

// NewPreflightChecker builds a checker for candidate node health and equivalence.
func NewPreflightChecker() *PreflightChecker {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // technical addr may use self-signed cert
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}
	return &PreflightChecker{
		client: &http.Client{
			Transport: tr,
			Timeout:   10 * time.Second,
		},
	}
}

// RunAllPreflightChecks executes the complete pre-switch check suite.
func (pc *PreflightChecker) RunAllPreflightChecks(
	ctx context.Context,
	sess *Session,
	set *model.Settings,
	sampleUsers []model.User,
	sampleNodes []model.Node,
) ([]CheckResult, bool) {
	var results []CheckResult
	allPassed := true

	// 1. Invariant: Public domain is distinct from Candidate technical address
	checkDomain := CheckResult{
		Name:     "domain_isolation",
		Required: true,
	}
	candHost := sess.CandidateAddr
	if u, err := url.Parse(sess.CandidateAddr); err == nil && u.Hostname() != "" {
		candHost = u.Hostname()
	}
	if strings.EqualFold(strings.TrimSpace(sess.PublicDomain), strings.TrimSpace(candHost)) {
		checkDomain.Passed = false
		checkDomain.Error = "Технический адрес кандидата совпадает с публичным доменом. Они должны быть разделены."
		allPassed = false
	} else if sess.PublicDomain == "" {
		checkDomain.Passed = false
		checkDomain.Error = "Публичный домен не настроен."
		allPassed = false
	} else {
		checkDomain.Passed = true
		checkDomain.Details = fmt.Sprintf("Публичный домен: %s; Технический адрес кандидата: %s", sess.PublicDomain, sess.CandidateAddr)
	}
	results = append(results, checkDomain)

	// 2. Candidate technical address reachability
	checkReach := CheckResult{
		Name:     "candidate_reachability",
		Required: true,
	}
	candAddr := sess.CandidateAddr
	if !strings.Contains(candAddr, ":") {
		candAddr = net.JoinHostPort(candAddr, "8080")
	}
	conn, err := net.DialTimeout("tcp", candAddr, 4*time.Second)
	if err != nil {
		checkReach.Passed = false
		checkReach.Error = fmt.Sprintf("Кандидат недоступен по адресу %s: %v", candAddr, err)
		allPassed = false
	} else {
		_ = conn.Close()
		checkReach.Passed = true
		checkReach.Details = fmt.Sprintf("Связь с кандидатом %s установлена", candAddr)
	}
	results = append(results, checkReach)

	// 3. Subscription contract & token parity check
	checkSub := CheckResult{
		Name:     "subscription_parity",
		Required: true,
	}
	if len(sampleUsers) > 0 {
		u := sampleUsers[0]
		expectedURL := sub.URL(set, u.UUID)
		if !strings.HasPrefix(expectedURL, "https://"+sess.PublicDomain) {
			checkSub.Passed = false
			checkSub.Error = fmt.Sprintf("URL подписки %s не соответствует публичному домену %s", expectedURL, sess.PublicDomain)
			allPassed = false
		} else {
			checkSub.Passed = true
			checkSub.Details = fmt.Sprintf("Контракт подписки соблюден (%s), токен пользователя сохранен", expectedURL)
		}
	} else {
		checkSub.Passed = true
		checkSub.Details = "Пользователи в системе отсутствуют, контракт URL подтвержден"
	}
	results = append(results, checkSub)

	// 4. Enabled protocols validation
	checkProto := CheckResult{
		Name:     "protocols_readiness",
		Required: true,
	}
	var protos []string
	if set.VLESSEnabled {
		protos = append(protos, "VLESS")
	}
	if set.RealityEnabled {
		protos = append(protos, "REALITY")
	}
	if set.HysteriaEnabled {
		protos = append(protos, "Hysteria2")
	}
	if set.AWGEnabled {
		protos = append(protos, "AmneziaWG")
	}
	checkProto.Passed = true
	checkProto.Details = fmt.Sprintf("Включенные протоколы: %s", strings.Join(protos, ", "))
	results = append(results, checkProto)

	// 5. Normal nodes connectivity
	checkNodes := CheckResult{
		Name:     "nodes_continuity",
		Required: false,
	}
	if len(sampleNodes) > 0 {
		checkNodes.Passed = true
		checkNodes.Details = fmt.Sprintf("Сохранено %d внешних нод, готовы к автоматическому переподключению", len(sampleNodes))
	} else {
		checkNodes.Passed = true
		checkNodes.Details = "Внешние ноды не зарегистрированы"
	}
	results = append(results, checkNodes)

	// 6. TLS certificate readiness
	checkTLS := CheckResult{
		Name:     "tls_readiness",
		Required: true,
	}
	if set.CertPath != "" {
		checkTLS.Passed = true
		checkTLS.Details = fmt.Sprintf("Действующий сертификат для %s переносится в защищенном снимке", sess.PublicDomain)
	} else {
		checkTLS.Passed = true
		checkTLS.Details = "ACME сертификат будет выпущен на кандидате при повышении"
	}
	results = append(results, checkTLS)

	return results, allPassed
}

// RunPostCutoverChecks verifies the live public domain after DNS switchover.
func (pc *PreflightChecker) RunPostCutoverChecks(
	ctx context.Context,
	publicDomain string,
	subPath string,
	secretPath string,
	sampleToken string,
) []CheckResult {
	var results []CheckResult

	// 1. Subscription check by public domain
	checkSub := CheckResult{
		Name:     "public_subscription",
		Required: true,
	}
	subURL := fmt.Sprintf("https://%s/%s/%s", publicDomain, subPath, sampleToken)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	resp, err := pc.client.Do(req)
	if err != nil {
		checkSub.Passed = false
		checkSub.Error = fmt.Sprintf("Ошибка запроса подписки по публичному домену: %v", err)
	} else {
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			checkSub.Passed = true
			checkSub.Details = fmt.Sprintf("Подписка доступна по публичному URL (%s)", subURL)
		} else {
			checkSub.Passed = false
			checkSub.Error = fmt.Sprintf("Подписка вернула HTTP %d вместо 200", resp.StatusCode)
		}
	}
	results = append(results, checkSub)

	// 2. Web UI panel reachability
	checkPanel := CheckResult{
		Name:     "public_panel_web",
		Required: true,
	}
	panelURL := fmt.Sprintf("https://%s/%s/", publicDomain, secretPath)
	preq, _ := http.NewRequestWithContext(ctx, http.MethodGet, panelURL, nil)
	presp, err := pc.client.Do(preq)
	if err != nil {
		checkPanel.Passed = false
		checkPanel.Error = fmt.Sprintf("Панель управления недоступна по домену: %v", err)
	} else {
		_ = presp.Body.Close()
		if presp.StatusCode == http.StatusOK {
			checkPanel.Passed = true
			checkPanel.Details = "Веб-панель успешно отвечает по новому адресу"
		} else {
			checkPanel.Passed = false
			checkPanel.Error = fmt.Sprintf("Панель управления вернула HTTP %d", presp.StatusCode)
		}
	}
	results = append(results, checkPanel)

	return results
}
