import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  applyConnections,
  applyNodeConnections,
  getConnections,
  getNodeConnections,
  type AWGParams,
  type ConnectionsStatus,
  type ConnectionsUpdate,
} from "./api";
import { ApplyingModal, useXrayApply } from "./apply";
import {
  AWG_PRESET_OPTIONS,
  generateAwgPresetSignatures,
  type AwgPresetId,
} from "./awgPresets";
import { useAction } from "./hooks";
import { errMessage, notifyError, notifySuccess } from "./notify";
import { Badge, Button, CenterLoader, IconChevron, Select, Switch, TextInput } from "./ui";

function LongField({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-sm text-ink-muted">{label}</span>
      <code className="block break-all rounded border border-gray-200 bg-white/60 px-2 py-1 font-mono text-xs text-ink dark:border-neutral-800 dark:bg-neutral-900/60 dark:text-neutral-200">
        {value}
      </code>
    </div>
  );
}

export function AwgEditor({
  serverId = 0,
  restartsPanel,
  readOnly = false,
}: {
  serverId: number;
  restartsPanel: boolean;
  readOnly?: boolean;
}) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<ConnectionsStatus | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [open, setOpen] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);

  const { busy, run } = useAction();
  const { applying, apply: applyXray } = useXrayApply();

  const [enabled, setEnabled] = useState(false);
  const [port, setPort] = useState(0);
  const [dns, setDns] = useState("");
  const [params, setParams] = useState<AWGParams>({
    jc: 4,
    jmin: 50,
    jmax: 1000,
    s1: 64,
    s2: 70,
    s3: 80,
    s4: 20,
    h1: "",
    h2: "",
    h3: "",
    h4: "",
    i1: "",
    i2: "",
    i3: "",
    i4: "",
    i5: "",
    content_padding_addition: "0-32",
    random_trailers: true,
    disable_cookies: true,
    rekey_after_time: "110-130",
    rekey_timeout: "4-6",
    reject_after_time: "175-195",
    keepalive_timeout: "12-18",
    max_handshake_attempts: "10-15",
  });
  const [regenKeys, setRegenKeys] = useState(false);
  const [selectedPreset, setSelectedPreset] = useState<AwgPresetId>("none");

  // Saved baseline for dirty check
  const [saved, setSaved] = useState<{
    enabled: boolean;
    port: number;
    dns: string;
    params: AWGParams;
  } | null>(null);

  const loadStatus = async () => {
    try {
      const s = serverId > 0 ? await getNodeConnections(serverId) : await getConnections();
      setStatus(s);
      const isEn = s.protocols.find((p) => p.key === "awg")?.enabled ?? false;
      setEnabled(isEn);
      setPort(s.awg_port || 0);
      setDns(s.awg_dns || "");
      if (s.awg_params && !s.awg_params.jc && !s.awg_params.h1) {
        // Uninitialized
      } else if (s.awg_params) {
        setParams({
          ...s.awg_params,
          h1: String(s.awg_params.h1 || ""),
          h2: String(s.awg_params.h2 || ""),
          h3: String(s.awg_params.h3 || ""),
          h4: String(s.awg_params.h4 || ""),
        });
      }
      setRegenKeys(false);
      setSaved({
        enabled: isEn,
        port: s.awg_port || 0,
        dns: s.awg_dns || "",
        params: s.awg_params,
      });
    } catch (e) {
      notifyError(errMessage(e));
    } finally {
      setLoaded(true);
    }
  };

  useEffect(() => {
    loadStatus();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  const onSelectPreset = (presetId: string) => {
    const id = presetId as AwgPresetId;
    setSelectedPreset(id);
    const signatures = generateAwgPresetSignatures(id);
    setParams((p) => ({
      ...p,
      i1: signatures.i1,
      i2: signatures.i2,
      i3: signatures.i3,
      i4: signatures.i4,
      i5: signatures.i5,
    }));
  };

  const isDirty =
    !saved ||
    enabled !== saved.enabled ||
    port !== saved.port ||
    dns !== saved.dns ||
    regenKeys ||
    JSON.stringify(params) !== JSON.stringify(saved.params);

  const handleSave = () => {
    if (readOnly) return;
    const task = async () => {
      // Build full update payload based on current status
      if (!status) return;
      const protoMap: Record<string, boolean> = {};
      status.protocols.forEach((p) => {
        protoMap[p.key] = p.key === "awg" ? enabled : p.enabled;
      });

      const updatePayload: ConnectionsUpdate = {
        protocols: protoMap,
        fingerprints: {},
        names: {},
        hysteria_port: status.hysteria_port,
        hop_start: status.hop_start,
        hop_end: status.hop_end,
        hop_interval: status.hop_interval || "5-10",
        reality_port: status.reality_port,
        reality_dest: status.reality_dest,
        reality_anti_replay: status.reality_anti_replay,
        regen_reality_keys: false,
        tls_fragment: status.tls_fragment,
        tls_min13: status.tls_min13,
        block_quic: status.block_quic,
        awg_port: port,
        awg_dns: dns,
        awg_params: params,
        regen_awg_keys: regenKeys,
      };

      const updated =
        serverId > 0
          ? await applyNodeConnections(serverId, updatePayload)
          : await applyConnections(updatePayload);

      setStatus(updated);
      const isEn = updated.protocols.find((p) => p.key === "awg")?.enabled ?? false;
      setEnabled(isEn);
      setPort(updated.awg_port || 0);
      setDns(updated.awg_dns || "");
      if (updated.awg_params) {
        setParams({
          ...updated.awg_params,
          h1: String(updated.awg_params.h1 || ""),
          h2: String(updated.awg_params.h2 || ""),
          h3: String(updated.awg_params.h3 || ""),
          h4: String(updated.awg_params.h4 || ""),
        });
      }
      setRegenKeys(false);
      setSaved({
        enabled: isEn,
        port: updated.awg_port || 0,
        dns: updated.awg_dns || "",
        params: updated.awg_params,
      });
      notifySuccess(t("common.saved"));
    };

    if (restartsPanel) applyXray(task);
    else run(task);
  };

  if (!loaded) return <CenterLoader />;
  if (!status) return null;

  const presetOptions = AWG_PRESET_OPTIONS.map((opt) => ({
    value: opt.id as string,
    label: String(t(opt.labelKey as any)),
  }));

  const awgRunning = status.awg_running;
  const awgError = status.awg_error;

  return (
    <div className="overflow-hidden rounded-xl border border-gray-200/80 bg-gray-50/60 shadow-sm transition dark:border-neutral-800 dark:bg-neutral-900/60">
      {/* Header */}
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center justify-between gap-2 p-4 text-left hover:bg-gray-100/50 dark:hover:bg-neutral-800/40"
      >
        <div className="flex min-w-0 items-center gap-2">
          <IconChevron
            className={`shrink-0 text-gray-400 transition-transform ${open ? "rotate-180" : ""}`}
          />
          <span className="font-semibold text-ink">AmneziaWG</span>
          <Badge color="purple">AWG 3.1</Badge>
          {port > 0 ? (
            <Badge color="gray">{port}</Badge>
          ) : (
            <Badge color="gray">{t("conn.awgPortAuto")}</Badge>
          )}
          {!enabled && <Badge color="gray">{t("conn.off")}</Badge>}
          {enabled && serverId === 0 && (
            <Badge color={awgRunning ? "green" : awgError ? "red" : "orange"}>
              {awgRunning ? t("awg.running") : awgError ? t("awg.error") : t("awg.stopped")}
            </Badge>
          )}
        </div>
        {!readOnly && (
          <span onClick={(e) => e.stopPropagation()} className="flex items-center">
            <Switch checked={enabled} onChange={(v) => setEnabled(v)} />
          </span>
        )}
      </button>

      {/* Expanded details */}
      {open && (
        <div className="flex flex-col gap-3.5 border-t border-gray-100 px-4 pb-4 pt-3.5 dark:border-neutral-800">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <TextInput
              label={t("conn.awgPort")}
              type="number"
              value={port ? String(port) : ""}
              placeholder={t("conn.awgPortAuto")}
              disabled={readOnly}
              onChange={(v) => setPort(Number(v.replace(/\D/g, "")) || 0)}
            />
            <TextInput
              label={t("conn.awgDns")}
              value={dns}
              placeholder={t("conn.awgDnsAuto")}
              disabled={readOnly}
              onChange={(v) => setDns(v)}
            />
          </div>

          {status.awg_public_key && (
            <LongField label={t("awg.serverPublicKey")} value={status.awg_public_key} />
          )}

          {awgError && (
            <p className="warning-tint rounded-lg px-2.5 py-1.5 text-xs text-warning">
              {awgError}
            </p>
          )}

          {/* Quick Obfuscation Summary */}
          {status.awg_params && (
            <div className="rounded-lg border border-gray-200/70 bg-white/70 p-3 text-xs dark:border-neutral-800 dark:bg-neutral-900/50">
              <div className="mb-1.5 font-bold text-ink">{t("awg.activeParams")}</div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-1 font-mono text-[11px] sm:grid-cols-4">
                <span>Jc: <strong>{params.jc}</strong></span>
                <span>Jmin-max: <strong>{params.jmin}-{params.jmax}</strong></span>
                <span>S1-S2: <strong>{params.s1}, {params.s2}</strong></span>
                <span>S3-S4: <strong>{params.s3 ?? 0}, {params.s4 ?? 0}</strong></span>
                <span>H1: <strong>{String(params.h1)}</strong></span>
                <span>H2: <strong>{String(params.h2)}</strong></span>
                <span>H3: <strong>{String(params.h3)}</strong></span>
                <span>H4: <strong>{String(params.h4)}</strong></span>
              </div>
            </div>
          )}

          {/* Advanced Obfuscation Settings toggle */}
          <div className="border-t border-gray-100 pt-2 dark:border-neutral-800">
            <button
              type="button"
              onClick={() => setAdvancedOpen((v) => !v)}
              className="flex items-center gap-1.5 text-xs font-semibold text-indigo-600 hover:text-indigo-700 dark:text-indigo-400"
            >
              <IconChevron
                className={`size-3.5 transition-transform ${advancedOpen ? "rotate-180" : ""}`}
              />
              {advancedOpen ? t("awg.hideAdvanced") : t("awg.showAdvanced")}
            </button>
          </div>

          {advancedOpen && (
            <div className="flex flex-col gap-3 rounded-lg border border-gray-200/70 bg-gray-50/50 p-3.5 dark:border-neutral-800 dark:bg-neutral-950/40">
              {/* Junk & Padding */}
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <TextInput
                  label="Jc (Junk packets)"
                  type="number"
                  value={String(params.jc)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, jc: Number(v) || 0 }))}
                />
                <TextInput
                  label="Jmin (Bytes)"
                  type="number"
                  value={String(params.jmin)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, jmin: Number(v) || 0 }))}
                />
                <TextInput
                  label="Jmax (Bytes)"
                  type="number"
                  value={String(params.jmax)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, jmax: Number(v) || 0 }))}
                />
                <TextInput
                  label="Padding addition"
                  value={params.content_padding_addition || ""}
                  placeholder="0-32"
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, content_padding_addition: v }))}
                />
              </div>

              {/* Handshake & Transport paddings */}
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <TextInput
                  label="S1 (Init padding)"
                  type="number"
                  value={String(params.s1)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, s1: Number(v) || 0 }))}
                />
                <TextInput
                  label="S2 (Resp padding)"
                  type="number"
                  value={String(params.s2)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, s2: Number(v) || 0 }))}
                />
                <TextInput
                  label="S3 (Cookie padding)"
                  type="number"
                  value={String(params.s3 ?? 0)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, s3: Number(v) || 0 }))}
                />
                <TextInput
                  label="S4 (Transport pad)"
                  type="number"
                  value={String(params.s4 ?? 0)}
                  disabled={readOnly}
                  onChange={(v) => setParams((p) => ({ ...p, s4: Number(v) || 0 }))}
                />
              </div>

              {/* Headers ranges */}
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <TextInput
                  label="H1 (Handshake Init)"
                  value={String(params.h1)}
                  disabled={readOnly}
                  placeholder="e.g. 1000-5000"
                  onChange={(v) => setParams((p) => ({ ...p, h1: v }))}
                />
                <TextInput
                  label="H2 (Handshake Resp)"
                  value={String(params.h2)}
                  disabled={readOnly}
                  placeholder="e.g. 6000-12000"
                  onChange={(v) => setParams((p) => ({ ...p, h2: v }))}
                />
                <TextInput
                  label="H3 (Cookie Reply)"
                  value={String(params.h3)}
                  disabled={readOnly}
                  placeholder="e.g. 13000-19000"
                  onChange={(v) => setParams((p) => ({ ...p, h3: v }))}
                />
                <TextInput
                  label="H4 (Transport)"
                  value={String(params.h4)}
                  disabled={readOnly}
                  placeholder="e.g. 20000-30000"
                  onChange={(v) => setParams((p) => ({ ...p, h4: v }))}
                />
              </div>

              {/* Security Booleans */}
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <label className="flex items-center justify-between gap-2 rounded-lg border border-gray-200/80 bg-white/60 p-2.5 dark:border-neutral-800 dark:bg-neutral-900/60">
                  <div>
                    <span className="text-xs font-semibold text-ink">{t("awg.randomTrailers")}</span>
                    <span className="block text-[11px] text-ink-muted">{t("awg.randomTrailersHint")}</span>
                  </div>
                  <Switch
                    checked={params.random_trailers ?? true}
                    onChange={(v) => setParams((p) => ({ ...p, random_trailers: v }))}
                  />
                </label>
                <label className="flex items-center justify-between gap-2 rounded-lg border border-gray-200/80 bg-white/60 p-2.5 dark:border-neutral-800 dark:bg-neutral-900/60">
                  <div>
                    <span className="text-xs font-semibold text-ink">{t("awg.disableCookies")}</span>
                    <span className="block text-[11px] text-ink-muted">{t("awg.disableCookiesHint")}</span>
                  </div>
                  <Switch
                    checked={params.disable_cookies ?? true}
                    onChange={(v) => setParams((p) => ({ ...p, disable_cookies: v }))}
                  />
                </label>
              </div>

              {/* I1–I5 Custom Signatures & Presets */}
              <div className="flex flex-col gap-2 border-t border-gray-200/70 pt-3 dark:border-neutral-800">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <h4 className="text-xs font-bold text-ink">{t("awg.cpsTitle")}</h4>
                    <p className="text-[11px] text-ink-muted">{t("awg.cpsHint")}</p>
                  </div>
                  <div className="w-56">
                    <Select
                      label={t("awg.presetLabel")}
                      data={presetOptions}
                      value={selectedPreset}
                      onChange={onSelectPreset}
                    />
                  </div>
                </div>

                <div className="flex flex-col gap-1.5 pt-1">
                  <TextInput
                    label="I1"
                    value={params.i1 || ""}
                    placeholder="<b 0x...><r 20><t>"
                    disabled={readOnly}
                    onChange={(v) => setParams((p) => ({ ...p, i1: v }))}
                  />
                  <TextInput
                    label="I2"
                    value={params.i2 || ""}
                    placeholder="<b 0x...><r 10><rc 4>"
                    disabled={readOnly}
                    onChange={(v) => setParams((p) => ({ ...p, i2: v }))}
                  />
                  <TextInput
                    label="I3"
                    value={params.i3 || ""}
                    placeholder="<t><r 30>"
                    disabled={readOnly}
                    onChange={(v) => setParams((p) => ({ ...p, i3: v }))}
                  />
                  <TextInput
                    label="I4"
                    value={params.i4 || ""}
                    placeholder="<b 0x...><rd 6>"
                    disabled={readOnly}
                    onChange={(v) => setParams((p) => ({ ...p, i4: v }))}
                  />
                  <TextInput
                    label="I5"
                    value={params.i5 || ""}
                    placeholder="<r 16><t>"
                    disabled={readOnly}
                    onChange={(v) => setParams((p) => ({ ...p, i5: v }))}
                  />
                </div>
              </div>
            </div>
          )}

          {/* Action Footer */}
          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 dark:border-neutral-800">
            {!readOnly && (
              <Button
                size="sm"
                variant="light"
                color={regenKeys ? "orange" : "gray"}
                onClick={() => setRegenKeys((v) => !v)}
              >
                {t(regenKeys ? "conn.keysWillRegen" : "conn.regenKeys")}
              </Button>
            )}

            {!readOnly && (
              <Button
                size="sm"
                onClick={handleSave}
                loading={busy || applying}
                disabled={!isDirty}
              >
                {t("common.save")}
              </Button>
            )}
          </div>
        </div>
      )}

      <ApplyingModal open={applying} />
    </div>
  );
}
