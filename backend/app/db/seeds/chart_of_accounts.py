"""
LedgerAlps — Plan comptable PME suisse initial
Seed script — à exécuter une seule fois après la création des tables.

Source : KMU-Kontenrahmen / Plan comptable PME suisse (édition fiduciaire)
"""

SWISS_PME_ACCOUNTS = [
    # ─── 1000–1999 : Actif circulant ─────────────────────────────────────────
    {"number": "1000", "name": "Caisse CHF",                   "type": "asset"},
    {"number": "1010", "name": "Caisse EUR",                   "type": "asset"},
    {"number": "1020", "name": "Banque — compte courant",      "type": "asset"},
    {"number": "1021", "name": "Banque — compte épargne",      "type": "asset"},
    {"number": "1060", "name": "Titres cotés",                 "type": "asset"},
    {"number": "1100", "name": "Créances clients CHF",         "type": "asset"},
    {"number": "1101", "name": "Créances clients EUR",         "type": "asset"},
    {"number": "1170", "name": "TVA déductible (input tax)",   "type": "asset"},
    {"number": "1176", "name": "TVA — solde AFC",              "type": "asset"},
    {"number": "1200", "name": "Stocks marchandises",          "type": "asset"},
    {"number": "1210", "name": "Stocks matières premières",    "type": "asset"},
    {"number": "1300", "name": "Charges payées d'avance",      "type": "asset"},
    {"number": "1301", "name": "Produits à recevoir",          "type": "asset"},

    # ─── 2000–2999 : Actif immobilisé ────────────────────────────────────────
    {"number": "2000", "name": "Participations",               "type": "asset"},
    {"number": "2800", "name": "Immobilisations corporelles",  "type": "asset"},
    {"number": "2880", "name": "Amortissements cumulés",       "type": "asset"},
    {"number": "2900", "name": "Immobilisations incorporelles","type": "asset"},

    # ─── 3000–3999 : Dettes à court terme ────────────────────────────────────
    {"number": "3000", "name": "Dettes fournisseurs CHF",      "type": "liability"},
    {"number": "3001", "name": "Dettes fournisseurs EUR",      "type": "liability"},
    {"number": "3200", "name": "Dettes fiscales",              "type": "liability"},
    {"number": "3201", "name": "TVA collectée (output tax)",   "type": "liability"},
    {"number": "3220", "name": "Déductions sociales à payer",  "type": "liability"},
    {"number": "3500", "name": "Produits reçus d'avance",      "type": "liability"},
    {"number": "3501", "name": "Charges à payer",              "type": "liability"},

    # ─── 4000–4999 : Dettes à long terme ─────────────────────────────────────
    {"number": "4000", "name": "Emprunts bancaires LT",        "type": "liability"},
    {"number": "4100", "name": "Obligations",                  "type": "liability"},
    {"number": "4800", "name": "Impôts différés",              "type": "liability"},

    # ─── 5000–5999 : Capitaux propres ────────────────────────────────────────
    {"number": "5000", "name": "Capital social",               "type": "equity"},
    {"number": "5200", "name": "Réserves légales",             "type": "equity"},
    {"number": "5900", "name": "Bénéfice / perte reporté(e)", "type": "equity"},
    {"number": "5910", "name": "Résultat de l'exercice",       "type": "equity"},

    # ─── 6000–6999 : Produits ────────────────────────────────────────────────
    {"number": "6000", "name": "Ventes de marchandises",       "type": "revenue"},
    {"number": "6100", "name": "Prestations de services",      "type": "revenue"},
    {"number": "6200", "name": "Produits financiers",          "type": "revenue"},
    {"number": "6300", "name": "Produits exceptionnels",       "type": "revenue"},

    # ─── 7000–7999 : Charges de marchandises ─────────────────────────────────
    {"number": "7000", "name": "Achats de marchandises",       "type": "expense"},
    {"number": "7010", "name": "Variation de stock",           "type": "expense"},

    # ─── 8000–8999 : Charges de personnel ────────────────────────────────────
    {"number": "8000", "name": "Salaires bruts",               "type": "expense"},
    {"number": "8010", "name": "Charges sociales (AVS/AI/APG)","type": "expense"},
    {"number": "8020", "name": "Prévoyance professionnelle LPP","type": "expense"},
    {"number": "8030", "name": "Assurance accidents LAA",      "type": "expense"},

    # ─── 9000–9999 : Autres charges ──────────────────────────────────────────
    {"number": "9000", "name": "Loyer",                        "type": "expense"},
    {"number": "9100", "name": "Frais de bureau",              "type": "expense"},
    {"number": "9200", "name": "Frais informatiques",          "type": "expense"},
    {"number": "9300", "name": "Frais de déplacement",         "type": "expense"},
    {"number": "9400", "name": "Publicité et marketing",       "type": "expense"},
    {"number": "9500", "name": "Frais financiers",             "type": "expense"},
    {"number": "9600", "name": "Amortissements",               "type": "expense"},
    {"number": "9700", "name": "Impôts et taxes",              "type": "expense"},
    {"number": "9900", "name": "Charges exceptionnelles",      "type": "expense"},
]
