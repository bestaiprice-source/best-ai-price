-- 152_add_channel_monitor_maintenance.sql
-- Planned maintenance window for channel monitors.
--
-- While maintenance_until is in the future the monitor is reported as
-- "maintenance" on every user-facing surface (dashboard health block, model
-- cards, the channel-status screen) and excluded from the uptime percentage —
-- a planned outage must not read as an availability drop. The probe keeps
-- recording so the history timeline stays complete once the window closes.
--
-- Additive only: existing rows keep working untouched (both columns are
-- nullable / defaulted), and NULL means "not in maintenance".

ALTER TABLE channel_monitors ADD COLUMN IF NOT EXISTS maintenance_until TIMESTAMPTZ;
ALTER TABLE channel_monitors ADD COLUMN IF NOT EXISTS maintenance_note VARCHAR(200) NOT NULL DEFAULT '';
