import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  createMultiHopProfile,
  deleteMultiHopProfile,
  listMultiHopProfiles,
  type MultiHopProfile,
  type NodeView,
  updateMultiHopProfile,
} from "./api";
import { useAction } from "./hooks";
import { errMessage, notifySuccess } from "./notify";
import { nodeState, serverName } from "./NodeStatus";
import { Badge, Button, CenterLoader, Modal, Select, Switch, TextInput, useConfirm } from "./ui";

type Role = "entry" | "transit" | "exit";

const roles = [
  { role: "entry", label: "inb.multiHopEntry" },
  { role: "transit", label: "inb.multiHopTransit" },
  { role: "exit", label: "inb.multiHopExit" },
] as const;

const newProfile = (): MultiHopProfile => ({
  id: 0, name: "", entry: -1, transit: -1, exit: -1, port: 0, enabled: true,
});

export function MultiHopDialog({ nodes, canManage, onClose }: {
  nodes: NodeView[];
  canManage: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [profiles, setProfiles] = useState<MultiHopProfile[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const [draft, setDraft] = useState<MultiHopProfile | null>(null);
  const { busy, run } = useAction();
  const { confirm, confirmNode } = useConfirm();

  useEffect(() => {
    listMultiHopProfiles().then(setProfiles).catch((e) => setLoadError(errMessage(e)));
  }, []);

  const joined = (n: NodeView) => n.enabled && (n.is_local || n.joined);
  const available = (n: NodeView, role: Role) => joined(n) && (role === "entry" || n.vless_enabled);
  const entries = nodes.filter(joined);
  const peers = nodes.filter((n) => available(n, "transit"));
  const canCreate = peers.length >= 2 && entries.length >= 3;
  const nameOf = (id: number) => {
    const node = nodes.find((n) => n.id === id);
    return node ? serverName(node) : `#${id}`;
  };

  const save = () => {
    if (!draft) return;
    run(async () => {
      const profile = draft.id ? await updateMultiHopProfile(draft) : await createMultiHopProfile({
        name: draft.name, entry: draft.entry, transit: draft.transit, exit: draft.exit,
      });
      setProfiles((current) => draft.id
        ? current?.map((p) => p.id === profile.id ? profile : p) ?? [profile]
        : [...(current ?? []), profile]);
      setDraft(null);
      notifySuccess(t("common.saved"));
    });
  };

  const remove = async (profile: MultiHopProfile) => {
    const ok = await confirm({
      title: t("inb.deleteTitle"),
      body: t("inb.deleteBody", { name: profile.name }),
      confirmLabel: t("common.delete"),
    });
    if (!ok) return;
    run(async () => {
      await deleteMultiHopProfile(profile.id);
      setProfiles((current) => current?.filter((p) => p.id !== profile.id) ?? []);
      setDraft(null);
    });
  };

  const options = (role: Role, profile: MultiHopProfile) => {
    const value = profile[role];
    const used = roles.filter((r) => r.role !== role).map((r) => profile[r.role]);
    const list = nodes
      .filter((n) => n.id === value || (available(n, role) && !used.includes(n.id)))
      .map((n) => {
        const state = nodeState(n);
        return {
          value: String(n.id),
          label: state.tone === "success" ? serverName(n) : `${serverName(n)} · ${state.label}`,
        };
      });
    if (value >= 0 && !list.some((option) => option.value === String(value))) {
      list.push({ value: String(value), label: nameOf(value) });
    }
    return list;
  };

  const ready = draft && draft.name.trim() && roles.every(({ role }) => draft[role] >= 0);

  return (
    <>
      {confirmNode}
      <Modal open onClose={onClose} size="lg" title={t("inb.multiHop")}>
        <div className="flex flex-col gap-4">
          {draft ? (
            <>
              <TextInput
                label={t("groups.name")}
                value={draft.name}
                onChange={(name) => setDraft({ ...draft, name })}
              />
              <ol className="grid gap-2 sm:grid-cols-3">
                {roles.map(({ role, label }, index) => (
                  <li key={role} className="min-w-0 rounded-lg border border-gray-200 p-2">
                    <span aria-hidden="true" className="mb-1 block font-mono text-xs text-ink-muted">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    <Select
                      label={t(label)}
                      value={draft[role] < 0 ? "" : String(draft[role])}
                      onChange={(v) => setDraft({ ...draft, [role]: Number(v) })}
                      data={options(role, draft)}
                      disabled={!!draft.id && role === "entry"}
                      searchable
                    />
                  </li>
                ))}
              </ol>
              <p className="text-xs text-ink-muted">{t("inb.multiHopHint")}</p>
              {draft.id !== 0 && (
                <div className="flex items-center justify-between text-sm">
                  <span>{t("conn.port")} <span className="font-mono">{draft.port}/TCP</span></span>
                  <Switch label={t("groups.on")} checked={draft.enabled} onChange={(enabled) => setDraft({ ...draft, enabled })} />
                </div>
              )}
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  {draft.id !== 0 && (
                    <Button variant="light" color="red" onClick={() => remove(draft)} disabled={busy}>
                      {t("common.delete")}
                    </Button>
                  )}
                </div>
                <div className="flex gap-2">
                  <Button variant="light" color="gray" onClick={() => setDraft(null)} disabled={busy}>
                    {t("common.cancel")}
                  </Button>
                  <Button onClick={save} loading={busy} disabled={!ready || busy}>
                    {t("common.save")}
                  </Button>
                </div>
              </div>
            </>
          ) : (
            <>
              <p className="text-sm text-ink-muted">{t("inb.multiHopGroups")}</p>
              {loadError && <p role="alert" className="text-sm text-danger">{loadError}</p>}
              {profiles === null && !loadError && <CenterLoader />}
              {profiles?.map((profile) => (
                <div key={profile.id} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-gray-200 p-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-semibold">{profile.name}</span>
                      <Badge color={profile.enabled ? "green" : "gray"}>{t(profile.enabled ? "groups.on" : "groups.off")}</Badge>
                      <span className="font-mono text-xs text-ink-muted">{profile.port}/TCP</span>
                    </div>
                    <p className="mt-1 break-words text-xs text-ink-muted">
                      {roles.map(({ role }) => nameOf(profile[role])).join(" → ")}
                    </p>
                  </div>
                  {canManage && (
                    <Button size="sm" variant="outline" color="gray" onClick={() => setDraft(profile)}>
                      {t("common.edit")}
                    </Button>
                  )}
                </div>
              ))}
              {profiles?.length === 0 && <p className="text-sm">{t("inb.multiHopEmpty")}</p>}
              {canManage && profiles !== null && (
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-xs text-ink-muted">{t(canCreate ? "inb.multiHopPortHint" : "inb.multiHopNeedServers")}</p>
                  <Button onClick={() => setDraft(newProfile())} disabled={!canCreate}>
                    {t("inb.multiHopCreate")}
                  </Button>
                </div>
              )}
            </>
          )}
        </div>
      </Modal>
    </>
  );
}
