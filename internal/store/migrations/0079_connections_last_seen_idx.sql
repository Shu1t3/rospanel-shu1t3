-- Speed up PurgeConnections retention sweeps and range queries on connection activity.
CREATE INDEX IF NOT EXISTS idx_connections_last_seen ON connections(last_seen);
