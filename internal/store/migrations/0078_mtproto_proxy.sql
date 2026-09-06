-- MTProto-proxy (model.MTProtoConfig / model.MTProtoProxy):
-- Support for embedded FakeTLS Telegram proxy on master, remote nodes (mixed mode),
-- and standalone proxies managed on the Servers tab.

ALTER TABLE settings ADD COLUMN mtproto_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE settings ADD COLUMN mtproto_port INTEGER NOT NULL DEFAULT 8443;
ALTER TABLE settings ADD COLUMN mtproto_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE settings ADD COLUMN mtproto_domain TEXT NOT NULL DEFAULT 'cloudflare.com';
ALTER TABLE settings ADD COLUMN mtproto_max_conns INTEGER NOT NULL DEFAULT 512;

ALTER TABLE nodes ADD COLUMN mtproto_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN mtproto_port INTEGER NOT NULL DEFAULT 8443;
ALTER TABLE nodes ADD COLUMN mtproto_secret TEXT NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN mtproto_domain TEXT NOT NULL DEFAULT 'cloudflare.com';
ALTER TABLE nodes ADD COLUMN mtproto_max_conns INTEGER NOT NULL DEFAULT 512;

CREATE TABLE IF NOT EXISTS mtproto_proxies (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id       INTEGER REFERENCES nodes(id) ON DELETE CASCADE,
    name          TEXT    NOT NULL DEFAULT '',
    host          TEXT    NOT NULL DEFAULT '',
    port          INTEGER NOT NULL DEFAULT 8443,
    secret        TEXT    NOT NULL DEFAULT '',
    domain        TEXT    NOT NULL DEFAULT 'cloudflare.com',
    max_conns     INTEGER NOT NULL DEFAULT 512,
    enabled       INTEGER NOT NULL DEFAULT 1,
    token         TEXT    NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    last_seen     INTEGER NOT NULL DEFAULT 0,
    running       INTEGER NOT NULL DEFAULT 0,
    active_conns  INTEGER NOT NULL DEFAULT 0,
    bytes_read    INTEGER NOT NULL DEFAULT 0,
    bytes_written INTEGER NOT NULL DEFAULT 0,
    uptime_sec    INTEGER NOT NULL DEFAULT 0,
    mem_alloc     INTEGER NOT NULL DEFAULT 0,
    rss           INTEGER NOT NULL DEFAULT 0,
    last_error    TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_mtproto_proxies_token ON mtproto_proxies(token);
CREATE INDEX IF NOT EXISTS idx_mtproto_proxies_node ON mtproto_proxies(node_id);
