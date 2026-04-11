-- Sprint 9: add is_company, legal_name, uid_number, notes to contacts

ALTER TABLE contacts ADD COLUMN is_company  INTEGER NOT NULL DEFAULT 0;
ALTER TABLE contacts ADD COLUMN legal_name  TEXT;
ALTER TABLE contacts ADD COLUMN uid_number  TEXT;
ALTER TABLE contacts ADD COLUMN notes       TEXT;
