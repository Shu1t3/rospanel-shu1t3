/**
 * AmneziaWG 3.1 Packet Presets (I1–I5 CPS Signatures)
 * Adapted from Shu1t3/wg-easy-awg3 mimicry suite for amneziawg-go v3.
 *
 * Supported tags by amneziawg-go:
 * <b 0xHEX>  - raw hex bytes
 * <r SIZE>   - random bytes
 * <rc SIZE>  - random characters [a-zA-Z]
 * <rd SIZE>  - random digits [0-9]
 * <t>        - 4-byte unix timestamp
 */

function randInt(min: number, max: number): number {
  const lo = Math.ceil(min);
  const hi = Math.floor(max);
  if (lo >= hi) return lo;
  return lo + Math.floor(Math.random() * (hi - lo + 1));
}

function randHex(byteLen: number): string {
  const bytes = new Uint8Array(byteLen);
  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    crypto.getRandomValues(bytes);
  } else {
    for (let i = 0; i < byteLen; i++) {
      bytes[i] = Math.floor(Math.random() * 256);
    }
  }
  return Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}

function hexPad(val: number, bytes: number): string {
  let hex = Math.floor(val).toString(16);
  while (hex.length < bytes * 2) hex = "0" + hex;
  return hex.slice(-(bytes * 2));
}

function quicVarint(value: number): string {
  const n = Math.max(0, Math.floor(value));
  if (n < 0x40) return hexPad(n, 1);
  if (n < 0x4000) return hexPad(0x4000 + n, 2);
  if (n < 0x40000000) return hexPad(0x80000000 + n, 4);
  const high = Math.floor(n / 0x100000000);
  const low = n % 0x100000000;
  return hexPad(0xc0000000 + high, 4) + hexPad(low, 4);
}

function splitPad(n: number): string {
  n = Math.max(0, Math.floor(n));
  if (n === 0) return "";
  let out = "";
  while (n > 1000) {
    out += "<r 1000>";
    n -= 1000;
  }
  out += `<r ${n}>`;
  return out;
}

function encodeDnsName(host: string): string {
  let hex = "";
  for (const label of host.split(".")) {
    if (!label) continue;
    const clipped = label.slice(0, 63);
    hex += clipped.length.toString(16).padStart(2, "0");
    for (const ch of clipped) {
      hex += ch.charCodeAt(0).toString(16).padStart(2, "0");
    }
  }
  return hex + "00";
}

// 1. QUIC Initial Handshake Mimicry
export function buildQuicInitial(host = "cloudflare.com"): string {
  const dcid = randInt(8, 20);
  const scid = randInt(0, 20);
  const tokenLen = randInt(0, 1) === 0 ? 0 : randInt(8, 32);
  const sniRc = Math.min(host.length + randInt(0, 6), 64);
  const pnLen = randInt(1, 4);

  const prefix =
    hexPad(0xc0 | (pnLen - 1), 1) +
    "00000001" +
    hexPad(dcid, 1) +
    randHex(dcid) +
    hexPad(scid, 1) +
    randHex(scid) +
    quicVarint(tokenLen) +
    randHex(tokenLen);

  const pad = randInt(80, 240);
  const hex =
    prefix + quicVarint(pnLen + pad + sniRc + 4) + randHex(pnLen);

  return `<b 0x${hex}><rc ${sniRc}><t>${splitPad(pad)}`;
}

// 2. TLS 1.3 ClientHello Mimicry
export function buildTlsClientHello(host = "cloudflare.com"): string {
  const sniRc = Math.min(host.length + 9, 64);
  const padding = randInt(60, 200);
  const recordLen = 38 + sniRc + padding + 4;
  const handshakeLen = recordLen - 4;

  const hex =
    "160301" +
    hexPad(recordLen, 2) +
    "01" +
    hexPad(handshakeLen, 3) +
    "0303" +
    randHex(32);

  return `<b 0x${hex}><rc ${sniRc}><t>${splitPad(padding)}`;
}

// 3. DNS Query with EDNS0 Mimicry
export function buildDnsQuery(host = "google.com"): string {
  const txid = randHex(2);
  const flags = "0100";
  const qdcount = "0001";
  const ancount = "0000";
  const nscount = "0000";
  const arcount = "0001"; // with EDNS(0) opt

  const question = encodeDnsName(host) + "00010001"; // A IN
  const padding = randInt(40, 120);

  // OPT RR header (41) + UDP payload 1232 + Pad option
  const opt =
    "000029" +
    "04d0" + // 1232 MTU
    "00000000" +
    hexPad(4 + padding, 2) +
    "000c" + // PAD option
    hexPad(padding, 2);

  const hex = txid + flags + qdcount + ancount + nscount + arcount + question + opt;
  return `<b 0x${hex}>${splitPad(padding)}<t>`;
}

// 4. HTTP/3 Initial / 0-RTT Mimicry
export function buildHttp3(host = "cloudflare.com"): string {
  const dcid = randInt(8, 20);
  const scid = randInt(0, 20);
  const sniRc = Math.min(host.length + randInt(2, 8), 64);
  const pnLen = randInt(1, 4);

  const prefix =
    hexPad(0xe0 | (pnLen - 1), 1) +
    "00000001" +
    hexPad(dcid, 1) +
    randHex(dcid) +
    hexPad(scid, 1) +
    randHex(scid);

  const pad = randInt(70, 180);
  const hex = prefix + quicVarint(pnLen + pad + sniRc + 4) + randHex(pnLen);

  return `<b 0x${hex}><rc ${sniRc}><t>${splitPad(pad)}`;
}

// 5. DTLS 1.2 Handshake Mimicry
export function buildDtlsHandshake(host = "webrtc.example.org"): string {
  const sniRc = Math.min(host.length + randInt(2, 6), 60);
  const pad = randInt(50, 160);
  const bodyLen = 34 + sniRc + pad + 4;
  const recordLen = 12 + bodyLen;

  const hex =
    "16fefd0000000000000001" +
    hexPad(recordLen, 2) +
    "01" +
    hexPad(bodyLen, 3) +
    "0000000000" +
    hexPad(bodyLen, 3) +
    "fefd" +
    randHex(32);

  return `<b 0x${hex}><rc ${sniRc}><t>${splitPad(pad)}`;
}

// 6. WireGuard Noise Handshake Mimicry
export function buildWireguardNoise(): string {
  const rcLen = randInt(4, 12);
  const pad = randInt(20, 80);
  const hex = "01000000" + randHex(4) + randHex(32) + randHex(48) + randHex(28);
  return `<b 0x${hex}>${splitPad(pad)}<t><rc ${rcLen}>`;
}

// Helper for supplementary entropy packets (I2–I5)
function buildEntropyPacket(idx: number): string {
  const rLen = randInt(16, 64);
  const rcLen = randInt(4, 12);
  const rdLen = randInt(4, 8);
  const b = `<b 0x${randHex(randInt(4, 10))}>`;

  const patterns = [
    `${b}<r ${rLen}><t><rc ${rcLen}><rd ${rdLen}>`,
    `<rc ${rcLen}>${b}<r ${rLen}><t><rd ${rdLen}>`,
    `<t><r ${rLen}>${b}<rc ${rcLen}><rd ${rdLen}>`,
    `${b}<rc ${rcLen}><rd ${rdLen}><t><r ${rLen}>`,
  ];
  return patterns[idx % patterns.length] || `<r 20><t>`;
}

export type AwgPresetId =
  | "none"
  | "quic_initial"
  | "tls_client_hello"
  | "dns_query"
  | "http3"
  | "dtls"
  | "wireguard_noise";

export interface AwgPresetOption {
  id: AwgPresetId;
  labelKey: string;
}

export const AWG_PRESET_OPTIONS: AwgPresetOption[] = [
  {
    id: "none",
    labelKey: "awg.presets.custom",
  },
  {
    id: "quic_initial",
    labelKey: "awg.presets.quic",
  },
  {
    id: "tls_client_hello",
    labelKey: "awg.presets.tls",
  },
  {
    id: "dns_query",
    labelKey: "awg.presets.dns",
  },
  {
    id: "http3",
    labelKey: "awg.presets.h3",
  },
  {
    id: "dtls",
    labelKey: "awg.presets.dtls",
  },
  {
    id: "wireguard_noise",
    labelKey: "awg.presets.wgNoise",
  },
];

export function generateAwgPresetSignatures(
  presetId: AwgPresetId,
  host = "cloudflare.com"
): { i1: string; i2: string; i3: string; i4: string; i5: string } {
  if (presetId === "none") {
    return { i1: "", i2: "", i3: "", i4: "", i5: "" };
  }

  let i1 = "";
  switch (presetId) {
    case "quic_initial":
      i1 = buildQuicInitial(host);
      break;
    case "tls_client_hello":
      i1 = buildTlsClientHello(host);
      break;
    case "dns_query":
      i1 = buildDnsQuery("google.com");
      break;
    case "http3":
      i1 = buildHttp3(host);
      break;
    case "dtls":
      i1 = buildDtlsHandshake(host);
      break;
    case "wireguard_noise":
      i1 = buildWireguardNoise();
      break;
  }

  return {
    i1,
    i2: buildEntropyPacket(0),
    i3: buildEntropyPacket(1),
    i4: buildEntropyPacket(2),
    i5: buildEntropyPacket(3),
  };
}
