package sub

import (
	"fmt"

	"github.com/Shu1t3/rospanel-shu1t3/internal/extsub"
	"github.com/Shu1t3/rospanel-shu1t3/internal/link"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// SingBoxJSON renders an importable sing-box configuration for a single server.
func SingBoxJSON(u model.User, set *model.Settings) string {
	return GenerateSingBox(Request{
		User:     u,
		Settings: set,
		Servers:  One(set),
		Access:   model.UnrestrictedAccess(),
	})
}

// singboxProxies builds the protocol outbounds + their tags for one server. Tags
// carry the node label (see Settings.ProtoLabel) so multi-node configs stay
// unambiguous.
func singboxProxies(u model.User, srv Server) (proxies []any, tags []string) {
	set := srv.Set
	nV := link.Label(model.ProtoVLESS, set)
	nH := link.Label(model.ProtoHysteria, set)
	insecure := set.TLSInsecure // true only for a self-signed/IP cert

	vless := map[string]any{
		"type": "vless", "tag": nV, "server": set.Host, "server_port": set.VLESSPort,
		"uuid": u.UUID, "flow": "xtls-rprx-vision",
		"tls": map[string]any{
			"enabled": true, "server_name": set.SNI, "insecure": insecure,
			"utls": map[string]any{"enabled": true, "fingerprint": set.VLESSFP()},
		},
	}
	hy2 := map[string]any{
		"type": "hysteria2", "tag": nH, "server": set.Host, "server_port": set.HysteriaPort,
		"password": u.Password,
		"tls": map[string]any{
			"enabled": true, "server_name": set.SNI, "alpn": []string{"h3"}, "insecure": insecure,
		},
	}
	if set.HopEnd > set.HysteriaPort {
		// Port hopping: a range replaces the single server_port.
		hy2["server_ports"] = []string{fmt.Sprintf("%d:%d", model.HopAdvertised(set.HysteriaPort, set.HopStart), set.HopEnd)}
		hy2["hop_interval"] = "10s"
		delete(hy2, "server_port")
	}

	// Anti-DPI shaping of the generated config (client-side only; no server change).
	// ClientHello fragmentation (sing-box ≥1.12) defeats stateless SNI inspection on
	// the one lane whose handshake carries our real SNI — VLESS-Vision. REALITY hides
	// its SNI behind the donor and Hysteria2 is QUIC, so neither is fragmented here.
	// Fragmenting sits below the TLS record layer, so it doesn't disturb Vision's flow.
	if set.TLSFragment {
		vless["tls"].(map[string]any)["fragment"] = true
		// Record-level split on top (sing-box ≥1.12): survives a middlebox that
		// reassembles TCP segments before it looks at the ClientHello.
		if set.SubDPI.RecordFragment {
			vless["tls"].(map[string]any)["record_fragment"] = true
		}
	}
	// ALPN consistency on the Vision lane: the :443 inbound offers [h2,http/1.1];
	// offering the same aligns the ClientHello with a real browser to that cert.
	vless["tls"].(map[string]any)["alpn"] = []string{"h2", "http/1.1"}

	// Only the lanes enabled in the Connections panel become outbounds; tags collects
	// them in the same order for the selector/urltest groups.
	//
	// The built-in REALITY lane is deliberately absent: it runs on XHTTP, for which
	// sing-box has no transport at all. Xray-core clients still get it through the
	// universal link list — see model.SupportsSingBox, which applies the same rule to
	// custom inbounds.
	if set.VLESSEnabled && srv.allowsBuiltin(model.LaneVLESS) {
		proxies = append(proxies, vless)
		tags = append(tags, nV)
	}
	if set.HysteriaEnabled && srv.allowsBuiltin(model.LaneHysteria) {
		proxies = append(proxies, hy2)
		tags = append(tags, nH)
	}
	for _, in := range srv.Custom {
		if !srv.allowsInbound(in.ID) {
			continue
		}
		if o, tag, ok := singboxCustom(u, in, set); ok {
			proxies = append(proxies, o)
			tags = append(tags, tag)
		}
	}
	return proxies, tags
}

// singboxExternalProxies builds sing-box outbounds + tags for allowed external servers.
func singboxExternalProxies(access model.Access, ext []model.ExtServer) (proxies []any, tags []string) {
	for _, e := range ExternalEndpoints(ext, access) {
		if o, ok := extsub.SingBoxOutbound(e, e.Name); ok {
			proxies = append(proxies, o)
			tags = append(tags, e.Name)
		}
	}
	return proxies, tags
}

// singboxCustom renders one custom inbound as a sing-box outbound, or reports false
// when sing-box cannot express that protocol × transport (see model.SupportsSingBox
// — most notably it has no XHTTP at all). Dropped rather than approximated, for the
// same reason as the Clash side.
func singboxCustom(u model.User, in model.Inbound, set *model.Settings) (map[string]any, string, bool) {
	if !model.SupportsSingBox(in.Protocol, in.Opts.Transport) {
		return nil, "", false
	}
	o := in.Opts
	tag := link.CustomLabel(in, set)

	if in.Protocol == model.InbHysteria {
		out := map[string]any{
			"type": "hysteria2", "tag": tag, "server": set.Host, "server_port": in.Port,
			"password": u.Password,
			"tls": map[string]any{
				"enabled": true, "server_name": clashSNI(in, set),
				"alpn": []string{"h3"}, "insecure": set.TLSInsecure,
			},
		}
		if in.UsesHopping() {
			out["server_ports"] = []string{fmt.Sprintf("%d:%d", model.HopAdvertised(in.Port, o.HopStart), o.HopEnd)}
			out["hop_interval"] = "10s"
			delete(out, "server_port")
		}
		return out, tag, true
	}

	if in.Protocol == model.InbShadowsocks {
		// sing-box's shadowsocks: method plus the server-key:user-key password. It
		// relays UDP by default, so there is no udp flag to set as there is on Clash.
		return map[string]any{
			"type": "shadowsocks", "tag": tag, "server": set.Host, "server_port": in.Port,
			"method":   o.Method,
			"password": o.ShadowKey + ":" + model.UserShadowKey(u.UUID, o.Method),
		}, tag, true
	}

	out := map[string]any{
		"type": in.Protocol, "tag": tag, "server": set.Host, "server_port": in.Port,
	}
	if in.Protocol == model.InbVLESS {
		out["uuid"] = u.UUID
		if o.Flow != "" {
			out["flow"] = o.Flow
		}
	} else {
		out["password"] = u.Password
	}

	switch o.Security {
	case model.SecTLS:
		tls := map[string]any{
			"enabled": true, "server_name": clashSNI(in, set), "insecure": set.TLSInsecure,
			"utls": map[string]any{"enabled": true, "fingerprint": o.FPOr()},
		}
		if o.Transport == model.TrGRPC {
			tls["alpn"] = []string{"h2"}
		}
		out["tls"] = tls
	case model.SecReality:
		out["tls"] = map[string]any{
			"enabled": true, "server_name": o.RealitySNI(),
			"utls":    map[string]any{"enabled": true, "fingerprint": o.FPOr()},
			"reality": map[string]any{"enabled": true, "public_key": o.RealityPublicKey, "short_id": firstShortID(o)},
		}
	}

	switch o.Transport {
	case model.TrWS:
		out["transport"] = map[string]any{
			"type": "ws", "path": o.Path,
			"headers": map[string]any{"Host": clashHost(in, set)},
		}
	case model.TrHTTPUpgrade:
		out["transport"] = map[string]any{
			"type": "httpupgrade", "path": o.Path, "host": clashHost(in, set),
		}
	case model.TrGRPC:
		out["transport"] = map[string]any{"type": "grpc", "service_name": o.ServiceName}
	}
	return out, tag, true
}

// SingBoxJSONMulti renders a sing-box config spanning every server (legacy helper).
func SingBoxJSONMulti(u model.User, servers []Server) string {
	var set *model.Settings
	if len(servers) > 0 {
		set = servers[0].Set
	}
	return GenerateSingBox(Request{
		User:     u,
		Settings: set,
		Servers:  servers,
		Access:   model.UnrestrictedAccess(),
	})
}
