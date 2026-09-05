package sub

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/Shu1t3/rospanel-shu1t3/internal/i18n"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// Request defines the independent sources and parameters required for generating
// a client or human-facing subscription.
type Request struct {
	User     model.User
	Settings *model.Settings
	Servers  []Server          // Physical nodes
	External []model.ExtServer // External servers (not bound to any physical node)
	Access   model.Access      // User's group access grants
	DPI      model.SubDPI
}

// ensureSettings ensures a non-nil Settings pointer is available.
func (r *Request) ensureSettings() *model.Settings {
	if r.Settings != nil {
		return r.Settings
	}
	if len(r.Servers) > 0 && r.Servers[0].Set != nil {
		return r.Servers[0].Set
	}
	return &model.Settings{}
}

// GenerateShareLinks produces the complete list of universal share links across
// all allowed physical lanes and external servers.
func GenerateShareLinks(req Request) []string {
	var links []string
	for _, srv := range req.Servers {
		links = append(links, ShareLinks(req.User, srv)...)
	}
	links = append(links, ExternalShareLinks(req.External, req.Access)...)
	return links
}

// GenerateClash produces a complete Clash-Meta (Mihomo) configuration from independent
// physical and external server sources.
func GenerateClash(req Request) string {
	set := req.ensureSettings()
	proxies := clashProxiesAll(req.User, req.Servers, req.External, req.Access)
	if len(proxies) == 0 && len(req.Servers) == 0 && len(req.External) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("dns:\n" +
		"  enable: true\n" +
		"  enhanced-mode: fake-ip\n" +
		"  nameserver: [\"https://1.1.1.1/dns-query\", \"https://dns.google/dns-query\"]\n")
	b.WriteString("proxies:\n")
	quoted := make([]string, len(proxies))
	for i, p := range proxies {
		b.WriteString(p.line)
		b.WriteByte('\n')
		quoted[i] = fmt.Sprintf("%q", p.name)
	}
	group := clashGroupName(req.User, set)
	if len(proxies) == 0 {
		b.WriteString("rules:\n  - \"MATCH,DIRECT\"\n")
		return b.String()
	}
	fmt.Fprintf(&b,
		"proxy-groups:\n  - {name: %q, type: select, proxies: [%s]}\n",
		group, strings.Join(quoted, ", "))
	b.WriteString("rules:\n")
	if set.BlockQUIC {
		b.WriteString("  - AND,((NETWORK,udp),(DST-PORT,443)),REJECT\n")
	}
	fmt.Fprintf(&b, "  - %q\n", "MATCH,"+group)
	return b.String()
}

// GenerateClashWithTemplate injects the user's proxies into a routing template.
func GenerateClashWithTemplate(req Request, template string) string {
	proxies := clashProxiesAll(req.User, req.Servers, req.External, req.Access)
	if len(proxies) == 0 || !strings.Contains(template, clashProxiesMarker) {
		return GenerateClash(req)
	}
	defs := make([]string, len(proxies))
	for i, p := range proxies {
		defs[i] = p.line
	}
	out := strings.Replace(template,
		clashProxiesMarker,
		"proxies:\n"+strings.Join(defs, "\n"),
		1,
	)
	var names strings.Builder
	for _, p := range proxies {
		fmt.Fprintf(&names, "      - %q\n", p.name)
	}
	return strings.Replace(out, clashNamesMarker, strings.TrimRight(names.String(), "\n"), 1)
}

// GenerateSingBox produces a sing-box JSON configuration spanning physical and external servers.
func GenerateSingBox(req Request) string {
	set := req.ensureSettings()
	if len(req.Servers) == 0 && len(req.External) == 0 {
		return "{}"
	}

	proxies, tags := singboxProxiesAll(req)

	group := SubTitle(req.User, set)
	if len(tags) == 0 {
		out, err := json.MarshalIndent(map[string]any{
			"log":       map[string]any{"level": "warn"},
			"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
			"route":     map[string]any{"final": "direct"},
		}, "", "  ")
		if err != nil {
			return "{}"
		}
		return string(out)
	}

	outbounds := []any{
		map[string]any{"type": "selector", "tag": group, "outbounds": append([]string{"auto"}, tags...), "default": "auto"},
		map[string]any{"type": "urltest", "tag": "auto", "outbounds": tags,
			"url": "https://www.gstatic.com/generate_204", "interval": "5m"},
	}
	outbounds = append(outbounds, proxies...)
	outbounds = append(outbounds, map[string]any{"type": "direct", "tag": "direct"})

	dnsServers := []any{
		map[string]any{"tag": "remote", "address": "https://1.1.1.1/dns-query", "detour": group},
	}
	dns := map[string]any{"servers": dnsServers, "final": "remote", "strategy": "prefer_ipv4"}
	var bootstrapHosts []string
	for _, srv := range req.Servers {
		if net.ParseIP(srv.Set.Host) == nil && srv.Set.Host != "" {
			bootstrapHosts = append(bootstrapHosts, srv.Set.Host)
		}
	}
	for _, e := range ExternalEndpoints(req.External, req.Access) {
		if net.ParseIP(e.Host) == nil && e.Host != "" {
			bootstrapHosts = append(bootstrapHosts, e.Host)
		}
	}
	if len(bootstrapHosts) > 0 {
		dns["servers"] = append(dnsServers,
			map[string]any{"tag": "bootstrap", "address": "https://223.5.5.5/dns-query", "detour": "direct"})
		dns["rules"] = []any{map[string]any{"domain": bootstrapHosts, "server": "bootstrap"}}
	}

	routeRules := []any{
		map[string]any{"action": "sniff"},
		map[string]any{"protocol": "dns", "action": "hijack-dns"},
	}
	if set.BlockQUIC {
		routeRules = append(routeRules, map[string]any{"network": "udp", "port": 443, "action": "reject"})
	}
	routeRules = append(routeRules, map[string]any{"ip_is_private": true, "outbound": "direct"})

	cfg := map[string]any{
		"log": map[string]any{"level": "warn"},
		"dns": dns,
		"inbounds": []any{
			map[string]any{
				"type": "tun", "tag": "tun-in",
				"address":      []string{"172.19.0.1/30"},
				"auto_route":   true,
				"strict_route": true,
			},
		},
		"outbounds": outbounds,
		"route":     map[string]any{"rules": routeRules, "final": group},
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(out)
}

// GenerateSingBoxWithTemplate renders outbounds into the operator's own sing-box document.
// {{proxies}} takes the generated outbounds, {{tags}} their tags and {{group}} the profile's
// name. Falls back to GenerateSingBox whenever the template cannot produce a working profile.
func GenerateSingBoxWithTemplate(req Request, template string) (string, error) {
	if strings.TrimSpace(template) == "" {
		return GenerateSingBox(req), nil
	}
	if len(req.Servers) == 0 && len(req.External) == 0 {
		return GenerateSingBox(req), nil
	}
	proxies, tags := singboxProxiesAll(req)
	if len(tags) == 0 {
		return GenerateSingBox(req), nil
	}
	tagList := make([]any, len(tags))
	for i, t := range tags {
		tagList[i] = t
	}
	set := req.ensureSettings()
	out, err := renderJSONTemplate(template,
		map[string]any{TplGroup: SubTitle(req.User, set)},
		map[string][]any{TplProxies: proxies, TplTags: tagList},
	)
	if err != nil {
		return GenerateSingBox(req), err
	}
	return out, nil
}

// singboxProxiesAll gathers every server's outbounds, giving each a tag no other
// outbound shares. A duplicate tag is fatal in sing-box — the selector would name it
// twice and the profile is refused — so the de-duplication is not cosmetic; see
// uniqueLabel for how two differently-named lanes end up asking for the same tag.
func singboxProxiesAll(req Request) ([]any, []string) {
	var proxies []any
	var tags []string
	seen := map[string]int{}
	for _, srv := range req.Servers {
		p, t := singboxProxies(req.User, srv)
		for i := range p {
			uniq := uniqueLabel(seen, t[i])
			if uniq != t[i] {
				if m, ok := p[i].(map[string]any); ok {
					m["tag"] = uniq
				}
				t[i] = uniq
			}
			proxies = append(proxies, p[i])
			tags = append(tags, t[i])
		}
	}
	ep, et := singboxExternalProxies(req.Access, req.External)
	for i := range ep {
		uniq := uniqueLabel(seen, et[i])
		if uniq != et[i] {
			if m, ok := ep[i].(map[string]any); ok {
				m["tag"] = uniq
			}
			et[i] = uniq
		}
		proxies = append(proxies, ep[i])
		tags = append(tags, et[i])
	}
	return proxies, tags
}

// GenerateXrayJSON produces Xray client JSON array from all physical and external endpoints.
func GenerateXrayJSON(req Request) string {
	configs := make([]map[string]any, 0, 8)
	for _, l := range GenerateShareLinks(req) {
		if cfg, ok := xrayConfigFromLink(l, req.DPI); ok {
			configs = append(configs, cfg)
		}
	}
	b, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return "[]"
	}
	return string(b)
}

// GenerateXrayJSONWithTemplate renders each lane into the operator's own Xray config.
// The template is ONE config — the format is an array of independent configs, one per lane —
// so {{outbounds}} takes that lane's outbound chain and {{remarks}} its display name.
// Falls back to GenerateXrayJSON on an unparseable template or rendering error.
func GenerateXrayJSONWithTemplate(req Request, template string) (string, error) {
	if strings.TrimSpace(template) == "" {
		return GenerateXrayJSON(req), nil
	}
	configs := make([]any, 0, 8)
	for _, l := range GenerateShareLinks(req) {
		cfg, ok := xrayConfigFromLink(l, req.DPI)
		if !ok {
			continue
		}
		outbounds, ok := cfg["outbounds"].([]map[string]any)
		if !ok || len(outbounds) == 0 {
			return GenerateXrayJSON(req), fmt.Errorf("lane %q produced no outbound chain", cfg["remarks"])
		}
		chain := make([]any, len(outbounds))
		for i, o := range outbounds {
			chain[i] = o
		}
		rendered, err := renderJSONTemplate(template,
			map[string]any{TplRemarks: cfg["remarks"]},
			map[string][]any{TplOutbounds: chain},
		)
		if err != nil {
			return GenerateXrayJSON(req), err
		}
		var doc any
		if err := json.Unmarshal([]byte(rendered), &doc); err != nil {
			return GenerateXrayJSON(req), err
		}
		configs = append(configs, doc)
	}
	if len(configs) == 0 {
		return GenerateXrayJSON(req), nil
	}
	b, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return GenerateXrayJSON(req), err
	}
	return string(b), nil
}

// GeneratePage renders the subscription HTML page spanning physical and external servers.
func GeneratePage(req Request, billing Billing, devices Devices, showDownload bool, lang i18n.Lang) ([]byte, error) {
	return PageWithSources(req.User, req.ensureSettings(), req.Servers, req.External, req.Access, billing, devices, showDownload, lang)
}
