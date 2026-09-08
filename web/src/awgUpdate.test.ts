import assert from "node:assert/strict";
import test from "node:test";
import type { AWGParams, ConnectionsStatus } from "./api";
import { buildAwgConnectionsUpdate } from "./awgUpdate.ts";

const awgParams: AWGParams = {
  jc: 4,
  jmin: 50,
  jmax: 1000,
  s1: 64,
  s2: 70,
  h1: "1",
  h2: "2",
  h3: "3",
  h4: "4",
};

const status: ConnectionsStatus = {
  host: "vpn.example.com",
  sni: "cdn.example.com",
  protocols: [
    {
      key: "vless",
      name: "VLESS",
      display_name: "Primary VLESS",
      transport: "TCP",
      security: "TLS",
      port: "443",
      note: "",
      enabled: true,
      fingerprint: "chrome",
    },
    {
      key: "reality",
      name: "REALITY",
      display_name: "Primary REALITY",
      transport: "XHTTP",
      security: "REALITY",
      port: "8443",
      note: "cdn.example.com",
      enabled: true,
      fingerprint: "safari",
    },
    {
      key: "hysteria2",
      name: "Hysteria2",
      display_name: "Primary Hysteria",
      transport: "QUIC",
      security: "TLS",
      port: "4433",
      note: "4433–4443",
      enabled: true,
      fingerprint: "",
    },
    {
      key: "awg",
      name: "AmneziaWG",
      display_name: "AWG Moscow",
      transport: "UDP",
      security: "WireGuard",
      port: "—",
      note: "",
      enabled: false,
      fingerprint: "",
    },
  ],
  hysteria_port: 4433,
  hop_start: 4433,
  hop_end: 4443,
  hop_interval: "7-11",
  hysteria_obfs: "existing-salamander-key",
  reality_port: 8443,
  reality_dest: "cdn.example.com",
  reality_public_key: "public-key",
  reality_short_id: "short-id",
  reality_path: "/secret",
  reality_anti_replay: true,
  tls_fragment: true,
  tls_min13: true,
  block_quic: true,
  awg_port: 0,
  awg_public_key: "",
  awg_params: awgParams,
  awg_dns: "",
  awg_running: false,
};

test("enabling AWG preserves the main connection settings", () => {
  const update = buildAwgConnectionsUpdate(status, {
    enabled: true,
    port: 51820,
    dns: "1.1.1.1",
    params: awgParams,
    regenKeys: false,
  });

  assert.deepEqual(update.protocols, {
    vless: true,
    reality: true,
    hysteria2: true,
    awg: true,
  });
  assert.deepEqual(update.fingerprints, {
    vless: "chrome",
    reality: "safari",
  });
  assert.deepEqual(update.names, {
    vless: "Primary VLESS",
    reality: "Primary REALITY",
    hysteria2: "Primary Hysteria",
    awg: "AWG Moscow",
  });
  assert.equal(update.hysteria_obfs, "existing-salamander-key");
  assert.equal(update.hysteria_port, 4433);
  assert.equal(update.hop_start, 4433);
  assert.equal(update.hop_end, 4443);
  assert.equal(update.hop_interval, "7-11");
  assert.equal(update.reality_port, 8443);
  assert.equal(update.reality_dest, "cdn.example.com");
  assert.equal(update.reality_anti_replay, true);
  assert.equal(update.tls_fragment, true);
  assert.equal(update.tls_min13, true);
  assert.equal(update.block_quic, true);
});
