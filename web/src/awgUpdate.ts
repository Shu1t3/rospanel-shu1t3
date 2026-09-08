import type { AWGParams, ConnectionsStatus, ConnectionsUpdate } from "./api";

export interface AwgUpdateValues {
  enabled: boolean;
  port: number;
  dns: string;
  params: AWGParams;
  regenKeys: boolean;
}

// AWG shares the full-replacement connections endpoint with the main editor.
// Round-trip every field owned by that editor so saving this card cannot reset it.
export function buildAwgConnectionsUpdate(
  status: ConnectionsStatus,
  values: AwgUpdateValues,
): ConnectionsUpdate {
  const protocols: Record<string, boolean> = {};
  const fingerprints: Record<string, string> = {};
  const names: Record<string, string> = {};

  status.protocols.forEach((protocol) => {
    protocols[protocol.key] = protocol.key === "awg" ? values.enabled : protocol.enabled;
    if (protocol.fingerprint) fingerprints[protocol.key] = protocol.fingerprint;
    names[protocol.key] = protocol.display_name || "";
  });

  return {
    protocols,
    fingerprints,
    names,
    hysteria_port: status.hysteria_port,
    hop_start: status.hop_start,
    hop_end: status.hop_end,
    hop_interval: status.hop_interval || "5-10",
    hysteria_obfs: status.hysteria_obfs || "",
    reality_port: status.reality_port,
    reality_dest: status.reality_dest,
    reality_anti_replay: status.reality_anti_replay,
    regen_reality_keys: false,
    tls_fragment: status.tls_fragment,
    tls_min13: status.tls_min13,
    block_quic: status.block_quic,
    awg_port: values.port,
    awg_dns: values.dns,
    awg_params: values.params,
    regen_awg_keys: values.regenKeys,
  };
}
