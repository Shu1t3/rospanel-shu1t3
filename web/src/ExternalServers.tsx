import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  EMPTY_EXT_IDENTITY,
  type ExtIdentity,
  type ExtServer,
  type ExtSubscription,
  createExternal,
  deleteExternal,
  getExternal,
  setExternalEnabled,
  setExternalServerEnabled,
  setExternalServersEnabled,
  syncExternal,
  updateExternalSource,
} from "./api";
import { useAction, useShowMore } from "./hooks";
import i18n from "./i18n";
import { errMessage, notifyError, notifySuccess } from "./notify";
import {
  Badge,
  Button,
  Card,
  Modal,
  ShowMore,
  Switch,
  Textarea,
  TextInput,
  useConfirm,
} from "./ui";

function fmtWhen(unix: number): string {
  return unix ? new Date(unix * 1000).toLocaleString(i18n.language) : "—";
}

function sourceKind(source: string): "url" | "happ" | "text" {
  const s = source.trim().toLowerCase();
  if (s.startsWith("https://") || s.startsWith("http://")) return "url";
  if (s.startsWith("happ://crypt")) return "happ";
  return "text";
}

export function ExternalServers() {
  const { t } = useTranslation();
  const [subs, setSubs] = useState<ExtSubscription[] | null>(null);
  const [servers, setServers] = useState<ExtServer[]>([]);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<ExtSubscription | null>(null);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const { busy, run, isBusy } = useAction();
  const { confirm, confirmNode } = useConfirm();

  const load = () =>
    getExternal()
      .then((r) => {
        setSubs(r.subscriptions ?? []);
        setServers(r.servers ?? []);
      })
      .catch((e) => notifyError(errMessage(e)));

  // biome-ignore lint/correctness/useExhaustiveDependencies: runs once on mount; the loader is redefined every render, so listing it would refetch in a loop
  useEffect(() => {
    load();
  }, []);

  const bySub = useMemo(() => {
    const m = new Map<number, ExtServer[]>();
    for (const s of servers) {
      const cur = m.get(s.sub_id);
      if (cur) cur.push(s);
      else m.set(s.sub_id, [s]);
    }
    return m;
  }, [servers]);

  const toggleOpen = (id: number) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const sync = (sub: ExtSubscription) =>
    run(
      async () => {
        const r = await syncExternal(sub.id);
        notifySuccess(
          t("external.synced", {
            total: r.total,
            added: r.added,
            removed: r.removed,
          }),
        );
        await load();
      },
      { key: `sync-${sub.id}` },
    );

  const remove = async (sub: ExtSubscription) => {
    const ok = await confirm({
      title: t("external.deleteTitle"),
      body: t("external.deleteBody", { name: sub.name }),
      confirmLabel: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    run(async () => {
      await deleteExternal(sub.id);
      notifySuccess(t("external.deleted"));
      await load();
    });
  };

  if (subs === null) return null;

  return (
    <Card className="p-4">
      {confirmNode}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div>
          <h2 className="text-base font-semibold text-ink">{t("external.title")}</h2>
          <p className="mt-1 text-xs text-ink-muted">{t("external.hint")}</p>
        </div>
        <Button onClick={() => setAdding(true)} disabled={busy} className="self-start">
          {t("external.add")}
        </Button>
      </div>

      {subs.length > 0 && (
        <div className="mt-4 flex flex-col gap-3">
          {subs.map((s) => {
            const list = bySub.get(s.id) ?? [];
            const on = list.filter((x) => x.enabled).length;
            const kind = sourceKind(s.source);
            return (
              <div
                key={s.id}
                className="rounded-xl border border-gray-200/80 bg-gray-50/50 p-3"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-sm font-medium text-ink">
                      {s.name}
                    </span>
                    <Badge color="gray" size="xs">
                      {kind === "url"
                        ? t("external.kindUrl")
                        : kind === "happ"
                          ? t("external.kindHapp")
                          : t("external.kindText")}
                    </Badge>
                    {s.last_error ? (
                      <Badge color="orange" size="xs">
                        {t("external.readFailed")}
                      </Badge>
                    ) : (
                      <Badge color="gray" size="xs">
                        {t("external.serversOf", { on, total: list.length })}
                      </Badge>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      variant="light"
                      color="gray"
                      loading={isBusy(`sync-${s.id}`)}
                      disabled={busy}
                      onClick={() => sync(s)}
                    >
                      {t("external.sync")}
                    </Button>
                    <Button size="sm" variant="light" color="gray" disabled={busy} onClick={() => setEditing(s)}>
                      {t("common.edit")}
                    </Button>
                    <Button size="sm" variant="light" color="gray" disabled={busy} onClick={() => toggleOpen(s.id)}>
                      {t(open.has(s.id) ? "external.collapse" : "external.servers")}
                    </Button>
                    <Button size="sm" variant="light" color="red" disabled={busy} onClick={() => remove(s)}>
                      {t("common.delete")}
                    </Button>
                    <Switch
                      checked={s.enabled}
                      onChange={(v) =>
                        run(async () => {
                          await setExternalEnabled(s.id, v);
                          await load();
                        })
                      }
                    />
                  </div>
                </div>
                <p className="mt-1 text-xs text-ink-muted">
                  {kind === "url" ? (
                    <span className="break-all">{s.source}</span>
                  ) : (
                    t("external.pastedSource")
                  )}
                  {" · "}
                  {t("external.lastRead", { when: fmtWhen(s.last_fetch_at) })}
                  {s.last_error && (
                    <span className="block text-orange-600">{s.last_error}</span>
                  )}
                </p>
                {open.has(s.id) && (
                  <ServerList
                    sub={s}
                    servers={list}
                    busy={busy}
                    onToggle={(id, v) =>
                      run(async () => {
                        await setExternalServerEnabled(id, v);
                        await load();
                      })
                    }
                    onToggleAll={(v) =>
                      run(async () => {
                        await setExternalServersEnabled(s.id, v);
                        await load();
                      })
                    }
                  />
                )}
              </div>
            );
          })}
        </div>
      )}

      {editing && (
        <EditExternalDialog
          sub={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void load();
          }}
        />
      )}

      {adding && (
        <AddExternalDialog
          onClose={() => setAdding(false)}
          onAdded={() => {
            setAdding(false);
            load();
          }}
        />
      )}
    </Card>
  );
}

function ServerList({
  sub,
  servers,
  busy,
  onToggle,
  onToggleAll,
}: {
  sub: ExtSubscription;
  servers: ExtServer[];
  busy: boolean;
  onToggle: (id: number, enabled: boolean) => void;
  onToggleAll: (enabled: boolean) => void;
}) {
  const { t } = useTranslation();
  const rows = useShowMore(servers, { first: 10, step: 20, resetKey: servers });
  if (servers.length === 0) {
    return <p className="mt-3 text-sm text-ink-muted">{t("external.noServers")}</p>;
  }
  return (
    <div className="mt-3 flex flex-col gap-1">
      <div className="mb-1 flex justify-end gap-2">
        <Button size="sm" variant="light" color="gray" disabled={busy} onClick={() => onToggleAll(true)}>
          {t("external.enableAll")}
        </Button>
        <Button size="sm" variant="light" color="gray" disabled={busy} onClick={() => onToggleAll(false)}>
          {t("external.disableAll")}
        </Button>
      </div>
      {rows.shown.map((x) => (
        <div
          key={x.id}
          className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 rounded-lg border border-gray-200/70 bg-white px-3 py-1.5 text-sm"
        >
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-ink">{x.name}</span>
            <Badge color="gray" size="xs">
              {x.protocol}
            </Badge>
            <span className="truncate font-mono text-xs text-ink-muted">
              {x.host}:{x.port}
            </span>
          </div>
          <Switch checked={x.enabled} disabled={busy || !sub.enabled} onChange={(v) => onToggle(x.id, v)} />
        </div>
      ))}
      <ShowMore rest={rows.rest} onClick={rows.showMore} className="mt-1" />
    </div>
  );
}

function IdentityFields({
  source,
  value,
  onChange,
}: {
  source: string;
  value: ExtIdentity;
  onChange: (v: ExtIdentity) => void;
}) {
  const { t } = useTranslation();
  const patch = (p: Partial<ExtIdentity>) => onChange({ ...value, ...p });
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-gray-200/70 bg-gray-50/60 p-3">
      <span className="text-sm font-medium text-ink">{t("external.identity")}</span>
      <p className="text-xs text-ink-muted">{t("external.identityHint")}</p>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <TextInput
          label={t("external.hwid")}
          value={value.hwid}
          onChange={(v) => patch({ hwid: v })}
          placeholder={derivedHWIDPlaceholder(source)}
          mono
        />
        <TextInput
          label={t("external.userAgent")}
          value={value.user_agent}
          onChange={(v) => patch({ user_agent: v })}
          placeholder="rospanel/…"
        />
        <TextInput
          label={t("external.deviceOS")}
          value={value.device_os}
          onChange={(v) => patch({ device_os: v })}
          placeholder="rospanel"
        />
        <TextInput
          label={t("external.osVersion")}
          value={value.os_version}
          onChange={(v) => patch({ os_version: v })}
          placeholder={t("external.identityDefault")}
        />
        <TextInput
          label={t("external.deviceModel")}
          value={value.device_model}
          onChange={(v) => patch({ device_model: v })}
          placeholder="panel"
        />
      </div>
    </div>
  );
}

function derivedHWIDPlaceholder(source: string): string {
  const s = source.trim();
  return /^https?:\/\//i.test(s) ? i18n.t("external.hwidAuto") : "—";
}

function EditExternalDialog({
  sub,
  onClose,
  onSaved,
}: {
  sub: ExtSubscription;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [source, setSource] = useState(sub.source);
  const [identity, setIdentity] = useState<ExtIdentity>({ ...EMPTY_EXT_IDENTITY, ...sub.identity });
  const { busy, run } = useAction();

  const submit = () =>
    run(async () => {
      const r = await updateExternalSource(sub.id, source.trim(), identity);
      notifySuccess(t("external.updated", { total: r.report.total }));
      onSaved();
    });

  return (
    <Modal open onClose={onClose} title={t("external.editTitle", { name: sub.name })}>
      <div className="flex flex-col gap-4">
        <Textarea label={t("external.source")} value={source} onChange={setSource} rows={4} />
        <IdentityFields source={source} value={identity} onChange={setIdentity} />
        <div className="flex justify-end gap-2">
          <Button variant="light" color="gray" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} loading={busy} disabled={!source.trim()}>
            {t("common.save")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function AddExternalDialog({ onClose, onAdded }: { onClose: () => void; onAdded: () => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [source, setSource] = useState("");
  const [identity, setIdentity] = useState<ExtIdentity>(EMPTY_EXT_IDENTITY);
  const { busy, run } = useAction();

  const submit = () =>
    run(async () => {
      const r = await createExternal(name.trim(), source.trim(), identity);
      notifySuccess(t("external.added", { name: r.subscription.name, total: r.report.total }));
      onAdded();
    });

  return (
    <Modal open onClose={onClose} title={t("external.addTitle")}>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-ink-muted">{t("external.addHint")}</p>
        <TextInput label={t("external.name")} value={name} onChange={setName} placeholder={t("external.namePlaceholder")} />
        <Textarea
          label={t("external.source")}
          value={source}
          onChange={setSource}
          rows={5}
          placeholder={"https://provider.example/sub/…\nhapp://crypt5/…\nvless://…"}
        />
        <IdentityFields source={source} value={identity} onChange={setIdentity} />
        <div className="flex justify-end gap-2">
          <Button variant="light" color="gray" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button onClick={submit} loading={busy} disabled={!source.trim()}>
            {t("external.import")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
