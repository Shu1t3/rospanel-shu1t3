package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/backup"
	"github.com/Shu1t3/rospanel-shu1t3/internal/migration"
)

type startMigrationReq struct {
	CandidateAddr string `json:"candidate_addr"`
	DNSType       string `json:"dns_type"` // "cloudflare", "manual"
	CFToken       string `json:"cf_token,omitempty"`
	CFZoneID      string `json:"cf_zone_id,omitempty"`
}

type startMigrationResp struct {
	PairToken  string `json:"pair_token"`
	InstallCmd string `json:"install_cmd"`
	Phase      string `json:"phase"`
}

// InitMigration initializes the migration coordinator for the router.
func (rt *Router) InitMigration(dataDir string) error {
	coord, err := migration.NewCoordinator(dataDir, rt.mgr.Store())
	if err != nil {
		return err
	}
	rt.coord = coord
	if coord.StateManager().IsFenced() {
		rt.mgr.SetFenced(true)
	}
	return nil
}

// MigrationCoordinator returns the active migration coordinator (may be nil).
func (rt *Router) MigrationCoordinator() *migration.Coordinator {
	return rt.coord
}

func (rt *Router) handleMigrationStatus(w http.ResponseWriter, r *http.Request) {
	coord := rt.MigrationCoordinator()
	if coord == nil {
		writeErr(w, http.StatusInternalServerError, "координатор миграции не инициализирован")
		return
	}
	sess := coord.StateManager().GetSession()
	sess.PairToken = ""
	writeJSON(w, http.StatusOK, sess)
}

func (rt *Router) handleMigrationStart(w http.ResponseWriter, r *http.Request) {
	coord := rt.MigrationCoordinator()
	if coord == nil {
		writeErr(w, http.StatusInternalServerError, "координатор миграции не инициализирован")
		return
	}

	var req startMigrationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "неверный формат запроса")
		return
	}

	dnsType := req.DNSType
	if dnsType == "" {
		dnsType = "manual"
	}

	var cfConfig *migration.CloudflareConfig
	if dnsType == "cloudflare" && req.CFToken != "" {
		cfConfig = &migration.CloudflareConfig{
			APIToken: req.CFToken,
			ZoneID:   req.CFZoneID,
		}
	}

	token, err := coord.InitiateMigration(req.CandidateAddr, dnsType, cfConfig)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Generate one-line install command for candidate server
	candHost := coord.StateManager().GetSession().CandidateAddr
	masterURL := "https://" + r.Host
	installCmd := fmt.Sprintf(
		"curl -Ls https://raw.githubusercontent.com/Shu1t3/rospanel-shu1t3/main/install.sh | sudo bash -s -- --candidate '%s' --master '%s' --pair-token '%s'",
		candHost, masterURL, token,
	)

	writeJSON(w, http.StatusOK, startMigrationResp{
		PairToken:  token,
		InstallCmd: installCmd,
		Phase:      string(migration.PhasePrepare),
	})
}

func (rt *Router) handleMigrationVerify(w http.ResponseWriter, r *http.Request) {
	coord := rt.MigrationCoordinator()
	if coord == nil {
		writeErr(w, http.StatusInternalServerError, "координатор миграции не инициализирован")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	results, ready, err := coord.RunVerification(ctx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"ready":   ready,
	})
}

func (rt *Router) handleMigrationSwitch(w http.ResponseWriter, r *http.Request) {
	coord := rt.MigrationCoordinator()
	if coord == nil {
		writeErr(w, http.StatusInternalServerError, "координатор миграции не инициализирован")
		return
	}

	sess := coord.StateManager().GetSession()
	var dnsAdapter migration.DNSAdapter
	if sess.DNSType == "cloudflare" {
		dnsAdapter = migration.NewCloudflareAdapter(migration.CloudflareConfig{
			ZoneID: sess.DNSZoneID,
		})
	} else {
		dnsAdapter = migration.NewManualAdapter()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	if err := coord.ExecuteSwitchover(ctx, dnsAdapter, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Sync manager fencing
	rt.mgr.SetFenced(true)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"phase":   migration.PhaseStandby,
		"message": "Переключение выполнено. Старый сервер перешел в режим ожидания (standby).",
	})
}

func (rt *Router) handleMigrationDecommission(w http.ResponseWriter, r *http.Request) {
	coord := rt.MigrationCoordinator()
	if coord == nil {
		writeErr(w, http.StatusInternalServerError, "координатор миграции не инициализирован")
		return
	}

	force := r.URL.Query().Get("force") == "true"
	sess := coord.StateManager().GetSession()
	if sess.Role != migration.RoleStandby {
		writeErr(w, http.StatusBadRequest, "сервер не находится в режиме ожидания (standby)")
		return
	}

	target, err := migration.StandbyTarget(sess.CandidateAddr)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sc, err := migration.NewStandbyController(coord.StateManager(), target)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := sc.Decommission(r.Context(), force); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"phase":   migration.PhaseCompleted,
		"message": "Старый сервер успешно выведен из эксплуатации.",
	})
}

func (rt *Router) handleMigrationRollback(w http.ResponseWriter, r *http.Request) {
	coord := rt.MigrationCoordinator()
	if coord == nil {
		writeErr(w, http.StatusInternalServerError, "координатор миграции не инициализирован")
		return
	}

	sess := coord.StateManager().GetSession()
	var dnsAdapter migration.DNSAdapter
	if sess.DNSType == "cloudflare" {
		dnsAdapter = migration.NewCloudflareAdapter(migration.CloudflareConfig{
			ZoneID: sess.DNSZoneID,
		})
	}

	if err := coord.CancelRollback(r.Context(), dnsAdapter); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	rt.mgr.SetFenced(false)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"phase":   migration.PhaseRolledBack,
		"message": "Переезд отменен. Мастер разблокирован.",
	})
}

// Disaster recovery & backup health endpoints
func (rt *Router) handleMigrationBackupStatus(w http.ResponseWriter, r *http.Request) {
	locals, err := backup.ListLocal(rt.dataDir)
	var lastTime *time.Time
	rpoStr := "нет доступных копий"
	var rpoSec int64 = -1

	if err == nil && len(locals) > 0 {
		newest := filepath.Join(rt.dataDir, backup.LocalBackupDir, locals[0])
		if fi, err := os.Stat(newest); err == nil {
			t := fi.ModTime()
			lastTime = &t
			d, s := backup.EstimateRPO(t)
			rpoStr = s
			rpoSec = int64(d.Seconds())
		}
	}

	writeJSON(w, http.StatusOK, backup.BackupStatusView{
		LastBackupAt:      lastTime,
		LastSuccessfulRPO: rpoStr,
		RPOSeconds:        rpoSec,
		TotalLocalBackups: len(locals),
	})
}

func (rt *Router) handleMigrationTrialRestore(w http.ResponseWriter, r *http.Request) {
	locals, err := backup.ListLocal(rt.dataDir)
	if err != nil || len(locals) == 0 {
		writeErr(w, http.StatusNotFound, "нет локальных резервных копий для проверки")
		return
	}
	latest := filepath.Join(rt.dataDir, backup.LocalBackupDir, locals[0])
	report, err := backup.TrialRestoreSandbox(latest)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}
