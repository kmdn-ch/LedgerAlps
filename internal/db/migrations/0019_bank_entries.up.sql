-- Rapprochement bancaire : les écritures importées d'un camt.053.
--
-- L'import existait déjà, mais il ne gardait rien : le relevé était analysé,
-- renvoyé en JSON, et oublié. Impossible dans ces conditions de savoir ce qui
-- avait déjà été rapproché, ni de réimporter un relevé sans tout revoir.
--
-- La clé d'unicité est une empreinte, pas la référence bancaire seule : toutes
-- les banques ne renseignent pas AcctSvcrRef, et deux versements identiques du
-- même client le même jour existent. L'empreinte combine ce qui identifie
-- vraiment une opération.
CREATE TABLE IF NOT EXISTS bank_entries (
    id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    fingerprint   TEXT NOT NULL UNIQUE,

    amount        REAL NOT NULL,
    currency      TEXT NOT NULL DEFAULT 'CHF',
    is_credit     INTEGER NOT NULL,
    booking_date  DATE NOT NULL,
    value_date    DATE,

    bank_ref      TEXT NOT NULL DEFAULT '',
    end_to_end_ref TEXT NOT NULL DEFAULT '',
    qr_reference  TEXT NOT NULL DEFAULT '',
    counterparty  TEXT NOT NULL DEFAULT '',
    remittance    TEXT NOT NULL DEFAULT '',

    -- Rapprochement. NULL tant que personne n'a tranché : une suggestion n'est
    -- pas un rapprochement, et les confondre ferait passer pour réglées des
    -- factures que personne n'a vérifiées.
    invoice_id    TEXT REFERENCES invoices(id),
    matched_at    TIMESTAMP,
    matched_by_id TEXT REFERENCES users(id),
    -- ignored : l'utilisateur a décidé que cette ligne ne concerne aucune
    -- facture (frais bancaires, virement interne). Distinct de « pas encore
    -- regardé », sinon la liste ne se vide jamais et on cesse de la lire.
    ignored       INTEGER NOT NULL DEFAULT 0,

    imported_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_bank_entries_unmatched
    ON bank_entries(ignored, invoice_id, booking_date);
CREATE INDEX IF NOT EXISTS idx_bank_entries_qrref
    ON bank_entries(qr_reference);
