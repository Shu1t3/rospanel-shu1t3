package core

import (
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xhttpfront"
	"github.com/Shu1t3/rospanel-shu1t3/internal/xray"
)

// syncXHTTP points the VLESS-XHTTP-TLS lane's path at the inbound of the config just put
// in force, or takes the route down when the lane is off. Its clients' sockets are on
// the TCP-TLS lane's port, so that is where a removed user's connections are closed.
func (m *Manager) syncXHTTP(set *model.Settings, cfg *xray.Config) {
	path, addr, ok := xray.XHTTPRoute(cfg)
	if !ok {
		m.xhttp.SetRoutes(nil)
		m.sup.SetPublicPort(xray.TagXHTTP, 0)
		return
	}
	m.xhttp.SetRoutes([]xhttpfront.Route{{Path: path, Addr: addr}})
	m.sup.SetPublicPort(xray.TagXHTTP, set.VLESSPort)
}
