import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  createMultiHopProfile,
  deleteMultiHopProfile,
  listMultiHopProfiles,
  type MultiHopProfile,
  type MultiHopReverse,
  type NodeView,
  updateMultiHopProfile,
} from "./api";
import { countryFlag, countryName } from "./format";
import { useAction } from "./hooks";
import { errMessage, notifySuccess } from "./notify";
import { nodeState, serverName } from "./NodeStatus";
import { Badge, Button, CenterLoader, Modal, Select, Switch, TextInput, useConfirm } from "./ui";

type Draft = MultiHopProfile & { slotKeys: number[] };

const newProfile = (hasPeer: boolean): MultiHopProfile => ({
  id: 0, name: "", nodes: hasPeer ? [-1, -1] : [-1], port: 0, enabled: true,
});

const reverseRoles = ["inb.multiHopReverseA", "inb.multiHopReverseR", "inb.multiHopReverseB", "inb.multiHopReverseC"] as const;
const ipList = (items: string[]) => items.map((ip) => ip.trim()).filter(Boolean);

export function MultiHopDialog({ nodes, canManage, onClose }: {
  nodes: NodeView[];
  canManage: boolean;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [profiles, setProfiles] = useState<MultiHopProfile[] | null>(null);
  const [loadError, setLoadError] = useState("");
  const [draft, setDraft] = useState<Draft | null>(null);
  const nextSlot = useRef(0);
  const edit = (profile: MultiHopProfile) => {
    setDraft({ ...profile, slotKeys: profile.nodes.map(() => nextSlot.current++) });
  };
  const { busy, run } = useAction();
  const { confirm, confirmNode } = useConfirm();

  useEffect(() => {
    const load = () => listMultiHopProfiles().then((items) => {
      setProfiles(items);
      setLoadError("");
    }).catch((e) => setLoadError(errMessage(e)));
    void load();
    const timer = window.setInterval(() => { void load(); }, 5000);
    return () => window.clearInterval(timer);
  }, []);

  const joined = (n: NodeView) => n.enabled && (n.is_local || n.joined);
  const entries = nodes.filter(joined);
  const peers = entries.filter((n) => n.vless_enabled);
  const canCreate = entries.length > 0;
  const nameOf = (id: number) => {
    const node = nodes.find((n) => n.id === id);
    return node ? serverName(node) : `#${id}`;
  };

  const save = async () => {
    if (!draft) return;
    const old = profiles?.find((p) => p.id === draft.id);
    if (old && old.nodes[0] !== draft.nodes[0] && !await confirm({
      title: t("inb.multiHopEntryChange"),
      body: t("inb.multiHopEntryChangeBody"),
      confirmLabel: t("common.save"),
    })) return;
    run(async () => {
      const reverse = draft.reverse && {
        ...draft.reverse,
        transit_ip: draft.reverse.transit_ip.trim(),
        transit_ips: ipList(draft.reverse.transit_ips ?? []),
        relay_sources: ipList(draft.reverse.relay_sources),
      };
      const profile = draft.id ? await updateMultiHopProfile({ ...draft, reverse }) : await createMultiHopProfile({
        name: draft.name, nodes: draft.nodes, reverse,
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

  const options = (index: number, profile: MultiHopProfile) => {
    const value = profile.nodes[index];
    const last = profile.nodes.length - 1;
    const canShare = (other: number) => last >= 2 && ((index === 0 && other === last) || (index === last && other === 0));
    const list = nodes
      .filter((n) => n.id === value || (joined(n) && (profile.reverse || index === 0 || n.vless_enabled) &&
        !(profile.reverse && index === 2 && n.is_local) &&
        !profile.nodes.some((id, i) => id === n.id && !canShare(i))))
      .map((n) => {
        const state = nodeState(n);
        const parts = [serverName(n)];
        if (n.country) parts.push(`${countryFlag(n.country)} ${countryName(n.country, i18n.language, n.country)}`);
        if (state.tone !== "success") parts.push(state.label);
        return { value: String(n.id), label: parts.join(" · ") };
      });
    if (value >= 0 && !list.some((option) => option.value === String(value))) {
      list.push({ value: String(value), label: nameOf(value) });
    }
    return list;
  };

  const changeOrder = (from: number, to: number) => {
    if (!draft) return;
    const order = [...draft.nodes];
    const [node] = order.splice(from, 1);
    order.splice(to, 0, node);
    const keys = [...draft.slotKeys];
    const [key] = keys.splice(from, 1);
    keys.splice(to, 0, key);
    setDraft({ ...draft, nodes: order, slotKeys: keys });
  };

  const setReverse = (enabled: boolean) => {
    if (!draft) return;
    const entry = draft.nodes[0];
    if (enabled) {
      const order = draft.nodes.length === 4 ? draft.nodes : [entry, -1, -1, draft.nodes[draft.nodes.length - 1] ?? -1];
      edit({ ...draft, nodes: order, reverse: { relay_port: 29443, transit_port: 29444, transit_ip: "", transit_ips: [], relay_sources: [] } });
    } else {
      edit({ ...draft, nodes: [entry, -1], reverse: undefined });
    }
  };

  const updateReverse = (changes: Partial<MultiHopReverse>) => {
    if (draft?.reverse) setDraft({ ...draft, reverse: { ...draft.reverse, ...changes } });
  };

  const order = draft?.nodes ?? [];
  const distinct = order.length >= 3 && order[0] === order[order.length - 1] ? order.slice(0, -1) : order;
  const ready = draft && draft.name.trim() && order.every((id) => id >= 0) && new Set(distinct).size === distinct.length &&
    (!draft.reverse || (draft.reverse.relay_port > 0 && draft.reverse.relay_port <= 65535 &&
      draft.reverse.transit_port > 0 && draft.reverse.transit_port <= 65535 &&
      draft.reverse.transit_ip.trim() && ipList(draft.reverse.relay_sources).length > 0));
  const canAdd = draft && entries.length > 1 && draft.nodes.length < peers.length + 1;
  const add = () => {
    if (!draft) return;
    const order = [...draft.nodes];
    const index = Math.max(1, order.length - 1);
    order.splice(index, 0, -1);
    const keys = [...draft.slotKeys];
    keys.splice(index, 0, nextSlot.current++);
    setDraft({ ...draft, nodes: order, slotKeys: keys });
  };

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
              <div className="flex items-center justify-between gap-3 rounded-lg border border-gray-200 p-3">
                <span className="text-sm font-medium">{t("inb.multiHopReverseMode")}</span>
                <Switch label={t("inb.multiHopReverseMode")} checked={!!draft.reverse} onChange={setReverse} />
              </div>
              <ol className="space-y-2 border-s border-gray-200 ps-3">
                {draft.nodes.map((node, index) => (
                  <li key={draft.slotKeys[index]} className="flex flex-wrap items-end gap-2 rounded-lg border border-gray-200 p-2">
                    <span aria-hidden="true" className="self-center font-mono text-xs text-ink-muted">
                      {String(index + 1).padStart(2, "0")}
                    </span>
                    <div className="min-w-40 flex-1">
                      <Select
                        label={t(draft.reverse ? reverseRoles[index] : draft.nodes.length === 1 ? "inb.multiHopSingle" : index === 0
                          ? "inb.multiHopEntry" : index === draft.nodes.length - 1 ? "inb.multiHopExit" : "inb.multiHopTransit")}
                        value={node < 0 ? "" : String(node)}
                        onChange={(v) => setDraft({ ...draft, nodes: draft.nodes.map((id, i) => i === index ? (v === "" ? -1 : Number(v)) : id) })}
                        data={options(index, draft)}
                        searchable
                      />
                    </div>
                    {!draft.reverse && <div className="flex gap-1">
                      <Button size="sm" variant="light" color="gray" disabled={index === 0}
                        onClick={() => changeOrder(index, index - 1)}>
                        <span aria-hidden="true">↑</span>
                        <span className="sr-only">{t("inb.multiHopUp", { position: index + 1 })}</span>
                      </Button>
                      <Button size="sm" variant="light" color="gray" disabled={index === draft.nodes.length - 1}
                        onClick={() => changeOrder(index, index + 1)}>
                        <span aria-hidden="true">↓</span>
                        <span className="sr-only">{t("inb.multiHopDown", { position: index + 1 })}</span>
                      </Button>
                      <Button size="sm" variant="light" color="red" disabled={draft.nodes.length === 1}
                        onClick={() => setDraft({
                          ...draft,
                          nodes: draft.nodes.filter((_, i) => i !== index),
                          slotKeys: draft.slotKeys.filter((_, i) => i !== index),
                        })}>
                        <span aria-hidden="true">×</span>
                        <span className="sr-only">{t("inb.multiHopRemove", { position: index + 1 })}</span>
                      </Button>
                    </div>}
                  </li>
                ))}
              </ol>
              {draft.reverse ? (
                <>
                  <p className="text-xs text-ink-muted">{t("inb.multiHopReverseHint")}</p>
                  <div className="grid gap-2 sm:grid-cols-2">
                    <TextInput label={t("inb.multiHopReverseRPort")} mono inputMode="numeric"
                      value={String(draft.reverse.relay_port || "")} onChange={(v) => updateReverse({ relay_port: Number(v) })} />
                    <TextInput label={t("inb.multiHopReverseBPort")} mono inputMode="numeric"
                      value={String(draft.reverse.transit_port || "")} onChange={(v) => updateReverse({ transit_port: Number(v) })} />
                    <TextInput label={t("inb.multiHopReverseBIP")} mono value={draft.reverse.transit_ip}
                      onChange={(transit_ip) => updateReverse({ transit_ip })} />
                    <TextInput label={t("inb.multiHopReverseBIPs")} mono value={(draft.reverse.transit_ips ?? []).join(",")}
                      onChange={(v) => updateReverse({ transit_ips: v.split(",") })} />
                    <TextInput label={t("inb.multiHopReverseRSources")} mono className="sm:col-span-2"
                      value={draft.reverse.relay_sources.join(",")} onChange={(v) => updateReverse({ relay_sources: v.split(",") })} />
                  </div>
                </>
              ) : (
                <>
                  <Button className="self-start" size="sm" variant="outline" color="gray" onClick={add} disabled={!canAdd}>
                    + {t("inb.multiHopAdd")}
                  </Button>
                  <p className="text-xs text-ink-muted">{t(draft.nodes.length === 1 ? "inb.multiHopSingleHint" : "inb.multiHopHint")}</p>
                </>
              )}
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
                      <Badge color={profile.applying ? "yellow" : profile.enabled ? "green" : "gray"}>
                        {t(profile.applying ? "inb.multiHopApplying" : profile.enabled ? "groups.on" : "groups.off")}
                      </Badge>
                      {profile.reverse && <Badge>{t("inb.multiHopReverseMode")}</Badge>}
                      <span className="font-mono text-xs text-ink-muted">{profile.port}/TCP</span>
                    </div>
                    <p className="mt-1 break-words text-xs text-ink-muted">
                      {(profile.reverse ? [...profile.nodes.slice(0, 3), profile.nodes[1], profile.nodes[3]] : profile.nodes).map(nameOf).join(" → ")}
                    </p>
                  </div>
                  {canManage && (
                    <Button size="sm" variant="outline" color="gray" onClick={() => edit(profile)}>
                      {t("common.edit")}
                    </Button>
                  )}
                </div>
              ))}
              {profiles?.length === 0 && <p className="text-sm">{t("inb.multiHopEmpty")}</p>}
              {canManage && profiles !== null && (
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-xs text-ink-muted">{t(canCreate ? "inb.multiHopPortHint" : "inb.multiHopNeedServers")}</p>
                  <Button onClick={() => edit(newProfile(entries.length > 1 && peers.length > 0))} disabled={!canCreate}>
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
