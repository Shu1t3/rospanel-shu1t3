// Package awg runs an AmneziaWG server: WireGuard with the handshake hidden
// behind junk packets and random-looking headers, so a DPI box that recognises
// plain WireGuard sees nothing it knows. The protocol engine is amneziawg-go v3,
// embedded in the process (no daemon, no separate binary); this package owns the
// parameters, the keys, the peer list, the client configs and the counters.
//
// One tunnel per server: the master runs its own, every node runs its own with the
// keys and parameters the panel generated for it. A user is a peer on each tunnel
// they are allowed on, with the same address on all of them.
package awg

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"sort"
	"strconv"
	"strings"
)

// Iface is the tunnel interface name on every server.
const Iface = "awg0"

// DefaultMTU is what AmneziaWG clients default to; the junk headers ride inside
// the UDP payload, so the tunnel MTU is WireGuard's.
const DefaultMTU = 1420

// DefaultDNS is what clients resolve through inside the tunnel unless the
// operator says otherwise.
const DefaultDNS = "1.1.1.1, 8.8.8.8"

// Keepalive is the client-side persistent keepalive in seconds — the NATs
// mobile clients sit behind drop a silent UDP mapping in well under a minute.
const Keepalive = 25

// Params are the AmneziaWG obfuscation parameters (AWG 3.1): how
// many junk packets precede the handshake and how big they are (Jc, Jmin, Jmax),
// how much random padding the handshake and transport messages carry (S1–S4),
// the four message-type headers that replace WireGuard's 1/2/3/4 (H1–H4, as ranges
// or single integers), and AWG 3.1 advanced features: header protection, content padding,
// custom packet signatures (I1–I5) and timings.
type Params struct {
	Jc   int    `json:"jc"`
	Jmin int    `json:"jmin"`
	Jmax int    `json:"jmax"`
	S1   int    `json:"s1"`
	S2   int    `json:"s2"`
	S3   int    `json:"s3,omitempty"`
	S4   int    `json:"s4,omitempty"`
	H1   string `json:"h1"`
	H2   string `json:"h2"`
	H3   string `json:"h3"`
	H4   string `json:"h4"`

	I1 string `json:"i1,omitempty"`
	I2 string `json:"i2,omitempty"`
	I3 string `json:"i3,omitempty"`
	I4 string `json:"i4,omitempty"`
	I5 string `json:"i5,omitempty"`

	HeaderProtectionKey    string `json:"header_protection_key,omitempty"`
	ContentPaddingAddition string `json:"content_padding_addition,omitempty"`

	RandomTrailers bool `json:"random_trailers,omitempty"`
	DisableCookies bool `json:"disable_cookies,omitempty"`

	RekeyAfterTime       string `json:"rekey_after_time,omitempty"`
	RekeyTimeout         string `json:"rekey_timeout,omitempty"`
	RejectAfterTime      string `json:"reject_after_time,omitempty"`
	KeepaliveTimeout     string `json:"keepalive_timeout,omitempty"`
	MaxHandshakeAttempts string `json:"max_handshake_attempts,omitempty"`
}

// IsZero reports an unset parameter block (a server that has never had AWG on).
func (p Params) IsZero() bool {
	return p.Jc == 0 && p.Jmin == 0 && p.Jmax == 0 && p.S1 == 0 && p.S2 == 0 &&
		p.H1 == "" && p.H2 == "" && p.H3 == "" && p.H4 == ""
}

// Range is a parsed UintRange [Lo, Hi].
type Range struct {
	Lo uint32
	Hi uint32
}

// ParseRange parses a range string like "1234" or "1000-5000".
func ParseRange(s string) (Range, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Range{}, errors.New("awg: empty range")
	}
	parts := strings.Split(s, "-")
	if len(parts) > 2 {
		return Range{}, fmt.Errorf("awg: invalid range %q", s)
	}
	lo, err := strconv.ParseUint(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		return Range{}, fmt.Errorf("awg: invalid range start %q: %w", parts[0], err)
	}
	hi := lo
	if len(parts) == 2 {
		hi, err = strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
		if err != nil {
			return Range{}, fmt.Errorf("awg: invalid range end %q: %w", parts[1], err)
		}
	}
	if hi < lo {
		return Range{}, fmt.Errorf("awg: invalid range %d > %d", lo, hi)
	}
	return Range{Lo: uint32(lo), Hi: uint32(hi)}, nil
}

// Overlaps reports whether two ranges intersect.
func (r Range) Overlaps(other Range) bool {
	return r.Lo <= other.Hi && other.Lo <= r.Hi
}

// RandomParams picks a parameter set for AWG 3.1 with safe ranges:
// 3–8 junk packets of 50–1000 bytes, random padding sizes S1–S4 avoiding collisions,
// four distinct non-overlapping ranges for H1–H4, header protection key, and safe
// default timing intervals. I1–I5 remain empty by default.
func RandomParams() Params {
	p := Params{
		Jc:                     randInt(3, 8),
		Jmin:                   50,
		Jmax:                   1000,
		S1:                     randInt(30, 120),
		S2:                     randInt(30, 120),
		S3:                     randInt(30, 120),
		S4:                     randInt(12, 30),
		RandomTrailers:         true,
		DisableCookies:         true,
		ContentPaddingAddition: "0-32",
		RekeyAfterTime:         "110-130",
		RekeyTimeout:           "4-6",
		RejectAfterTime:        "175-195",
		KeepaliveTimeout:       "12-18",
		MaxHandshakeAttempts:   "10-15",
	}
	for p.S1+56 == p.S2 {
		p.S2 = randInt(30, 120)
	}
	for p.S1+84 == p.S3 || p.S2+28 == p.S3 {
		p.S3 = randInt(30, 120)
	}

	spread := uint32(randInt(50000, 200000))
	h1Start := uint32(randInt(100_000_000, 900_000_000))
	h2Start := uint32(randInt(1_000_000_000, 1_900_000_000))
	h3Start := uint32(randInt(2_000_000_000, 2_900_000_000))
	h4Start := uint32(randInt(3_000_000_000, 3_900_000_000))

	p.H1 = fmt.Sprintf("%d-%d", h1Start, h1Start+spread)
	p.H2 = fmt.Sprintf("%d-%d", h2Start, h2Start+spread)
	p.H3 = fmt.Sprintf("%d-%d", h3Start, h3Start+spread)
	p.H4 = fmt.Sprintf("%d-%d", h4Start, h4Start+spread)

	hpkBytes := make([]byte, 32)
	if _, err := rand.Read(hpkBytes); err == nil {
		p.HeaderProtectionKey = base64.StdEncoding.EncodeToString(hpkBytes)
	}

	return p
}

// Validate refuses parameter sets amneziawg-go would refuse or that break the obfuscation.
func (p Params) Validate() error {
	switch {
	case p.Jc < 0 || p.Jc > 128:
		return errors.New("awg: jc must be 0–128")
	case p.Jmin < 0 || p.Jmax < 0 || p.Jmin > p.Jmax || p.Jmax > 1280:
		return errors.New("awg: jmin ≤ jmax ≤ 1280")
	case p.S1 < 0 || p.S1 > 1132 || p.S2 < 0 || p.S2 > 1188:
		return errors.New("awg: s1 ≤ 1132, s2 ≤ 1188")
	case p.S3 < 0 || p.S3 > 1188 || p.S4 < 0 || p.S4 > 1188:
		return errors.New("awg: s3 ≤ 1188, s4 ≤ 1188")
	case p.S1 > 0 && p.S2 > 0 && p.S1+56 == p.S2:
		return errors.New("awg: s1 + 56 must not equal s2")
	case p.S1 > 0 && p.S3 > 0 && p.S1+84 == p.S3:
		return errors.New("awg: s1 + 84 must not equal s3")
	case p.S2 > 0 && p.S3 > 0 && p.S2+28 == p.S3:
		return errors.New("awg: s2 + 28 must not equal s3")
	}

	r1, err1 := ParseRange(p.H1)
	r2, err2 := ParseRange(p.H2)
	r3, err3 := ParseRange(p.H3)
	r4, err4 := ParseRange(p.H4)
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return errors.Join(err1, err2, err3, err4)
	}

	if r1.Lo < 5 || r2.Lo < 5 || r3.Lo < 5 || r4.Lo < 5 {
		return errors.New("awg: h1–h4 must be ≥ 5")
	}
	if r1.Overlaps(r2) || r1.Overlaps(r3) || r1.Overlaps(r4) ||
		r2.Overlaps(r3) || r2.Overlaps(r4) || r3.Overlaps(r4) {
		return errors.New("awg: h1–h4 ranges must not overlap")
	}

	if p.HeaderProtectionKey != "" {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(p.HeaderProtectionKey))
		if err != nil || len(raw) != 32 {
			return errors.New("awg: header protection key must be a valid 32-byte Base64 key")
		}
		if p.S1 < 12 || p.S2 < 12 || (p.S3 > 0 && p.S3 < 12) || (p.S4 > 0 && p.S4 < 12) {
			return errors.New("awg: when header protection is enabled, S1-S4 must be at least 12 bytes")
		}
	}

	for _, cp := range []string{p.I1, p.I2, p.I3, p.I4, p.I5} {
		if err := validateCPS(cp); err != nil {
			return err
		}
	}

	return nil
}

func validateCPS(spec string) error {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	remaining := spec
	for {
		start := strings.IndexByte(remaining, '<')
		if start == -1 {
			break
		}
		end := strings.IndexByte(remaining[start:], '>')
		if end == -1 {
			return errors.New("awg: unclosed tag in CPS signature")
		}
		end += start
		tag := remaining[start+1 : end]
		parts := strings.Fields(tag)
		if len(parts) == 0 {
			return errors.New("awg: empty tag in CPS signature")
		}
		key := parts[0]
		switch key {
		case "b", "t", "r", "rc", "rd", "d", "ds", "dz":
			// Known valid tags in amneziawg-go
		default:
			return fmt.Errorf("awg: unsupported CPS tag <%s>", key)
		}
		remaining = remaining[end+1:]
	}
	return nil
}

func randInt(lo, hi int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	if err != nil {
		return lo
	}
	return lo + int(n.Int64())
}

// GenerateKey mints a Curve25519 keypair, base64 as WireGuard writes keys.
func GenerateKey() (priv, pub string, err error) {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(k.Bytes()),
		base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

// PublicKey derives the public key of a base64 private key.
func PublicKey(privB64 string) (string, error) {
	raw, err := keyBytes(privB64)
	if err != nil {
		return "", err
	}
	k, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k.PublicKey().Bytes()), nil
}

func keyBytes(b64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("awg: key is not base64: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("awg: key is %d bytes, want 32", len(raw))
	}
	return raw, nil
}

// keyHex is the UAPI form of a key: the same 32 bytes, hex.
func keyHex(b64 string) (string, error) {
	raw, err := keyBytes(b64)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// Subnet is the tunnel network every server uses: /16 leaves room for 65,000
// users, each at the address ClientAddr derives from their id, the same on
// every server so a config for one server differs from the next only in the
// endpoint and keys.
var Subnet = netip.MustParsePrefix("10.66.0.0/16")

// ServerAddr is the server's own tunnel address, the first host of Subnet.
var ServerAddr = netip.MustParseAddr("10.66.0.1")

// ClientAddr is a user's tunnel address: host index id+1 inside Subnet, so user
// 1 is 10.66.0.2 and the two reserved hosts (.0.0, .0.1) are never handed out.
// false when the id is beyond what the subnet holds.
func ClientAddr(userID int64) (netip.Addr, bool) {
	idx := userID + 1
	if userID <= 0 || idx >= 65535 {
		return netip.Addr{}, false
	}
	base := Subnet.Addr().As4()
	return netip.AddrFrom4([4]byte{base[0], base[1], byte(idx >> 8), byte(idx)}), true
}

// Peer is one client on a server's tunnel.
type Peer struct {
	PublicKey string // base64
	Addr      netip.Addr
	Email     string // the user's Xray tag ("u12"), for counters and sightings
}

// Config is everything a server's tunnel is set up from.
type Config struct {
	PrivateKey string // base64
	ListenPort int
	Params     Params
	MTU        int
	Peers      []Peer
}

// UAPI renders the configuration in WireGuard's cross-platform IPC form
// with the AmneziaWG 3.1 extensions.
func (c Config) UAPI() (string, error) {
	priv, err := keyHex(c.PrivateKey)
	if err != nil {
		return "", err
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		return "", fmt.Errorf("awg: listen port %d out of range", c.ListenPort)
	}
	if err := c.Params.Validate(); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "private_key=%s\nlisten_port=%d\n", priv, c.ListenPort)
	p := c.Params
	fmt.Fprintf(&b, "jc=%d\njmin=%d\njmax=%d\ns1=%d\ns2=%d\n", p.Jc, p.Jmin, p.Jmax, p.S1, p.S2)
	if p.S3 > 0 {
		fmt.Fprintf(&b, "s3=%d\n", p.S3)
	}
	if p.S4 > 0 {
		fmt.Fprintf(&b, "s4=%d\n", p.S4)
	}
	fmt.Fprintf(&b, "h1=%s\nh2=%s\nh3=%s\nh4=%s\n", p.H1, p.H2, p.H3, p.H4)

	if p.HeaderProtectionKey != "" {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(p.HeaderProtectionKey))
		if err == nil && len(raw) == 32 {
			fmt.Fprintf(&b, "header_protection_key=%s\n", hex.EncodeToString(raw))
		}
	}
	if p.ContentPaddingAddition != "" {
		fmt.Fprintf(&b, "content_padding_addition=%s\n", p.ContentPaddingAddition)
	}
	if p.RandomTrailers {
		b.WriteString("random_trailers=true\n")
	}
	if p.DisableCookies {
		b.WriteString("disable_cookies=true\n")
	}
	if p.RekeyAfterTime != "" {
		fmt.Fprintf(&b, "rekey_after_time=%s\n", p.RekeyAfterTime)
	}
	if p.RekeyTimeout != "" {
		fmt.Fprintf(&b, "rekey_timeout=%s\n", p.RekeyTimeout)
	}
	if p.RejectAfterTime != "" {
		fmt.Fprintf(&b, "reject_after_time=%s\n", p.RejectAfterTime)
	}
	if p.KeepaliveTimeout != "" {
		fmt.Fprintf(&b, "keepalive_timeout=%s\n", p.KeepaliveTimeout)
	}
	if p.MaxHandshakeAttempts != "" {
		fmt.Fprintf(&b, "max_handshake_attempts=%s\n", p.MaxHandshakeAttempts)
	}
	if p.I1 != "" {
		fmt.Fprintf(&b, "i1=%s\n", p.I1)
	}
	if p.I2 != "" {
		fmt.Fprintf(&b, "i2=%s\n", p.I2)
	}
	if p.I3 != "" {
		fmt.Fprintf(&b, "i3=%s\n", p.I3)
	}
	if p.I4 != "" {
		fmt.Fprintf(&b, "i4=%s\n", p.I4)
	}
	if p.I5 != "" {
		fmt.Fprintf(&b, "i5=%s\n", p.I5)
	}

	b.WriteString("replace_peers=true\n")
	peers := append([]Peer(nil), c.Peers...)
	sort.Slice(peers, func(i, j int) bool { return peers[i].PublicKey < peers[j].PublicKey })
	for _, pe := range peers {
		pub, err := keyHex(pe.PublicKey)
		if err != nil {
			return "", fmt.Errorf("peer %s: %w", pe.Email, err)
		}
		if !pe.Addr.IsValid() {
			return "", fmt.Errorf("peer %s: no address", pe.Email)
		}
		fmt.Fprintf(&b, "public_key=%s\nreplace_allowed_ips=true\nallowed_ip=%s/32\n", pub, pe.Addr)
	}
	return b.String(), nil
}

// ClientConfig is one user's side of one server's tunnel.
type ClientConfig struct {
	PrivateKey      string // the user's, base64
	Address         netip.Addr
	DNS             string
	MTU             int
	Params          Params
	ServerPublicKey string
	Endpoint        string // host:port
}

// Render writes the AWG 3.1 config file every AmneziaWG client imports.
func (c ClientConfig) Render() string {
	mtu := c.MTU
	if mtu <= 0 {
		mtu = DefaultMTU
	}
	dns := strings.TrimSpace(c.DNS)
	if dns == "" {
		dns = DefaultDNS
	}
	p := c.Params
	var b strings.Builder
	fmt.Fprintf(&b, "[Interface]\nPrivateKey = %s\nAddress = %s/32\nDNS = %s\nMTU = %d\n",
		c.PrivateKey, c.Address, dns, mtu)
	fmt.Fprintf(&b, "Jc = %d\nJmin = %d\nJmax = %d\nS1 = %d\nS2 = %d\n",
		p.Jc, p.Jmin, p.Jmax, p.S1, p.S2)
	if p.S3 > 0 {
		fmt.Fprintf(&b, "S3 = %d\n", p.S3)
	}
	if p.S4 > 0 {
		fmt.Fprintf(&b, "S4 = %d\n", p.S4)
	}
	fmt.Fprintf(&b, "H1 = %s\nH2 = %s\nH3 = %s\nH4 = %s\n",
		p.H1, p.H2, p.H3, p.H4)

	if p.HeaderProtectionKey != "" {
		fmt.Fprintf(&b, "HeaderProtectionKey = %s\n", p.HeaderProtectionKey)
	}
	if p.ContentPaddingAddition != "" {
		fmt.Fprintf(&b, "ContentPaddingAddition = %s\n", p.ContentPaddingAddition)
	}
	if p.RandomTrailers {
		b.WriteString("RandomTrailers = on\n")
	}
	if p.DisableCookies {
		b.WriteString("DisableCookies = on\n")
	}
	if p.RekeyAfterTime != "" {
		fmt.Fprintf(&b, "RekeyAfterTime = %s\n", p.RekeyAfterTime)
	}
	if p.RekeyTimeout != "" {
		fmt.Fprintf(&b, "RekeyTimeout = %s\n", p.RekeyTimeout)
	}
	if p.RejectAfterTime != "" {
		fmt.Fprintf(&b, "RejectAfterTime = %s\n", p.RejectAfterTime)
	}
	if p.KeepaliveTimeout != "" {
		fmt.Fprintf(&b, "KeepaliveTimeout = %s\n", p.KeepaliveTimeout)
	}
	if p.MaxHandshakeAttempts != "" {
		fmt.Fprintf(&b, "MaxHandshakeAttempts = %s\n", p.MaxHandshakeAttempts)
	}
	if p.I1 != "" {
		fmt.Fprintf(&b, "I1 = %s\n", p.I1)
	}
	if p.I2 != "" {
		fmt.Fprintf(&b, "I2 = %s\n", p.I2)
	}
	if p.I3 != "" {
		fmt.Fprintf(&b, "I3 = %s\n", p.I3)
	}
	if p.I4 != "" {
		fmt.Fprintf(&b, "I4 = %s\n", p.I4)
	}
	if p.I5 != "" {
		fmt.Fprintf(&b, "I5 = %s\n", p.I5)
	}

	fmt.Fprintf(&b, "\n[Peer]\nPublicKey = %s\nAllowedIPs = 0.0.0.0/0, ::/0\nEndpoint = %s\nPersistentKeepalive = %d\n",
		c.ServerPublicKey, c.Endpoint, Keepalive)
	return b.String()
}

// PeerStat is what the device knows about one peer: counters since the device
// came up, the last handshake and the address the last packet came from.
type PeerStat struct {
	RxBytes       int64
	TxBytes       int64
	LastHandshake int64  // unix seconds, 0 = never
	Endpoint      string // ip:port, "" = never
}

// ParseStats reads an IpcGet dump into per-peer stats keyed by the peer's
// base64 public key.
func ParseStats(dump string) map[string]PeerStat {
	out := map[string]PeerStat{}
	var cur string
	var st PeerStat
	flush := func() {
		if cur != "" {
			out[cur] = st
		}
	}
	for _, line := range strings.Split(dump, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "public_key":
			flush()
			st = PeerStat{}
			cur = ""
			if raw, err := hex.DecodeString(v); err == nil && len(raw) == 32 {
				cur = base64.StdEncoding.EncodeToString(raw)
			}
		case "rx_bytes":
			st.RxBytes, _ = strconv.ParseInt(v, 10, 64)
		case "tx_bytes":
			st.TxBytes, _ = strconv.ParseInt(v, 10, 64)
		case "last_handshake_time_sec":
			st.LastHandshake, _ = strconv.ParseInt(v, 10, 64)
		case "endpoint":
			st.Endpoint = v
		}
	}
	flush()
	return out
}

// EndpointIP is the address half of a peer's endpoint, or "".
func EndpointIP(endpoint string) string {
	ap, err := netip.ParseAddrPort(endpoint)
	if err != nil {
		return ""
	}
	return ap.Addr().Unmap().String()
}

// Device is a running tunnel. The Linux implementation drives amneziawg-go over
// a TUN; elsewhere every method reports ErrUnsupported so the panel still builds
// and runs (and hands out configs) on a developer's machine.
type Device interface {
	// Apply brings the tunnel to cfg: starts it if needed, restarts it if the key
	// or port changed, otherwise replaces the peer list in place.
	Apply(cfg Config) error
	// Stats reads every peer's counters.
	Stats() (map[string]PeerStat, error)
	// Running reports whether the tunnel is up.
	Running() bool
	// LastError is what the last Apply failed with, "" when it succeeded.
	LastError() string
	// Close tears the tunnel down.
	Close()
}

// ErrUnsupported is what the stub device answers on a platform without TUN
// support in this build.
var ErrUnsupported = errors.New("awg: not supported on this platform")
