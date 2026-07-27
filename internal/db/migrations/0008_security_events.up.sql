-- Security telemetry (failed-login lockouts, and future auth events).
--
-- Deliberately NOT stored in audit_logs: that table is the CO art. 957a
-- accounting integrity chain (user_id FK, table_name/record_id, before/after
-- state, hash chain). A lockout usually concerns an address that does not map
-- to any user — an attacker guessing e-mails — so it cannot satisfy the
-- user_id foreign key, and injecting non-accounting rows would dilute a chain
-- whose purpose is proving the books were not altered.
--
-- nLPD: an IP address is personal data. Rows are retention-limited rather than
-- kept for the ten years the CO requires of accounting records.
CREATE TABLE IF NOT EXISTS security_events (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    event_type  TEXT NOT NULL,          -- e.g. 'login_lockout'
    ip_address  TEXT,                   -- personal data (nLPD) — retention-limited
    detail      TEXT,                   -- free-form context, no credentials
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_security_events_created ON security_events(created_at);
CREATE INDEX IF NOT EXISTS idx_security_events_type    ON security_events(event_type, created_at);
