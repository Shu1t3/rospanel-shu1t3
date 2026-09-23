import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  decommissionMigration,
  getBackupHealth,
  getMigrationStatus,
  rollbackMigration,
  runTrialRestore,
  startMigration,
  switchMigration,
  verifyMigration,
  type BackupHealthView,
  type MigrationCheckResult,
  type MigrationSession,
  type TrialRestoreReport,
} from "./api";
import { fmtBytes, fmtStamp } from "./format";
import { errMessage, notifyError, notifySuccess } from "./notify";
import { Badge, Button, cn, Code, Modal, Spinner, TextInput } from "./ui";

function RadioCard({
  checked,
  onChange,
  label,
  hint,
  disabled,
}: {
  checked: boolean;
  onChange: () => void;
  label: React.ReactNode;
  hint?: React.ReactNode;
  disabled?: boolean;
}) {
  return (
    <label
      className={cn(
        "relative flex cursor-pointer select-none items-center gap-3 rounded-xl border px-3 py-2.5 text-sm transition",
        checked
          ? "border-brand-600 bg-brand-50/20"
          : "border-gray-200 bg-white hover:border-gray-300 hover:bg-gray-50",
        disabled && "pointer-events-none opacity-60"
      )}
    >
      <input
        type="radio"
        className="sr-only"
        checked={checked}
        onChange={onChange}
        disabled={disabled}
      />
      <span
        className={cn(
          "flex h-4 w-4 shrink-0 items-center justify-center rounded-full border transition",
          checked ? "border-brand-600 bg-brand-600" : "border-gray-300 bg-white"
        )}
      >
        {checked && <span className="h-1.5 w-1.5 rounded-full bg-white" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className={cn("block", checked ? "font-semibold text-ink" : "text-ink")}>{label}</span>
        {hint && <span className="block text-xs text-ink-muted">{hint}</span>}
      </span>
    </label>
  );
}

interface MigrationModalProps {
  open: boolean;
  onClose: () => void;
  currentDomain?: string;
}

export function MigrationModal({ open, onClose, currentDomain }: MigrationModalProps) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<"wizard" | "disaster">("wizard");
  const [loading, setLoading] = useState(false);
  const [session, setSession] = useState<MigrationSession | null>(null);

  // Form fields for starting migration
  const [candidateAddr, setCandidateAddr] = useState("");
  const [dnsType, setDnsType] = useState<"manual" | "cloudflare">("manual");
  const [cfToken, setCfToken] = useState("");
  const [cfZoneId, setCfZoneId] = useState("");

  // Generated command after start
  const [installCmd, setInstallCmd] = useState<string>("");

  // Verification results
  const [checks, setChecks] = useState<MigrationCheckResult[]>([]);
  const [ready, setReady] = useState(false);
  const [verifying, setVerifying] = useState(false);

  // Switchover and decommission confirmations
  const [confirmSwitch, setConfirmSwitch] = useState(false);
  const [confirmDecommission, setConfirmDecommission] = useState(false);
  const [confirmRollback, setConfirmRollback] = useState(false);
  const [switching, setSwitching] = useState(false);
  const [decommissioning, setDecommissioning] = useState(false);
  const [rollingBack, setRollingBack] = useState(false);

  // Disaster recovery & backup health
  const [backupHealth, setBackupHealth] = useState<BackupHealthView | null>(null);
  const [trialReport, setTrialReport] = useState<TrialRestoreReport | null>(null);
  const [trialLoading, setTrialLoading] = useState(false);

  // Load status on open
  const refreshStatus = useCallback(async () => {
    try {
      const s = await getMigrationStatus();
      setSession(s);
      if (s.checks && s.checks.length > 0) {
        setChecks(s.checks);
        const allRequiredPassed = s.checks.every((c) => !c.required || c.passed);
        setReady(allRequiredPassed && s.phase === "candidate_ready");
      } else {
        setChecks([]);
        setReady(false);
      }
    } catch {
      // Ignored if migration not initialized
    }
  }, []);

  const refreshBackup = useCallback(async () => {
    try {
      const bh = await getBackupHealth();
      setBackupHealth(bh);
    } catch {
      // Ignored
    }
  }, []);

  useEffect(() => {
    if (open) {
      refreshStatus();
      refreshBackup();
    }
  }, [open, refreshStatus, refreshBackup]);

  // Polling standby stats if in standby phase
  useEffect(() => {
    if (!open || session?.phase !== "standby") return;
    const timer = setInterval(() => {
      refreshStatus();
    }, 5000);
    return () => clearInterval(timer);
  }, [open, session?.phase, refreshStatus]);

  const handleStart = async () => {
    if (!candidateAddr.trim()) return;
    setLoading(true);
    try {
      const resp = await startMigration({
        candidate_addr: candidateAddr.trim(),
        dns_type: dnsType,
        cf_token: dnsType === "cloudflare" ? cfToken.trim() : undefined,
        cf_zone_id: dnsType === "cloudflare" ? cfZoneId.trim() : undefined,
      });
      setInstallCmd(resp.install_cmd);
      setChecks([]);
      setReady(false);
      notifySuccess(t("migration.started", "Переезд запущен. Скопируйте команду установки."));
      await refreshStatus();
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setLoading(false);
    }
  };

  const handleVerify = async () => {
    setVerifying(true);
    try {
      const resp = await verifyMigration();
      setChecks(resp.results);
      setReady(resp.ready);
      if (resp.ready) {
        notifySuccess(t("migration.verifySuccess", "Все обязательные проверки успешно пройдены!"));
      } else {
        notifyError(t("migration.verifyFailed", "Некоторые проверки не пройдены. Исправьте ошибки перед переключением."));
      }
      await refreshStatus();
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setVerifying(false);
    }
  };

  const handleSwitch = async () => {
    setSwitching(true);
    try {
      await switchMigration();
      notifySuccess(t("migration.switchSuccess", "Переключение завершено! Старый сервер перешел в режим ожидания (standby)."));
      setConfirmSwitch(false);
      await refreshStatus();
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setSwitching(false);
    }
  };

  const handleDecommission = async (force: boolean) => {
    setDecommissioning(true);
    try {
      await decommissionMigration(force);
      notifySuccess(t("migration.decommissionSuccess", "Старый сервер успешно выведен из эксплуатации!"));
      setConfirmDecommission(false);
      await refreshStatus();
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setDecommissioning(false);
    }
  };

  const handleRollback = async () => {
    setRollingBack(true);
    try {
      await rollbackMigration();
      notifySuccess(t("migration.rollbackSuccess", "Переезд отменен. Мастер разблокирован."));
      setConfirmRollback(false);
      await refreshStatus();
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setRollingBack(false);
    }
  };

  const handleTrialRestore = async () => {
    setTrialLoading(true);
    try {
      const r = await runTrialRestore();
      setTrialReport(r);
      if (r.success) {
        notifySuccess(t("migration.trialSuccess", "Тестовое восстановление успешно: база данных целостна."));
      } else {
        notifyError(r.error || t("migration.trialFailed", "Ошибка тестового восстановления"));
      }
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setTrialLoading(false);
    }
  };

  const currentPhase = session?.phase || "idle";
  const isStandby = currentPhase === "standby";
  const isCompleted = currentPhase === "completed";
  const isRolledBack = currentPhase === "rolled_back";

  return (
    <Modal open={open} onClose={onClose} title={t("migration.modalTitle", "Переезд мастер-ноды")} size="lg">
      {/* Tabs */}
      <div className="mb-4 flex border-b border-gray-200 text-sm font-medium">
        <button
          type="button"
          className={cn(
            "border-b-2 px-4 py-2 transition-colors",
            tab === "wizard"
              ? "border-brand-600 text-brand-600"
              : "border-transparent text-ink-muted hover:text-ink"
          )}
          onClick={() => setTab("wizard")}
        >
          {t("migration.tabWizard", "Мастер переезда")}
        </button>
        <button
          type="button"
          className={cn(
            "border-b-2 px-4 py-2 transition-colors",
            tab === "disaster"
              ? "border-brand-600 text-brand-600"
              : "border-transparent text-ink-muted hover:text-ink"
          )}
          onClick={() => setTab("disaster")}
        >
          {t("migration.tabDisaster", "Аварийное восстановление")}
        </button>
      </div>

      {tab === "wizard" && (
        <div className="flex flex-col gap-4 text-sm">
          {/* Phase Banner */}
          <div className="flex items-center justify-between rounded-xl bg-gray-50 p-3">
            <div>
              <span className="text-xs font-semibold uppercase tracking-wider text-ink-muted">
                {t("migration.currentStatus", "Текущий статус")}
              </span>
              <div className="mt-0.5 font-medium text-ink">
                {currentPhase === "idle" && t("migration.phaseIdle", "Готов к началу переезда")}
                {currentPhase === "prepare" && t("migration.phasePrepare", "Ожидание подключения кандидата")}
                {currentPhase === "copying" && t("migration.phaseCopying", "Синхронизация данных")}
                {currentPhase === "candidate_ready" && t("migration.phaseReady", "Кандидат готов к проверке")}
                {currentPhase === "switching" && t("migration.phaseSwitching", "Выполняется переключение...")}
                {currentPhase === "standby" && t("migration.phaseStandby", "Режим ожидания (Standby)")}
                {currentPhase === "completed" && t("migration.phaseCompleted", "Переезд успешно завершён")}
                {currentPhase === "rolled_back" && t("migration.phaseRolledBack", "Переезд отменён (откат)")}
                {currentPhase === "error" && t("migration.phaseError", "Ошибка переезда")}
              </div>
            </div>
            <Badge
              color={
                isCompleted
                  ? "green"
                  : isStandby
                  ? "orange"
                  : isRolledBack
                  ? "gray"
                  : currentPhase === "idle"
                  ? "brand"
                  : "teal"
              }
            >
              {currentPhase.toUpperCase()}
            </Badge>
          </div>

          {currentPhase === "error" && session?.last_error && (
            <div className="rounded-lg border border-danger/30 bg-red-50 p-3 text-xs text-danger">
              {session.last_error}
            </div>
          )}

          {/* Standby Mode Active View */}
          {isStandby && session && (
            <div className="flex flex-col gap-3 rounded-xl border border-amber-200 bg-amber-50/50 p-4">
              <div className="font-semibold text-amber-900">
                {t("migration.standbyActiveTitle", "Мастер переведен в режим ожидания (Standby)")}
              </div>
              <p className="text-xs text-amber-800">
                {t(
                  "migration.standbyDesc",
                  "Публичный домен перенаправлен на новый мастер. Этот сервер продолжает обслуживать клиентов, у которых закеширован старый IP. Изменяемые запросы проксируются на новый мастер."
                )}
              </p>

              <div className="grid grid-cols-2 gap-3 text-xs sm:grid-cols-4">
                <div className="rounded-lg bg-white p-2.5 shadow-sm border border-amber-100">
                  <div className="text-ink-muted">{t("migration.activeClients", "Клиентов на старом IP")}</div>
                  <div className="mt-1 text-lg font-bold text-amber-900">
                    {session.standby?.active_clients ?? 0}
                  </div>
                </div>
                <div className="rounded-lg bg-white p-2.5 shadow-sm border border-amber-100">
                  <div className="text-ink-muted">{t("migration.lastActivity", "Последняя активность")}</div>
                  <div className="mt-1 font-semibold text-ink">
                    {session.standby?.last_seen_client_at ? fmtStamp(session.standby.last_seen_client_at) : "—"}
                  </div>
                </div>
                <div className="rounded-lg bg-white p-2.5 shadow-sm border border-amber-100">
                  <div className="text-ink-muted">{t("migration.standbyTraffic", "Трафик (Up / Down)")}</div>
                  <div className="mt-1 font-semibold text-ink">
                    {fmtBytes(session.standby?.traffic_up_standby || 0)} / {fmtBytes(session.standby?.traffic_down_standby || 0)}
                  </div>
                </div>
                <div className="rounded-lg bg-white p-2.5 shadow-sm border border-amber-100">
                  <div className="text-ink-muted">{t("migration.syncErrors", "Ошибок синхронизации")}</div>
                  <div className="mt-1 font-semibold text-ink">
                    {session.standby?.sync_failures ?? 0}
                  </div>
                </div>
              </div>

              <div className="mt-2 flex flex-wrap gap-2 justify-end">
                <Button color="red" onClick={() => setConfirmDecommission(true)}>
                  {t("migration.btnDecommission", "Отключить старый сервер")}
                </Button>
                <Button variant="outline" color="gray" onClick={() => setConfirmRollback(true)}>
                  {t("migration.btnRollback", "Откатить переезд")}
                </Button>
              </div>
            </div>
          )}

          {/* Completed State */}
          {isCompleted && (
            <div className="rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-emerald-900">
              <div className="font-semibold">{t("migration.completedTitle", "Переезд полностью завершён!")}</div>
              <p className="mt-1 text-xs text-emerald-800">
                {t(
                  "migration.completedDesc",
                  "Новый мастер успешно управляет кластером. Старый сервер выведен из эксплуатации. URL подписок и пользовательские токены сохранены без изменений."
                )}
              </p>
            </div>
          )}

          {/* Wizard Steps (when not standby and not completed) */}
          {!isStandby && !isCompleted && (
            <>
              {/* Domain invariant confirmation */}
              <div className="rounded-xl border border-gray-200 bg-gray-50/70 p-3">
                <div className="flex items-center justify-between text-xs">
                  <span className="text-ink-muted">{t("migration.publicDomain", "Публичный домен (не меняется):")}</span>
                  <span className="font-mono font-semibold text-ink">{currentDomain || window.location.hostname}</span>
                </div>
                <div className="mt-1 text-[11px] text-ink-muted">
                  {t(
                    "migration.domainNotice",
                    "Контракт URL подписок https://<домен>/<путь>/<токен> гарантированно сохраняется байт-в-байт."
                  )}
                </div>
              </div>

              {/* Step 1: Configuration Form */}
              <div className="flex flex-col gap-3 rounded-xl border border-gray-200 p-4">
                <div className="font-semibold text-ink">
                  {t("migration.step1Title", "1. Параметры нового сервера и DNS")}
                </div>

                <div>
                  <TextInput
                    label={t("migration.candidateAddr", "Технический адрес кандидата (IP:порт)")}
                    placeholder="203.0.113.10:8080"
                    value={candidateAddr}
                    onChange={setCandidateAddr}
                    mono
                    disabled={currentPhase !== "idle" && currentPhase !== "rolled_back" && currentPhase !== "error"}
                  />
                  <span className="mt-1 block text-[11px] text-ink-muted">
                    {t("migration.candidateAddrHint", "Используется исключительно для передачи снимка и проверок до смены DNS.")}
                  </span>
                </div>

                <div className="mt-2">
                  <span className="block text-xs font-medium text-ink-muted mb-2">
                    {t("migration.dnsMode", "Управление DNS")}
                  </span>
                  <div className="flex flex-col gap-2">
                    <RadioCard
                      checked={dnsType === "manual"}
                      onChange={() => setDnsType("manual")}
                      label={t("migration.dnsManual", "Вручную (самостоятельное обновление A/AAAA записей)")}
                      hint={t("migration.dnsManualHint", "Потребуется изменить IP домена у вашего DNS-провайдера.")}
                      disabled={currentPhase !== "idle" && currentPhase !== "rolled_back" && currentPhase !== "error"}
                    />
                    <RadioCard
                      checked={dnsType === "cloudflare"}
                      onChange={() => setDnsType("cloudflare")}
                      label={t("migration.dnsCloudflare", "Автоматически через Cloudflare API")}
                      hint={t("migration.dnsCloudflareHint", "Автоматическое понижение TTL до 60с и мгновенное переключение.")}
                      disabled={currentPhase !== "idle" && currentPhase !== "rolled_back" && currentPhase !== "error"}
                    />
                  </div>
                </div>

                {dnsType === "cloudflare" && (
                  <div className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
                    <TextInput
                      label="Cloudflare API Token"
                      type="password"
                      placeholder="Bearer token"
                      value={cfToken}
                      onChange={setCfToken}
                      disabled={currentPhase !== "idle" && currentPhase !== "rolled_back" && currentPhase !== "error"}
                    />
                    <TextInput
                      label="Cloudflare Zone ID"
                      placeholder="32-символьный Zone ID"
                      value={cfZoneId}
                      onChange={setCfZoneId}
                      disabled={currentPhase !== "idle" && currentPhase !== "rolled_back" && currentPhase !== "error"}
                    />
                  </div>
                )}

                {(currentPhase === "idle" || currentPhase === "rolled_back" || currentPhase === "error") && (
                  <div className="mt-2 flex justify-end">
                    <Button onClick={handleStart} disabled={loading || !candidateAddr.trim()}>
                      {loading ? <Spinner size={16} /> : t("migration.btnStart", "Создать переезд")}
                    </Button>
                  </div>
                )}
              </div>

              {/* Step 2: Install Command */}
              {installCmd && (
                <div className="flex flex-col gap-2 rounded-xl border border-brand-200 bg-brand-50/30 p-4">
                  <div className="font-semibold text-brand-900">
                    {t("migration.step2Title", "2. Развёртывание на новом сервере")}
                  </div>
                  <p className="text-xs text-brand-800">
                    {t(
                      "migration.installCmdHint",
                      "Выполните эту команду на новом сервере от имени root. Токен действует 15 минут."
                    )}
                  </p>
                  <div className="mt-1">
                    <Code block copy>
                      {installCmd}
                    </Code>
                  </div>
                </div>
              )}

              {/* Step 3: Verification Checklist */}
              <div className="flex flex-col gap-3 rounded-xl border border-gray-200 p-4">
                <div className="flex items-center justify-between">
                  <div className="font-semibold text-ink">
                    {t("migration.step3Title", "3. Проверка готовности кандидата")}
                  </div>
                  <Button variant="light" color="gray" size="sm" onClick={handleVerify} disabled={verifying}>
                    {verifying ? <Spinner size={14} /> : t("migration.btnVerify", "Запустить проверки")}
                  </Button>
                </div>

                {checks.length === 0 ? (
                  <div className="text-xs text-ink-muted py-2">
                    {t("migration.noChecksYet", "Проверки ещё не запускались. Нажмите «Запустить проверки».")}
                  </div>
                ) : (
                  <div className="flex flex-col divide-y divide-gray-100 overflow-hidden rounded-lg border border-gray-200 text-xs">
                    {checks.map((c) => (
                      <div key={c.name} className="flex items-start justify-between p-2.5">
                        <div className="flex flex-col gap-0.5">
                          <div className="font-medium text-ink flex items-center gap-1.5">
                            <span>{c.name}</span>
                            {c.required && (
                              <span className="text-[10px] text-danger font-normal">
                                ({t("migration.required", "обязательно")})
                              </span>
                            )}
                          </div>
                          <div className="text-ink-muted text-[11px]">{c.details}</div>
                          {c.error && <div className="text-danger text-[11px] font-mono mt-0.5">{c.error}</div>}
                        </div>
                        <Badge color={c.passed ? "green" : c.required ? "red" : "orange"} size="xs">
                          {c.passed ? t("migration.passed", "ПРОЙДЕНО") : t("migration.failed", "ОШИБКА")}
                        </Badge>
                      </div>
                    ))}
                  </div>
                )}

                {ready && (
                  <div className="rounded-lg bg-emerald-50 border border-emerald-200 p-2.5 text-xs text-emerald-800">
                    {t(
                      "migration.readyNotice",
                      "Все ключевые проверки пройдены! Кандидат полностью готов принять нагрузку."
                    )}
                  </div>
                )}

                {/* Step 4: Switchover Button */}
                <div className="mt-2 flex justify-end">
                  <Button
                    color="brand"
                    disabled={!ready || switching}
                    onClick={() => setConfirmSwitch(true)}
                  >
                    {switching ? <Spinner size={16} /> : t("migration.btnSwitch", "Переключить на новый мастер")}
                  </Button>
                </div>
              </div>
            </>
          )}
        </div>
      )}

      {/* Disaster Recovery Tab */}
      {tab === "disaster" && (
        <div className="flex flex-col gap-4 text-sm">
          <div className="rounded-xl border border-gray-200 bg-gray-50/70 p-4">
            <div className="font-semibold text-ink mb-1">
              {t("migration.backupHealthTitle", "Состояние резервных копий")}
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3 text-xs mt-3">
              <div className="rounded-lg bg-white p-3 border border-gray-200">
                <span className="text-ink-muted">{t("migration.lastBackupDate", "Последняя копия")}</span>
                <div className="font-semibold text-ink mt-1">
                  {backupHealth?.last_backup_at ? new Date(backupHealth.last_backup_at).toLocaleString() : "—"}
                </div>
              </div>
              <div className="rounded-lg bg-white p-3 border border-gray-200">
                <span className="text-ink-muted">{t("migration.rpoEstimate", "Оценка RPO (потери)")}</span>
                <div className="font-semibold text-ink mt-1">
                  {backupHealth?.last_successful_rpo || "нет копий"}
                </div>
              </div>
              <div className="rounded-lg bg-white p-3 border border-gray-200">
                <span className="text-ink-muted">{t("migration.totalBackups", "Всего локальных копий")}</span>
                <div className="font-semibold text-ink mt-1">
                  {backupHealth?.total_local_backups ?? 0}
                </div>
              </div>
            </div>

            <div className="mt-4 flex items-center justify-between">
              <span className="text-xs text-ink-muted">
                {t("migration.trialRestoreHint", "Проверить пригодность последней копии без риска для текущей базы:")}
              </span>
              <Button size="sm" onClick={handleTrialRestore} disabled={trialLoading}>
                {trialLoading ? <Spinner size={14} /> : t("migration.btnTrialRestore", "Пробное восстановление в песочнице")}
              </Button>
            </div>

            {trialReport && (
              <div className="mt-3 rounded-lg border border-gray-200 bg-white p-3 text-xs">
                <div className="flex items-center justify-between font-semibold">
                  <span>{trialReport.archive_name}</span>
                  <Badge color={trialReport.success ? "green" : "red"} size="xs">
                    {trialReport.success ? "SQLite OK" : "CORRUPT"}
                  </Badge>
                </div>
                <div className="mt-2 grid grid-cols-2 gap-2 text-[11px] text-ink-muted">
                  <div>Пользователей в копии: <strong className="text-ink">{trialReport.users_count}</strong></div>
                  <div>Проверено таблиц: <strong className="text-ink">{trialReport.tables_checked}</strong></div>
                  <div>Версия схемы: <strong className="text-ink">{trialReport.schema_version}</strong></div>
                  <div>Размер: <strong className="text-ink">{fmtBytes(trialReport.bytes)}</strong></div>
                </div>
              </div>
            )}
          </div>

          <div className="rounded-xl border border-gray-200 p-4">
            <div className="font-semibold text-ink mb-2">
              {t("migration.disasterCmdTitle", "Аварийное развёртывание на новом сервере")}
            </div>
            <p className="text-xs text-ink-muted mb-3">
              {t(
                "migration.disasterCmdDesc",
                "Если старый мастер полностью уничтожен или недоступен, новый мастер можно поднять одной командой из сохранённой резервной копии:"
              )}
            </p>
            <Code block copy>
              {`rospanel disaster-recover --backup /var/backups/rospanel-latest.tar.gz --domain ${currentDomain || "your-domain.com"} --listen :443`}
            </Code>
            <div className="mt-2 text-[11px] text-ink-muted">
              {t(
                "migration.disasterNotice",
                "Команда восстановит SQLite, секреты, токены пользователей и запустит панель. После восстановления направьте DNS на новый IP."
              )}
            </div>
          </div>
        </div>
      )}

      {/* Switchover Confirmation Modal */}
      {confirmSwitch && (
        <Modal open onClose={() => setConfirmSwitch(false)} title={t("migration.confirmSwitchTitle", "Подтверждение переключения мастера")}>
          <div className="text-sm flex flex-col gap-3">
            <p className="text-ink">
              {t(
                "migration.confirmSwitchWarning",
                "Внимание! Старый мастер заморозит ВСЕ изменения (fencing), передаст финальный снимок данных на новый сервер, сделает его активным мастером и переключит DNS."
              )}
            </p>
            <div className="rounded-lg bg-amber-50 border border-amber-200 p-3 text-xs text-amber-900">
              {t(
                "migration.confirmSwitchNotice",
                "Старый сервер перейдёт в режим ожидания (standby) и продолжит обслуживать VPN-клиентов с закешированным IP."
              )}
            </div>
            <div className="flex justify-end gap-2 mt-2">
              <Button variant="outline" color="gray" onClick={() => setConfirmSwitch(false)}>
                {t("common.cancel", "Отмена")}
              </Button>
              <Button color="brand" onClick={handleSwitch} disabled={switching}>
                {switching ? <Spinner size={16} /> : t("migration.btnConfirmSwitch", "Подтвердить и переключить")}
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Decommission Confirmation Modal */}
      {confirmDecommission && (
        <Modal open onClose={() => setConfirmDecommission(false)} title={t("migration.confirmDecommissionTitle", "Вывод старого сервера из эксплуатации")}>
          <div className="text-sm flex flex-col gap-3">
            <p className="text-ink">
              {t(
                "migration.confirmDecommissionWarning",
                "Вы собираетесь окончательно остановить службы старого мастера."
              )}
            </p>
            {(session?.standby?.active_clients ?? 0) > 0 && (
              <div className="rounded-lg bg-red-50 border border-red-200 p-3 text-xs text-red-900">
                {t(
                  "migration.activeClientsWarning",
                  `Внимание: ${session?.standby?.active_clients} клиентов всё ещё обращаются к старому IP! При отключении их соединение будет прервано до обновления DNS.`
                )}
              </div>
            )}
            <div className="flex justify-end gap-2 mt-2">
              <Button variant="outline" color="gray" onClick={() => setConfirmDecommission(false)}>
                {t("common.cancel", "Отмена")}
              </Button>
              <Button color="red" onClick={() => handleDecommission(true)} disabled={decommissioning}>
                {decommissioning ? <Spinner size={16} /> : t("migration.btnConfirmDecommission", "Отключить окончательно")}
              </Button>
            </div>
          </div>
        </Modal>
      )}

      {/* Rollback Confirmation Modal */}
      {confirmRollback && (
        <Modal open onClose={() => setConfirmRollback(false)} title={t("migration.confirmRollbackTitle", "Откат переезда")}>
          <div className="text-sm flex flex-col gap-3">
            <p className="text-ink">
              {t(
                "migration.confirmRollbackWarning",
                "Вы уверены, что хотите отменить переезд? Старый мастер будет разблокирован и вернется к активной работе."
              )}
            </p>
            <div className="flex justify-end gap-2 mt-2">
              <Button variant="outline" color="gray" onClick={() => setConfirmRollback(false)}>
                {t("common.cancel", "Отмена")}
              </Button>
              <Button color="red" onClick={handleRollback} disabled={rollingBack}>
                {rollingBack ? <Spinner size={16} /> : t("migration.btnConfirmRollback", "Откатить")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </Modal>
  );
}
