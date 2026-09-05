package sub

import (
	"fmt"
	"github.com/Shu1t3/rospanel-shu1t3/internal/extsub"
	"strings"

	"github.com/Shu1t3/rospanel-shu1t3/internal/branding"
	"github.com/Shu1t3/rospanel-shu1t3/internal/link"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// SubTitle is the per-user profile title: the configured subscription title (or
// "RosPanel" by default), optionally suffixed with the user name when
// SubNameInTitle is enabled.
func SubTitle(u model.User, set *model.Settings) string {
	base := strings.TrimSpace(set.SubTitle)
	if base == "" {
		// One source of truth for the stock name — it used to be duplicated here as
		// a literal, so rebranding the panel left the client profile title behind.
		base = branding.DefaultName
	}
	if set.SubNameInTitle {
		if name := strings.TrimSpace(u.Name); name != "" {
			return base + " — " + name
		}
	}
	return base
}

// clashProxy is one Clash proxy: its node name and the YAML flow-map line
// (already indented with "  - ").
type clashProxy struct {
	name string
	line string
}

// clashProxies builds the enabled-lane Clash proxy entries for a user on one server.
//
// Every stream-based lane carries `udp: true`. Mihomo defaults a proxy's UDP support
// to FALSE, and when a rule resolves to a proxy that can't take UDP it SKIPS that
// rule and keeps matching — the packet falls through to DIRECT instead of the tunnel.
// That is what broke Telegram calls on mihomo clients (Koala Clash, FlClashX): the
// voice UDP left untunneled and the censor dropped it. Xray-core clients have no such
// flag and were never affected, and Hysteria2 escaped it only because mihomo hardcodes
// UDP support for that protocol — hence "calls work only on the Hysteria lane".
func clashProxies(u model.User, srv Server) []clashProxy {
	set := srv.Set
	sv := "false" // skip-cert-verify: true only for a self-signed/IP cert
	if set.TLSInsecure {
		sv = "true"
	}
	out := make([]clashProxy, 0, 2+len(srv.Custom))
	if set.VLESSEnabled && srv.allowsBuiltin(model.LaneVLESS) {
		n := link.LabelFor(model.ProtoVLESS, u, set)
		out = append(out, clashProxy{n, fmt.Sprintf(
			"  - {name: %q, type: vless, server: %q, port: %d, uuid: %q, network: tcp, tls: true, udp: true, servername: %q, flow: xtls-rprx-vision, client-fingerprint: %s, skip-cert-verify: %s}",
			n, set.Host, set.VLESSPort, u.UUID, set.SNI, set.VLESSFP(), sv)})
	}
	// No public key, no dialable lane — see ShareLinks.
	if set.RealityEnabled && set.RealityPublicKey != "" && srv.allowsBuiltin(model.LaneReality) {
		n := link.LabelFor(model.ProtoReality, u, set)
		out = append(out, clashProxy{n, fmt.Sprintf(
			"  - {name: %q, type: vless, server: %q, port: %d, uuid: %q, network: xhttp, tls: true, udp: true, servername: %q, client-fingerprint: %s, reality-opts: {public-key: %q, short-id: %q}, xhttp-opts: {path: %q}}",
			n, set.Host, set.RealityPort, u.UUID, set.RealitySNI(), set.RealityFP(), set.RealityPublicKey, set.RealitySID(), set.RealityPathOr())})
	}
	if set.HysteriaEnabled && srv.allowsBuiltin(model.LaneHysteria) {
		hop := ""
		if set.HopEnd > set.HysteriaPort {
			hop = fmt.Sprintf(", ports: %q", fmt.Sprintf("%d-%d", model.HopAdvertised(set.HysteriaPort, set.HopStart), set.HopEnd))
		}
		n := link.LabelFor(model.ProtoHysteria, u, set)
		out = append(out, clashProxy{n, fmt.Sprintf(
			"  - {name: %q, type: hysteria2, server: %q, port: %d, password: %q, sni: %q, alpn: [h3], skip-cert-verify: %s%s%s}",
			n, set.Host, set.HysteriaPort, u.Password, set.SNI, sv, hop, clashObfs(set.HysteriaObfs))})
	}
	for _, in := range srv.Custom {
		if !srv.allowsInbound(in.ID) {
			continue
		}
		if p, ok := clashCustom(u, in, set, sv); ok {
			out = append(out, p)
		}
	}
	return out
}

// clashObfs renders mihomo's Salamander fields for a Hysteria2 proxy, or "" when
// the lane is not obfuscated. %q on the key is safe rather than decorative: the key
// is operator input, and an unquoted one would end the inline mapping early and cost
// the user every proxy in the profile.
func clashObfs(obfs string) string {
	if obfs == "" {
		return ""
	}
	return fmt.Sprintf(", obfs: salamander, obfs-password: %q", obfs)
}

// clashExternalProxies builds Clash proxy entries for allowed external servers.
func clashExternalProxies(access model.Access, ext []model.ExtServer) []clashProxy {
	endpoints := ExternalEndpoints(ext, access)
	out := make([]clashProxy, 0, len(endpoints))
	for _, e := range endpoints {
		if name, line, ok := extsub.ClashProxy(e); ok {
			out = append(out, clashProxy{name, line})
		}
	}
	return out
}

// clashCustom renders one custom inbound as a Clash proxy, or reports false when
// mihomo cannot express that protocol × transport (see model.SupportsClash). An
// inexpressible combination is DROPPED rather than approximated: a client that
// rejects one proxy entry usually rejects the whole profile, so a bad line would
// cost the user every other server too.
func clashCustom(u model.User, in model.Inbound, set *model.Settings, sv string) (clashProxy, bool) {
	if !model.SupportsClash(in.Protocol, in.Opts.Transport) {
		return clashProxy{}, false
	}
	o := in.Opts
	n := link.CustomLabelFor(in, u, set)

	if in.Protocol == model.InbHysteria {
		hop := ""
		if in.UsesHopping() {
			hop = fmt.Sprintf(", ports: %q", fmt.Sprintf("%d-%d", model.HopAdvertised(in.Port, o.HopStart), o.HopEnd))
		}
		return clashProxy{n, fmt.Sprintf(
			"  - {name: %q, type: hysteria2, server: %q, port: %d, password: %q, sni: %q, alpn: [h3], skip-cert-verify: %s%s%s}",
			n, set.Host, in.Port, u.Password, clashSNI(in, set), sv, hop, clashObfs(o.Obfs))}, true
	}

	if in.Protocol == model.InbShadowsocks {
		// mihomo's Shadowsocks-2022 shape: cipher is the method, and the password is
		// the server key and the user key joined by a colon (the multi-user form).
		pw := o.ShadowKey + ":" + model.UserShadowKey(u.UUID, o.Method)
		return clashProxy{n, fmt.Sprintf(
			"  - {name: %q, type: ss, server: %q, port: %d, cipher: %s, password: %q, udp: true}",
			n, set.Host, in.Port, o.Method, pw)}, true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  - {name: %q, type: %s, server: %q, port: %d",
		n, in.Protocol, set.Host, in.Port)
	if in.Protocol == model.InbVLESS {
		fmt.Fprintf(&b, ", uuid: %q", u.UUID)
		if o.Flow != "" {
			fmt.Fprintf(&b, ", flow: %s", o.Flow)
		}
	} else {
		fmt.Fprintf(&b, ", password: %q", u.Password)
	}
	fmt.Fprintf(&b, ", network: %s, udp: true", o.Transport)

	switch o.Security {
	case model.SecTLS:
		// Trojan spells the server name "sni", VLESS spells it "servername".
		key := "servername"
		if in.Protocol == model.InbTrojan {
			key = "sni"
		}
		fmt.Fprintf(&b, ", tls: true, %s: %q, client-fingerprint: %s, skip-cert-verify: %s",
			key, clashSNI(in, set), o.FPOr(), sv)
	case model.SecReality:
		fmt.Fprintf(&b, ", tls: true, servername: %q, client-fingerprint: %s, reality-opts: {public-key: %q, short-id: %q}",
			o.RealitySNI(), o.FPOr(), o.RealityPublicKey, firstShortID(o))
	}

	switch o.Transport {
	case model.TrWS:
		fmt.Fprintf(&b, ", ws-opts: {path: %q, headers: {Host: %q}}", o.Path, clashHost(in, set))
	case model.TrGRPC:
		fmt.Fprintf(&b, ", grpc-opts: {grpc-service-name: %q}", o.ServiceName)
	case model.TrXHTTP:
		fmt.Fprintf(&b, ", xhttp-opts: {path: %q", o.Path)
		if o.Host != "" {
			fmt.Fprintf(&b, ", host: %q", o.Host)
		}
		if o.Mode != "" {
			fmt.Fprintf(&b, ", mode: %q", o.Mode)
		}
		b.WriteString("}")
	}
	b.WriteString("}")
	return clashProxy{n, b.String()}, true
}

// clashSNI is the server name a custom inbound's client should present.
func clashSNI(in model.Inbound, set *model.Settings) string {
	if in.Opts.SNI != "" {
		return in.Opts.SNI
	}
	return set.SNI
}

// clashHost is the Host header for the HTTP-shaped transports (defaults to the SNI).
func clashHost(in model.Inbound, set *model.Settings) string {
	if in.Opts.Host != "" {
		return in.Opts.Host
	}
	return clashSNI(in, set)
}

// firstShortID is the REALITY shortId that goes into client configs (the server
// accepts any of the stored set).
func firstShortID(o model.InboundOpts) string {
	if ids := o.RealityShortIDs(); len(ids) > 0 {
		return ids[0]
	}
	return ""
}

// clashProxiesAll concatenates a user's proxy entries across physical servers and external servers.
// Names are unique because Settings.ProtoLabel appends the node label, and uniqueLabel ensures
// lanes whose names resolve to the same variable expansion never collide.
func clashProxiesAll(u model.User, servers []Server, ext []model.ExtServer, access model.Access) []clashProxy {
	out := make([]clashProxy, 0, len(servers)*2+len(ext))
	seen := map[string]int{}
	for _, srv := range servers {
		for _, p := range clashProxies(u, srv) {
			uniq := uniqueLabel(seen, p.name)
			if uniq != p.name {
				// The name is the first %q-quoted field of the line, so replacing its
				// first occurrence rewrites exactly the one that matters and leaves an
				// SNI or a password that happens to read the same alone.
				p.line = strings.Replace(p.line, fmt.Sprintf("%q", p.name), fmt.Sprintf("%q", uniq), 1)
				p.name = uniq
			}
			out = append(out, p)
		}
	}
	for _, p := range clashExternalProxies(access, ext) {
		uniq := uniqueLabel(seen, p.name)
		if uniq != p.name {
			p.line = strings.Replace(p.line, fmt.Sprintf("%q", p.name), fmt.Sprintf("%q", uniq), 1)
			p.name = uniq
		}
		out = append(out, p)
	}
	return out
}

// ClashYAML renders a minimal self-contained Clash-Meta config for one server.
func ClashYAML(u model.User, set *model.Settings) string {
	return GenerateClash(Request{User: u, Settings: set, Servers: One(set), Access: model.UnrestrictedAccess()})
}

// ClashYAMLMulti renders a Clash-Meta (Mihomo) config across physical servers (legacy helper).
func ClashYAMLMulti(u model.User, servers []Server) string {
	var set *model.Settings
	if len(servers) > 0 {
		set = servers[0].Set
	}
	return GenerateClash(Request{
		User:     u,
		Settings: set,
		Servers:  servers,
		Access:   model.UnrestrictedAccess(),
	})
}

// The two markers a mihomo template carries. Named constants because the operator's
// template, the validator and the injector all have to agree on them character for
// character — a template whose marker is a space out is silently served without any
// proxies in it.
const (
	clashProxiesMarker = "proxies: # LEAVE THIS LINE!"
	clashNamesMarker   = "    # LEAVE THIS LINE!"
)

// clashGroupName is the select-group name for the generated profile. Mihomo parses a
// rule line by splitting on commas, so a comma in the operator's subscription title
// (or the user name appended to it) would be read as a rule separator and shift the
// MATCH target — strip it here, where both the group definition and the rule read the
// same name.
func clashGroupName(u model.User, set *model.Settings) string {
	return strings.TrimSpace(strings.ReplaceAll(SubTitle(u, set), ",", " "))
}

// ClashWithTemplateMulti injects the user's proxies into a RoscomVPN-style Mihomo
// routing template (legacy helper).
func ClashWithTemplateMulti(u model.User, servers []Server, template string) string {
	var set *model.Settings
	if len(servers) > 0 {
		set = servers[0].Set
	}
	return GenerateClashWithTemplate(Request{
		User:     u,
		Settings: set,
		Servers:  servers,
		Access:   model.UnrestrictedAccess(),
	}, template)
}
