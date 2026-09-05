-- Remove experimental node rentals and tenant resource sharing.

DELETE FROM inbounds WHERE tenant_id <> '';
DELETE FROM nodes WHERE is_rented = 1;
DROP TABLE IF EXISTS node_tenants;

ALTER TABLE nodes DROP COLUMN share_enabled;
ALTER TABLE nodes DROP COLUMN share_quota_percent;
ALTER TABLE nodes DROP COLUMN share_speed_limit;
ALTER TABLE nodes DROP COLUMN share_token;
ALTER TABLE nodes DROP COLUMN is_rented;
ALTER TABLE nodes DROP COLUMN rent_owner_node_id;
ALTER TABLE nodes DROP COLUMN rent_share_key;
ALTER TABLE nodes DROP COLUMN rent_tenant_id;
ALTER TABLE nodes DROP COLUMN rent_master_host;

ALTER TABLE inbounds DROP COLUMN tenant_id;

ALTER TABLE settings DROP COLUMN share_enabled;
ALTER TABLE settings DROP COLUMN share_quota_percent;
ALTER TABLE settings DROP COLUMN share_speed_limit;
ALTER TABLE settings DROP COLUMN share_token;
