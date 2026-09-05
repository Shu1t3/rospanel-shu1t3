-- Speed up WorkingUsers device limit queries: seek directly by user_id and last_seen.
CREATE INDEX IF NOT EXISTS idx_connections_user_last_seen ON connections(user_id, last_seen, ip);
