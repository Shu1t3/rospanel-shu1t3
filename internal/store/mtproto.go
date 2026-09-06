package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Shu1t3/rospanel-shu1t3/internal/auth"
	"github.com/Shu1t3/rospanel-shu1t3/internal/model"
	"github.com/Shu1t3/rospanel-shu1t3/internal/mtproto"
)

const mtprotoTokenPrefix = "mtp_"

const mtprotoColumns = `id, node_id, name, host, port, secret, domain, max_conns,
	enabled, token, created_at, last_seen, running, active_conns,
	bytes_read, bytes_written, uptime_sec, mem_alloc, rss, last_error`

func generateMTProtoToken() (string, error) {
	tok, err := auth.RandomToken()
	if err != nil {
		return "", fmt.Errorf("generate mtproto token: %w", err)
	}
	return mtprotoTokenPrefix + tok, nil
}

func scanMTProtoProxy(sc interface{ Scan(...any) error }) (*model.MTProtoProxy, error) {
	var p model.MTProtoProxy
	var enabled, running int
	var nodeID sql.NullInt64

	if err := sc.Scan(
		&p.ID, &nodeID, &p.Name, &p.Host, &p.Port, &p.Secret, &p.Domain, &p.MaxConns,
		&enabled, &p.Token, &p.CreatedAt, &p.LastSeen, &running, &p.ActiveConns,
		&p.BytesRead, &p.BytesWritten, &p.UptimeSec, &p.MemAlloc, &p.RSS, &p.LastError,
	); err != nil {
		return nil, err
	}

	p.Enabled = enabled != 0
	p.Running = running != 0
	if nodeID.Valid {
		v := nodeID.Int64
		p.NodeID = &v
	}
	return &p, nil
}

// CreateMTProtoProxy persists a new standalone MTProto proxy and generates a bearer token if empty.
func (s *Store) CreateMTProtoProxy(p *model.MTProtoProxy) error {
	if p.Token == "" {
		tok, err := generateMTProtoToken()
		if err != nil {
			return err
		}
		p.Token = tok
	}
	if p.Port <= 0 {
		p.Port = 8443
	}
	if p.Domain == "" {
		p.Domain = model.DefaultFakeTLSDomain
	}
	if p.MaxConns <= 0 {
		p.MaxConns = 512
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = time.Now().Unix()
	}

	res, err := s.db.Exec(`
		INSERT INTO mtproto_proxies (
			name, host, port, secret, domain, max_conns, enabled, token, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.Host, p.Port, p.Secret, p.Domain, p.MaxConns,
		boolToInt(p.Enabled), p.Token, p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create mtproto proxy: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	p.ID = id
	return nil
}

// GetMTProtoProxy returns one proxy by ID, or (nil, nil) if not found.
func (s *Store) GetMTProtoProxy(id int64) (*model.MTProtoProxy, error) {
	row := s.db.QueryRow(`SELECT `+mtprotoColumns+` FROM mtproto_proxies WHERE id = ?`, id)
	p, err := scanMTProtoProxy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get mtproto proxy %d: %w", id, err)
	}
	return p, nil
}

// GetMTProtoProxyByToken returns one proxy by its bearer token, or (nil, nil) if not found.
func (s *Store) GetMTProtoProxyByToken(token string) (*model.MTProtoProxy, error) {
	row := s.db.QueryRow(`SELECT `+mtprotoColumns+` FROM mtproto_proxies WHERE token = ?`, token)
	p, err := scanMTProtoProxy(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get mtproto proxy by token: %w", err)
	}
	return p, nil
}

// UpdateMTProtoProxy persists changes to a standalone proxy's operator-editable fields.
func (s *Store) UpdateMTProtoProxy(p model.MTProtoProxy) error {
	if p.Port <= 0 {
		p.Port = 8443
	}
	if p.Domain == "" {
		p.Domain = model.DefaultFakeTLSDomain
	}
	if p.MaxConns <= 0 {
		p.MaxConns = 512
	}
	res, err := s.db.Exec(`
		UPDATE mtproto_proxies SET name = ?, host = ?, port = ?, secret = ?,
			domain = ?, max_conns = ?, enabled = ? WHERE id = ?`,
		p.Name, p.Host, p.Port, p.Secret, p.Domain, p.MaxConns, boolToInt(p.Enabled), p.ID,
	)
	if err != nil {
		return fmt.Errorf("update mtproto proxy %d: %w", p.ID, err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteMTProtoProxy removes a standalone MTProto proxy.
func (s *Store) DeleteMTProtoProxy(id int64) error {
	_, err := s.db.Exec(`DELETE FROM mtproto_proxies WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete mtproto proxy %d: %w", id, err)
	}
	return nil
}

// SetMTProtoProxyEnabled updates the enabled toggle of a standalone proxy.
func (s *Store) SetMTProtoProxyEnabled(id int64, enabled bool) error {
	res, err := s.db.Exec(`UPDATE mtproto_proxies SET enabled = ? WHERE id = ?`, boolToInt(enabled), id)
	if err != nil {
		return fmt.Errorf("set mtproto proxy %d enabled: %w", id, err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateMTProtoHeartbeat updates live statistics reported by the standalone proxy runner.
func (s *Store) UpdateMTProtoHeartbeat(token string, snap mtproto.Snapshot) error {
	res, err := s.db.Exec(`
		UPDATE mtproto_proxies SET
			last_seen = unixepoch(), running = ?, active_conns = ?,
			bytes_read = ?, bytes_written = ?, uptime_sec = ?,
			mem_alloc = ?, rss = ?, last_error = ?
		WHERE token = ?`,
		boolToInt(snap.Running), snap.ActiveConns,
		snap.BytesRead, snap.BytesWritten, snap.UptimeSec,
		snap.MemAlloc, snap.RSS, snap.LastError,
		token,
	)
	if err != nil {
		return fmt.Errorf("update mtproto heartbeat: %w", err)
	}
	if aff, _ := res.RowsAffected(); aff == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListStandaloneMTProtoProxies returns all standalone proxies from mtproto_proxies.
func (s *Store) ListStandaloneMTProtoProxies() ([]model.MTProtoProxy, error) {
	rows, err := s.db.Query(`SELECT ` + mtprotoColumns + ` FROM mtproto_proxies ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list standalone mtproto proxies: %w", err)
	}
	defer rows.Close()

	var out []model.MTProtoProxy
	for rows.Next() {
		p, err := scanMTProtoProxy(rows)
		if err != nil {
			return nil, fmt.Errorf("scan mtproto proxy: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ListAllMTProtoProxies returns all MTProto proxies: standalone ones plus nodes running in mixed mode.
func (s *Store) ListAllMTProtoProxies() ([]model.MTProtoProxy, error) {
	standalone, err := s.ListStandaloneMTProtoProxies()
	if err != nil {
		return nil, err
	}

	nodes, err := s.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("list nodes for mtproto: %w", err)
	}

	set, err := s.GetSettings()
	if err != nil {
		return nil, fmt.Errorf("get settings for mtproto: %w", err)
	}

	out := make([]model.MTProtoProxy, 0, len(standalone)+len(nodes)+1)
	out = append(out, standalone...)

	if set.MTProto.Enabled {
		host := set.Host
		if host == "" {
			host = set.SNI
		}
		if host == "" {
			host = "127.0.0.1"
		}
		var localID int64 = model.LocalNodeID
		out = append(out, model.MTProtoProxy{
			ID:        0,
			NodeID:    &localID,
			NodeName:  "Основной сервер",
			Name:      "Основной сервер (MTProto)",
			Host:      host,
			Port:      set.MTProto.Port,
			Secret:    set.MTProto.Secret,
			Domain:    set.MTProto.Domain,
			MaxConns:  set.MTProto.MaxConns,
			Enabled:   set.MTProto.Enabled,
			CreatedAt: 0,
			LastSeen:  time.Now().Unix(),
			Running:   true,
		})
	}

	now := time.Now().Unix()
	for i := range nodes {
		n := &nodes[i]
		if !n.MTProto.Enabled {
			continue
		}
		nodeID := n.ID
		running := n.Online(now) && n.XrayRunning
		out = append(out, model.MTProtoProxy{
			ID:        -n.ID, // negative ID to distinguish node proxy from standalone proxy
			NodeID:    &nodeID,
			NodeName:  n.Name,
			Name:      n.Name + " (MTProto)",
			Host:      n.Host,
			Port:      n.MTProto.Port,
			Secret:    n.MTProto.Secret,
			Domain:    n.MTProto.Domain,
			MaxConns:  n.MTProto.MaxConns,
			Enabled:   n.Enabled && n.MTProto.Enabled,
			CreatedAt: n.CreatedAt,
			LastSeen:  n.LastSeen,
			Running:   running,
		})
	}

	return out, nil
}
