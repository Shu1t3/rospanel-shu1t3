import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { QRCodeSVG } from "qrcode.react";
import {
  createMTProto,
  deleteMTProto,
  generateMTProtoSecret,
  getMTProto,
  setMTProtoEnabled,
  updateMTProto,
  type MTProtoProxy,
} from "./api";
import { useAction } from "./hooks";
import { errMessage, notifyError, notifySuccess } from "./notify";
import {
  Badge,
  Button,
  Card,
  Modal,
  Switch,
  TextInput,
  useConfirm,
} from "./ui";

function fmtBytes(b?: number): string {
  if (!b || b <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = b;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

function fmtUptime(sec?: number): string {
  if (!sec || sec <= 0) return "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}д ${h}ч`;
  if (h > 0) return `${h}ч ${m}м`;
  return `${m}м`;
}

export function MTProtoServers() {
  const { t } = useTranslation();
  const [proxies, setProxies] = useState<MTProtoProxy[] | null>(null);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<MTProtoProxy | null>(null);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const [qrProxy, setQrProxy] = useState<MTProtoProxy | null>(null);
  const { busy, run } = useAction();
  const { confirm, confirmNode } = useConfirm();

  const load = () =>
    getMTProto()
      .then((r) => setProxies(r.proxies ?? []))
      .catch((e) => notifyError(errMessage(e)));

  useEffect(() => {
    load();
  }, []);

  if (proxies === null) return null;

  const toggleOpen = (id: number) =>
    setOpen((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const remove = async (p: MTProtoProxy) => {
    const ok = await confirm({
      title: t("mtproto.deleteTitle"),
      body: t("mtproto.deleteBody", { name: p.name }),
      confirmLabel: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    run(async () => {
      await deleteMTProto(p.id);
      notifySuccess(t("mtproto.deleted"));
      await load();
    });
  };

  const copyText = (text: string, msg: string) => {
    navigator.clipboard.writeText(text);
    notifySuccess(msg);
  };

  return (
    <Card className="p-4">
      {confirmNode}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-bold text-ink">{t("mtproto.title")}</h3>
            <Badge color="gray" size="xs">
              {proxies.length}
            </Badge>
          </div>
          <p className="mt-0.5 text-sm text-ink-muted">{t("mtproto.hint")}</p>
        </div>
        <Button variant="light" color="gray" onClick={() => setAdding(true)}>
          {t("mtproto.add")}
        </Button>
      </div>

      {proxies.length > 0 && (
        <div className="mt-4 flex flex-col gap-3">
          {proxies.map((p) => {
            const isMixed = (p.node_id !== undefined && p.node_id !== null) || p.id < 0;
            const link = p.link || `tg://proxy?server=${p.host}&port=${p.port}&secret=${p.secret}`;
            const httpsLink = p.https_link || `https://t.me/proxy?server=${p.host}&port=${p.port}&secret=${p.secret}`;
            const isOpen = open.has(p.id);

            return (
              <div key={p.id} className="rounded-xl border border-gray-200/80 bg-gray-50/60 p-3">
                <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <span className="truncate font-medium text-ink">{p.name}</span>
                    <Badge color={p.running ? "green" : "gray"} size="xs">
                      {t(p.running ? "mtproto.running" : "mtproto.stopped")}
                    </Badge>
                    <Badge color="gray" size="xs">
                      {isMixed ? t("mtproto.mixedMode") : t("mtproto.standalone")}
                    </Badge>
                    <span className="text-xs text-ink-muted">
                      {p.host}:{p.port}
                    </span>
                    {p.active_conns !== undefined && p.active_conns > 0 && (
                      <Badge color="brand" size="xs">
                        {p.active_conns} {t("mtproto.activeConns").toLowerCase()}
                      </Badge>
                    )}
                  </div>

                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      variant="light"
                      color="gray"
                      disabled={busy}
                      onClick={() => toggleOpen(p.id)}
                    >
                      {t(isOpen ? "mtproto.collapse" : "mtproto.settingsBtn")}
                    </Button>
                    <Button
                      size="sm"
                      variant="light"
                      color="brand"
                      onClick={() => copyText(link, t("mtproto.copied"))}
                    >
                      {t("mtproto.copyLink")}
                    </Button>
                    <Button
                      size="sm"
                      variant="light"
                      color="gray"
                      onClick={() => setQrProxy(p)}
                    >
                      {t("mtproto.qrCode")}
                    </Button>
                    {!isMixed && (
                      <>
                        <Button
                          size="sm"
                          variant="light"
                          color="gray"
                          disabled={busy}
                          onClick={() => setEditing(p)}
                        >
                          {t("common.edit")}
                        </Button>
                        <Button
                          size="sm"
                          variant="light"
                          color="red"
                          disabled={busy}
                          onClick={() => remove(p)}
                        >
                          {t("common.delete")}
                        </Button>
                      </>
                    )}
                    <Switch
                      checked={p.enabled}
                      onChange={(v) =>
                        run(async () => {
                          await setMTProtoEnabled(p.id, v);
                          await load();
                        })
                      }
                    />
                  </div>
                </div>

                {isOpen && (
                  <div className="mt-3 flex flex-col gap-2 rounded-lg border border-gray-200 bg-white p-3 text-xs text-ink">
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-4">
                      <div>
                        <span className="text-ink-muted">{t("mtproto.domain")}:</span>{" "}
                        <span className="font-mono">{p.domain}</span>
                      </div>
                      <div>
                        <span className="text-ink-muted">{t("mtproto.maxConns")}:</span>{" "}
                        <span>{p.max_conns}</span>
                      </div>
                      <div>
                        <span className="text-ink-muted">{t("mtproto.uptime")}:</span>{" "}
                        <span>{fmtUptime(p.uptime_sec)}</span>
                      </div>
                      <div>
                        <span className="text-ink-muted">{t("mtproto.memory")}:</span>{" "}
                        <span>{fmtBytes(p.rss)} RSS</span>
                      </div>
                      <div className="sm:col-span-2">
                        <span className="text-ink-muted">{t("mtproto.traffic")}:</span>{" "}
                        <span>
                          ↑ {fmtBytes(p.bytes_read)} · ↓ {fmtBytes(p.bytes_written)}
                        </span>
                      </div>
                      <div className="sm:col-span-2">
                        <span className="text-ink-muted">{t("mtproto.secret")}:</span>{" "}
                        <span className="break-all font-mono">{p.secret}</span>
                      </div>
                    </div>

                    <div className="mt-2 flex flex-wrap gap-2 pt-2 border-t border-gray-100">
                      <Button
                        size="xs"
                        variant="light"
                        color="gray"
                        onClick={() => copyText(httpsLink, t("mtproto.copied"))}
                      >
                        {t("mtproto.copyHTTPS")}
                      </Button>
                      <Button
                        size="xs"
                        variant="light"
                        color="gray"
                        onClick={() => copyText(p.secret, t("common.copied"))}
                      >
                        {t("mtproto.secret")}
                      </Button>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {adding && (
        <AddMTProtoDialog
          onClose={() => setAdding(false)}
          onAdded={() => {
            setAdding(false);
            load();
          }}
        />
      )}

      {editing && (
        <EditMTProtoDialog
          proxy={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            load();
          }}
        />
      )}

      {qrProxy && (
        <Modal
          title={`${qrProxy.name} · QR-код`}
          open={true}
          onClose={() => setQrProxy(null)}
        >
          <div className="flex flex-col items-center gap-4 py-2">
            <div className="rounded-xl border border-gray-200 bg-white p-4 shadow-sm">
              <QRCodeSVG
                value={qrProxy.link || `tg://proxy?server=${qrProxy.host}&port=${qrProxy.port}&secret=${qrProxy.secret}`}
                size={220}
              />
            </div>
            <p className="break-all text-center font-mono text-xs text-ink-muted max-w-sm">
              {qrProxy.link || `tg://proxy?server=${qrProxy.host}&port=${qrProxy.port}&secret=${qrProxy.secret}`}
            </p>
            <div className="flex gap-2">
              <Button
                variant="light"
                color="brand"
                onClick={() =>
                  copyText(
                    qrProxy.link ||
                      `tg://proxy?server=${qrProxy.host}&port=${qrProxy.port}&secret=${qrProxy.secret}`,
                    t("mtproto.copied")
                  )
                }
              >
                {t("mtproto.copyLink")}
              </Button>
              <Button variant="light" color="gray" onClick={() => setQrProxy(null)}>
                {t("common.close")}
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </Card>
  );
}

function AddMTProtoDialog({
  onClose,
  onAdded,
}: {
  onClose: () => void;
  onAdded: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [port, setPort] = useState("8443");
  const [domain, setDomain] = useState("cloudflare.com");
  const [secret, setSecret] = useState("");
  const [maxConns, setMaxConns] = useState("512");
  const [created, setCreated] = useState<{
    proxy: MTProtoProxy;
    install_command: string;
    cli_command: string;
  } | null>(null);

  const { busy, run } = useAction();

  const handleGenSecret = async () => {
    try {
      const res = await generateMTProtoSecret(domain);
      setSecret(res.secret);
    } catch (e) {
      notifyError(errMessage(e));
    }
  };

  const handleSubmit = () =>
    run(async () => {
      try {
        const res = await createMTProto({
          name: name.trim() || `MTProto :${port}`,
          host: host.trim(),
          port: Number(port) || 8443,
          domain: domain.trim() || "cloudflare.com",
          secret: secret.trim(),
          max_conns: Number(maxConns) || 512,
          enabled: true,
        });
        setCreated(res);
        notifySuccess(t("mtproto.saveSuccess"));
      } catch (e) {
        notifyError(errMessage(e));
      }
    });

  if (created) {
    return (
      <Modal title={t("mtproto.addTitle")} open={true} onClose={onAdded}>
        <div className="flex flex-col gap-4">
          <p className="text-sm text-ink-muted">{t("mtproto.addDesc")}</p>

          <div className="flex flex-col items-center gap-2 py-2">
            <div className="rounded-xl border border-gray-200 bg-white p-3 shadow-sm">
              <QRCodeSVG
                value={created.proxy.link || `tg://proxy?server=${created.proxy.host}&port=${created.proxy.port}&secret=${created.proxy.secret}`}
                size={180}
              />
            </div>
            <Button
              size="sm"
              variant="light"
              color="brand"
              onClick={() => {
                navigator.clipboard.writeText(created.proxy.link || "");
                notifySuccess(t("mtproto.copied"));
              }}
            >
              {t("mtproto.copyLink")}
            </Button>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-ink">{t("mtproto.installCmd")}</label>
            <div className="flex gap-2">
              <input
                readOnly
                className="w-full rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 font-mono text-xs text-ink select-all"
                value={created.install_command}
              />
              <Button
                size="sm"
                variant="light"
                color="gray"
                onClick={() => {
                  navigator.clipboard.writeText(created.install_command);
                  notifySuccess(t("common.copied"));
                }}
              >
                {t("common.copy")}
              </Button>
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <label className="text-xs font-medium text-ink">{t("mtproto.cliCommand")}</label>
            <div className="flex gap-2">
              <input
                readOnly
                className="w-full rounded-lg border border-gray-200 bg-gray-50 px-2.5 py-1.5 font-mono text-xs text-ink select-all"
                value={created.cli_command}
              />
              <Button
                size="sm"
                variant="light"
                color="gray"
                onClick={() => {
                  navigator.clipboard.writeText(created.cli_command);
                  notifySuccess(t("common.copied"));
                }}
              >
                {t("common.copy")}
              </Button>
            </div>
          </div>

          <div className="flex justify-end pt-2">
            <Button variant="filled" color="brand" onClick={onAdded}>
              {t("common.done")}
            </Button>
          </div>
        </div>
      </Modal>
    );
  }

  return (
    <Modal title={t("mtproto.addTitle")} open={true} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSubmit();
        }}
        className="flex flex-col gap-3"
      >
        <TextInput
          label={t("mtproto.name")}
          value={name}
          onChange={setName}
          placeholder="MTProto Germany"
        />
        <TextInput
          label={t("mtproto.host")}
          value={host}
          onChange={setHost}
          placeholder="203.0.113.10 или proxy.example.com"
        />
        <div className="grid grid-cols-2 gap-3">
          <TextInput
            label={t("mtproto.port")}
            type="number"
            value={port}
            onChange={setPort}
          />
          <TextInput
            label={t("mtproto.maxConns")}
            type="number"
            value={maxConns}
            onChange={setMaxConns}
          />
        </div>
        <TextInput
          label={t("mtproto.domain")}
          value={domain}
          onChange={setDomain}
          placeholder="cloudflare.com"
        />
        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-medium text-ink">{t("mtproto.secret")}</label>
          <div className="flex gap-2">
            <input
              type="text"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-xs text-ink focus:border-blue-500 focus:outline-none"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              placeholder="ee... (пусто для автогенерации)"
            />
            <Button type="button" size="sm" variant="light" color="gray" onClick={handleGenSecret}>
              {t("mtproto.genSecret")}
            </Button>
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-3">
          <Button variant="light" color="gray" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" variant="filled" color="brand" loading={busy} disabled={!host.trim()}>
            {t("common.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

function EditMTProtoDialog({
  proxy,
  onClose,
  onSaved,
}: {
  proxy: MTProtoProxy;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(proxy.name);
  const [host, setHost] = useState(proxy.host);
  const [port, setPort] = useState(String(proxy.port));
  const [domain, setDomain] = useState(proxy.domain);
  const [secret, setSecret] = useState(proxy.secret);
  const [maxConns, setMaxConns] = useState(String(proxy.max_conns));

  const { busy, run } = useAction();

  const handleGenSecret = async () => {
    try {
      const res = await generateMTProtoSecret(domain);
      setSecret(res.secret);
    } catch (e) {
      notifyError(errMessage(e));
    }
  };

  const handleSubmit = () =>
    run(async () => {
      try {
        await updateMTProto(proxy.id, {
          name: name.trim(),
          host: host.trim(),
          port: Number(port) || 8443,
          domain: domain.trim() || "cloudflare.com",
          secret: secret.trim(),
          max_conns: Number(maxConns) || 512,
          enabled: proxy.enabled,
        });
        notifySuccess(t("mtproto.saveSuccess"));
        onSaved();
      } catch (e) {
        notifyError(errMessage(e));
      }
    });

  return (
    <Modal title={t("mtproto.editTitle")} open={true} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSubmit();
        }}
        className="flex flex-col gap-3"
      >
        <TextInput
          label={t("mtproto.name")}
          value={name}
          onChange={setName}
        />
        <TextInput
          label={t("mtproto.host")}
          value={host}
          onChange={setHost}
        />
        <div className="grid grid-cols-2 gap-3">
          <TextInput
            label={t("mtproto.port")}
            type="number"
            value={port}
            onChange={setPort}
          />
          <TextInput
            label={t("mtproto.maxConns")}
            type="number"
            value={maxConns}
            onChange={setMaxConns}
          />
        </div>
        <TextInput
          label={t("mtproto.domain")}
          value={domain}
          onChange={setDomain}
        />
        <div className="flex flex-col gap-1.5">
          <label className="text-xs font-medium text-ink">{t("mtproto.secret")}</label>
          <div className="flex gap-2">
            <input
              type="text"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 font-mono text-xs text-ink focus:border-blue-500 focus:outline-none"
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              required
            />
            <Button type="button" size="sm" variant="light" color="gray" onClick={handleGenSecret}>
              {t("mtproto.genSecret")}
            </Button>
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-3">
          <Button variant="light" color="gray" onClick={onClose} disabled={busy}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" variant="filled" color="brand" loading={busy} disabled={!host.trim()}>
            {t("common.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
