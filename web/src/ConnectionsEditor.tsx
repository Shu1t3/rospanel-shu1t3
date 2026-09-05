import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  FINGERPRINTS,
  getNodeReservedPorts,
  type ConnectionsStatus,
  type ConnectionsUpdate,
  type PortInfo,
} from "./api";
import { ApplyingModal, useXrayApply } from "./apply";
import { useAction } from "./hooks";
import i18n from "./i18n";
import { errMessage, notifyError, notifySuccess } from "./notify";
import {
  Badge,
  Button,
  CenterLoader,
  cn,
  IconChevron,
  Select,
  Switch,
  TagsInput,
  TextInput,
  useConfirm,
} from "./ui";

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-sm text-ink-muted">{label}</span>
      <span className="text-right text-sm font-medium">{value}</span>
    </div>
  );
}

// LongField stacks the label over a wrapping monospace value — for long read-only
// values (keys, shortIds) that would overflow a single row on mobile.
function LongField({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-sm text-ink-muted">{label}</span>
      <code className="block break-all rounded border border-gray-200 bg-white/60 px-2 py-1 font-mono text-xs text-ink">
        {value}
      </code>
    </div>
  );
}

const FP_OPTIONS = FINGERPRINTS.map((f) => ({
  value: f,
  label: f.charAt(0).toUpperCase() + f.slice(1),
}));

const hopIntervals = () => [
  { value: "5-10", label: i18n.t("conn.sec", { range: "5–10" }) },
  { value: "10-30", label: i18n.t("conn.sec", { range: "10–30" }) },
  { value: "30-60", label: i18n.t("conn.sec", { range: "30–60" }) },
  { value: "60-120", label: i18n.t("conn.sec", { range: "60–120" }) },
];

type Hy = { port: number; start: number; end: number; interval: string };
type Reality = { port: number; dests: string[]; antiReplay: boolean };
type Anti = { fragment: boolean; min13: boolean; blockQuic: boolean };

// ConnectionsEditor is the full connection editor (protocols on/off + names +
// fingerprints + ports + hop + REALITY donor/keys/regen/port/anti-replay +
// anti-DPI + optional factory reset) for one server. Controlled: the caller
// injects how to load, save and optionally reset (master = global connections;
// a node = its own).
export function ConnectionsEditor({
  load,
  save,
  reset,
  serverId = 0,
  restartsPanel,
}: {
  load: () => Promise<ConnectionsStatus>;
  save: (u: ConnectionsUpdate) => Promise<ConnectionsStatus>;
  reset?: () => Promise<ConnectionsStatus>;
  serverId?: number;
  restartsPanel: boolean;
}) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<ConnectionsStatus | null>(null);
  const [loaded, setLoaded] = useState(false);
  const { busy, run } = useAction();
  const { applying, apply: applyXray } = useXrayApply();

  const [enabled, setEnabled] = useState<Record<string, boolean>>({});
  const [fps, setFps] = useState<Record<string, string>>({});
  const [names, setNames] = useState<Record<string, string>>({});
  const [hy, setHy] = useState<Hy>({ port: 443, start: 443, end: 443, interval: "5-10" });
  const [reality, setReality] = useState<Reality>({ port: 8443, dests: [], antiReplay: false });
  const [anti, setAnti] = useState<Anti>({ fragment: false, min13: false, blockQuic: false });
  const [regenReality, setRegenReality] = useState(false);
  const [saved, setSaved] = useState<{
    enabled: Record<string, boolean>;
    fps: Record<string, string>;
    names: Record<string, string>;
    hy: Hy;
    reality: Reality;
    anti: Anti;
  }>({
    enabled: {},
    fps: {},
    names: {},
    hy: { port: 443, start: 443, end: 443, interval: "5-10" },
    reality: { port: 8443, dests: [], antiReplay: false },
    anti: { fragment: false, min13: false, blockQuic: false },
  });
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [reservedPorts, setReservedPorts] = useState<PortInfo[]>([]);
  const { confirm, confirmNode } = useConfirm();

  const applyStatus = (s: ConnectionsStatus) => {
    setStatus(s);
    const en: Record<string, boolean> = {};
    const fp: Record<string, string> = {};
    const nm: Record<string, string> = {};
    s.protocols
      .filter((p) => p.key !== "awg")
      .forEach((p) => {
        en[p.key] = p.enabled;
        if (p.fingerprint) fp[p.key] = p.fingerprint;
        nm[p.key] = p.display_name || "";
      });
    const h: Hy = {
      port: s.hysteria_port,
      start: s.hop_start,
      end: s.hop_end,
      interval: s.hop_interval || "5-10",
    };
    const r: Reality = {
      port: s.reality_port,
      dests: s.reality_dest
        ? s.reality_dest
            .split(",")
            .map((d) => d.trim())
            .filter(Boolean)
        : [],
      antiReplay: s.reality_anti_replay,
    };
    const a: Anti = { fragment: s.tls_fragment, min13: s.tls_min13, blockQuic: s.block_quic };
    setEnabled(en);
    setFps(fp);
    setNames(nm);
    setHy(h);
    setReality(r);
    setAnti(a);
    setRegenReality(false);
    setSaved({ enabled: en, fps: fp, names: nm, hy: h, reality: r, anti: a });
  };

  useEffect(() => {
    load()
      .then(applyStatus)
      .catch((e) => notifyError(errMessage(e)))
      .finally(() => setLoaded(true));

    if (serverId > 0) {
      getNodeReservedPorts(serverId)
        .then((r) => setReservedPorts(r.ports || []))
        .catch(() => {});
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId]);

  const protocolsChanged = Object.keys(enabled).some((k) => enabled[k] !== saved.enabled[k]);
  const portsChanged = hy.port !== saved.hy.port || hy.start !== saved.hy.start || hy.end !== saved.hy.end;
  const hyChanged = portsChanged || hy.interval !== saved.hy.interval;
  const realityChanged =
    reality.port !== saved.reality.port ||
    reality.dests.join(",") !== saved.reality.dests.join(",") ||
    reality.antiReplay !== saved.reality.antiReplay;
  const fpsChanged = Object.keys(fps).some((k) => fps[k] !== saved.fps[k]);
  const namesChanged = Object.keys(names).some((k) => names[k] !== saved.names[k]);
  const antiServerChanged = anti.min13 !== saved.anti.min13;
  const antiClientChanged = anti.fragment !== saved.anti.fragment || anti.blockQuic !== saved.anti.blockQuic;
  const dirty =
    fpsChanged || namesChanged || protocolsChanged || hyChanged ||
    realityChanged || regenReality || antiServerChanged || antiClientChanged;
  // Config-affecting changes restart Xray (on the master) or re-push to the node.
  const restartsXray = protocolsChanged || portsChanged || realityChanged || regenReality || antiServerChanged;

  const cancel = () => {
    setEnabled(saved.enabled);
    setFps(saved.fps);
    setNames(saved.names);
    setHy(saved.hy);
    setReality(saved.reality);
    setAnti(saved.anti);
    setRegenReality(false);
  };

  // A reset takes the same road as a save — validated, reconciled, audited — and on
  // the master restarts Xray like any port change would.
  const doReset = async () => {
    if (!reset) return;
    const ok = await confirm({
      title: t("conn.resetTitle"),
      body: t("conn.resetBody"),
      confirmLabel: t("conn.resetConfirm"),
      danger: true,
    });
    if (!ok) return;
    const run1 = async () => {
      applyStatus(await reset());
      notifySuccess(t("conn.resetDone"));
    };
    if (restartsPanel) applyXray(run1);
    else run(run1);
  };

  const doSave = () => {
    if (!status) return;
    const curStatus = status;
    const run1 = async () => {
      const s = await save({
        protocols: {
          ...enabled,
          ...(curStatus.protocols.find((p) => p.key === "awg")
            ? { awg: curStatus.protocols.find((p) => p.key === "awg")!.enabled }
            : {}),
        },
        fingerprints: fps,
        names,
        hysteria_port: hy.port,
        hop_start: hy.start,
        hop_end: hy.end,
        hop_interval: hy.interval,
        reality_port: reality.port,
        reality_dest: reality.dests.join(","),
        reality_anti_replay: reality.antiReplay,
        regen_reality_keys: regenReality,
        tls_fragment: anti.fragment,
        tls_min13: anti.min13,
        block_quic: anti.blockQuic,
      });
      applyStatus(s);
      notifySuccess(t("common.saved"));
    };
    if (restartsPanel && restartsXray) applyXray(run1);
    else run(run1);
  };

  const setHyNum = (key: keyof Hy) => (v: string) =>
    setHy((h) => ({ ...h, [key]: Number(v.replace(/\D/g, "")) || 0 }));

  if (!loaded) return <CenterLoader />;
  if (!status) return null;

  return (
    <div className="flex flex-col gap-3">
      {/* Reserved Ports Summary (if any, for rented nodes) */}
      {reservedPorts.length > 0 && (
        <div className="rounded-xl border border-indigo-200 bg-indigo-50/70 p-3.5 text-xs dark:border-indigo-900/50 dark:bg-indigo-950/30">
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <span className="font-bold text-indigo-950 dark:text-indigo-100">
              🛡️ {t("nodes.reservedPorts")} ({reservedPorts.length})
            </span>
            <span className="text-[11px] text-ink-muted">{t("nodes.reservedPortsHint")}</span>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {reservedPorts.map((pi) => (
              <span
                key={`${pi.port}-${pi.protocol}`}
                className={cn(
                  "inline-flex items-center gap-1 rounded-md px-2 py-0.5 font-mono text-[11px]",
                  pi.is_owner
                    ? "bg-amber-100/80 text-amber-900 dark:bg-amber-950/60 dark:text-amber-200"
                    : "bg-indigo-100/80 text-indigo-900 dark:bg-indigo-950/60 dark:text-indigo-200",
                )}
                title={pi.is_owner ? t("nodes.portOwner") : t("nodes.portTenant")}
              >
                <strong>{pi.port}</strong>/{pi.protocol.toUpperCase()} ({pi.service || (pi.is_owner ? t("nodes.portOwner") : t("nodes.portTenant"))})
              </span>
            ))}
          </div>
        </div>
      )}

      {/* Built-in protocols list */}
      <div className="grid grid-cols-1 gap-3">
        {status.protocols.filter((p) => p.key !== "awg").map((p) => {
          const isOpen = !!open[p.key];
          const on = !!enabled[p.key];
          return (
            <div
              key={p.key}
              className="overflow-hidden rounded-xl border border-gray-200/80 bg-gray-50/60"
            >
              <button
                type="button"
                onClick={() => setOpen((o) => ({ ...o, [p.key]: !o[p.key] }))}
                className="flex w-full items-center justify-between gap-2 p-4 text-left"
              >
                <div className="flex min-w-0 items-center gap-2">
                  <IconChevron
                    className={`shrink-0 text-gray-400 transition-transform ${isOpen ? "rotate-180" : ""}`}
                  />
                  <span className="font-medium text-ink">{p.name}</span>
                  <Badge color="gray">{p.port}</Badge>
                  {!on && <Badge color="gray">{t("conn.off")}</Badge>}
                </div>
                <span onClick={(e) => e.stopPropagation()} className="flex items-center">
                  <Switch checked={on} onChange={(v) => setEnabled((e) => ({ ...e, [p.key]: v }))} />
                </span>
              </button>

              {isOpen && (
                <div className="flex flex-col gap-3 border-t border-gray-100 px-4 pb-4 pt-3">
                  <div className="flex flex-col gap-2">
                    <TextInput
                      label={t("conn.name")}
                      value={names[p.key] ?? ""}
                      onChange={(v) => setNames((n) => ({ ...n, [p.key]: v }))}
                      placeholder={p.name}
                    />
                    <p className="text-xs text-ink-muted">
                      {t("conn.nameHint", { name: p.name })}
                    </p>
                  </div>

                  <div className="flex flex-col gap-1 border-t border-gray-100 pt-3">
                    <Field label={t("conn.transport")} value={p.transport} />
                    <Field label={t("conn.security")} value={p.security} />
                    {p.note && <Field label={t("conn.note")} value={p.note} />}
                  </div>

                  {p.fingerprint && (
                    <div className="border-t border-gray-100 pt-3">
                      <Select
                        label="Fingerprint (uTLS)"
                        data={FP_OPTIONS}
                        value={fps[p.key] ?? "firefox"}
                        onChange={(v) => setFps((f) => ({ ...f, [p.key]: v }))}
                      />
                      <p className="mt-2 text-xs text-ink-muted">
                        {t("conn.fpHint")}
                      </p>
                    </div>
                  )}

                  {p.key === "hysteria2" &&
                    (on ? (
                      <div className="flex flex-col gap-3 border-t border-gray-100 pt-3">
                        <div className="grid grid-cols-3 gap-2">
                          <TextInput label={t("conn.port")} type="number" value={String(hy.port)} onChange={setHyNum("port")} />
                          <TextInput label={t("conn.hopFrom")} type="number" value={String(hy.start)} onChange={setHyNum("start")} />
                          <TextInput label={t("conn.hopTo")} type="number" value={String(hy.end)} onChange={setHyNum("end")} />
                        </div>
                        <Select
                          label={t("conn.hopInterval")}
                          data={hopIntervals()}
                          value={hy.interval}
                          onChange={(v) => setHy((h) => ({ ...h, interval: v }))}
                        />
                        <p className="text-xs text-ink-muted">
                          {t("conn.hopHint")}
                        </p>
                      </div>
                    ) : (
                      <p className="border-t border-gray-100 pt-3 text-xs text-ink-muted">
                        {t("conn.enableHysteria")}
                      </p>
                    ))}

                  {p.key === "reality" &&
                    (on ? (
                      <div className="flex flex-col gap-3 border-t border-gray-100 pt-3">
                        <TextInput
                          label={t("conn.port")}
                          type="number"
                          value={String(reality.port)}
                          onChange={(v) => setReality((r) => ({ ...r, port: Number(v.replace(/\D/g, "")) || 0 }))}
                        />
                        <TagsInput
                          label={t("conn.masquerade")}
                          value={reality.dests}
                          onChange={(v) => setReality((r) => ({ ...r, dests: v }))}
                          placeholder={t("conn.sniPlaceholder")}
                        />
                        <div
                          className="flex cursor-pointer items-center justify-between gap-3 select-none"
                          onClick={() => setReality((r) => ({ ...r, antiReplay: !r.antiReplay }))}
                        >
                          <span className="text-sm">
                            {t("conn.antiReplay")}
                            <span className="block text-xs text-ink-muted">
                              {t("conn.antiReplayHint")}
                            </span>
                          </span>
                          <Switch
                            checked={reality.antiReplay}
                            onChange={(v) => setReality((r) => ({ ...r, antiReplay: v }))}
                            aria-label={t("conn.antiReplay")}
                          />
                        </div>
                        <LongField label="Public key" value={status.reality_public_key} />
                        <LongField label="Short IDs" value={status.reality_short_id} />
                        <LongField label={t("conn.xhttpPath")} value={status.reality_path} />
                        <div>
                          <Button
                            size="sm"
                            variant="light"
                            color={regenReality ? "orange" : "gray"}
                            onClick={() => setRegenReality((v) => !v)}
                          >
                            {t(regenReality ? "conn.keysWillRegen" : "conn.regenKeys")}
                          </Button>
                        </div>
                        <p className="text-xs text-ink-muted">
                          {t("conn.realityHint")}
                        </p>
                      </div>
                    ) : (
                      <p className="border-t border-gray-100 pt-3 text-xs text-ink-muted">
                        {t("conn.enableReality")}
                      </p>
                    ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* Anti-DPI transport hardening */}
      <div className="rounded-xl border border-gray-200/80 bg-gray-50/60 p-4">
        <h3 className="mb-1 font-bold text-ink">{t("conn.antiDpi")}</h3>
        <p className="mb-3 text-sm text-ink-muted">
          {t("conn.antiDpiHint")}
        </p>
        <div className="flex flex-col divide-y divide-gray-100">
          <div
            className="flex cursor-pointer items-center justify-between gap-3 py-3 first:pt-0 select-none"
            onClick={() => setAnti((a) => ({ ...a, fragment: !a.fragment }))}
          >
            <span className="text-sm">
              {t("conn.fragment")}
              <span className="block text-xs text-ink-muted">
                {t("conn.fragmentHint")}
                (VLESS-Vision).
              </span>
            </span>
            <Switch
              checked={anti.fragment}
              onChange={(v) => setAnti((a) => ({ ...a, fragment: v }))}
              aria-label={t("conn.fragment")}
            />
          </div>
          <div
            className="flex cursor-pointer items-center justify-between gap-3 py-3 select-none"
            onClick={() => setAnti((a) => ({ ...a, blockQuic: !a.blockQuic }))}
          >
            <span className="text-sm">
              {t("conn.blockQuic")}
              <span className="block text-xs text-ink-muted">
                {t("conn.blockQuicHint")}
              </span>
            </span>
            <Switch
              checked={anti.blockQuic}
              onChange={(v) => setAnti((a) => ({ ...a, blockQuic: v }))}
              aria-label={t("conn.blockQuic")}
            />
          </div>
          <div
            className="flex cursor-pointer items-center justify-between gap-3 py-3 last:pb-0 select-none"
            onClick={() => setAnti((a) => ({ ...a, min13: !a.min13 }))}
          >
            <span className="text-sm">
              {t("conn.requireTls13")}
              <span className="block text-xs text-ink-muted">
                {t("conn.requireTls13Hint")}
              </span>
            </span>
            <Switch
              checked={anti.min13}
              onChange={(v) => setAnti((a) => ({ ...a, min13: v }))}
              aria-label={t("conn.requireTls13")}
            />
          </div>
        </div>
      </div>

      {reset && (
        <div className="rounded-xl border border-red-200/70 bg-red-50/40 p-4">
          <h3 className="font-bold text-ink">{t("conn.resetTitle")}</h3>
          <p className="mt-0.5 text-sm text-ink-muted">{t("conn.resetHint")}</p>
          {/* Below the text, not beside it: beside, the row squeezes the button into
              three lines as soon as the hint is longer than a sentence. */}
          <div className="mt-3 flex justify-end">
            <Button variant="light" color="red" className="whitespace-nowrap" onClick={doReset} disabled={busy || applying}>
              {t("conn.reset")}
            </Button>
          </div>
        </div>
      )}

      {confirmNode}
      <div className="flex flex-col gap-2 border-t border-gray-100 pt-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-xs text-ink-muted">
          {t("conn.saveHint")}
        </p>
        <div className="flex justify-end gap-2">
          <Button
            variant="light"
            color="gray"
            onClick={cancel}
            disabled={!dirty || busy || applying}
          >
            {t("common.cancel")}
          </Button>
          <Button onClick={doSave} loading={busy || applying} disabled={!dirty}>
            {t("common.save")}
          </Button>
        </div>
      </div>
      <ApplyingModal open={applying} />
    </div>
  );
}
