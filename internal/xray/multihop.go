package xray

import (
	"slices"

	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
)

// MultiHopLocalExit explicitly enables terminal egress on a single-node route.
const MultiHopLocalExit = "local-exit"

// Each leg has a dedicated rule. A missing or unusable next leg keeps the rule
// in place, pointing at blackhole instead of falling through to ordinary egress.
func multiHopRouting(opts Options, rc model.RoutingConfig) ([]Outbound, []RouteRule) {
	var outbounds []Outbound
	var rules []RouteRule
	add := func(rule RouteRule, link string, exit bool) {
		rule.Type = "field"
		rule.OutboundTag = "block"
		switch {
		case exit:
			const tag = "multi-hop-exit"
			if !slices.ContainsFunc(outbounds, func(o Outbound) bool { return o.Tag == tag }) {
				outbounds = append(outbounds, Outbound{
					Tag: tag, Protocol: "freedom",
					Settings: map[string]any{
						"domainStrategy": rc.DirectStrategy,
						"finalRules": []map[string]any{
							{"action": "block", "ip": model.PrivateEgressCIDRs, "blockDelay": "0"},
						},
					},
				})
			}
			rule.OutboundTag = tag
		case link != "":
			if o, ok := linkOutbound(link, rule.RuleTag, opts.LegacyUDPHop); ok {
				outbounds = append(outbounds, Outbound{
					Tag: rule.RuleTag, Protocol: "vless", Settings: o["settings"], StreamSettings: o["streamSettings"],
				})
				rule.OutboundTag = rule.RuleTag
			}
		}
		rules = append(rules, rule)
	}
	for _, in := range opts.Custom {
		if in.Enabled && in.Opts.MultiHop != nil {
			tag := in.Tag()
			link := opts.MultiHopLinks[in.ID]
			add(RouteRule{InboundTag: []string{tag}, RuleTag: "multi-hop-" + tag}, link, link == MultiHopLocalExit)
		}
	}
	for _, bridge := range opts.Bridges {
		if hop := bridge.MultiHop; hop != nil {
			add(RouteRule{User: []string{bridge.Email}, RuleTag: "multi-hop-" + bridge.Email}, hop.Next, hop.Exit)
		}
	}
	return outbounds, rules
}
