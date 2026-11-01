"""Tests de la veille légale : ce qu'elle regarde, sans réseau.

    python -m unittest discover -s scripts -p "test_*.py"

Bibliothèque standard seulement, comme le script lui-même : une veille qui casse
parce qu'un paquet tiers a publié une mauvaise version est pire que pas de veille.
"""
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import compliance_watch as cw  # noqa: E402


class EmpreinteFedlex(unittest.TestCase):
    # Les dates du Code des obligations relevées sur l'API de Fedlex : une consolidation
    # FUTURE (2027-07-01) existe déjà à côté de celles de 2026.
    CO = ["2027-07-01", "2026-10-01", "2026-01-01", "2025-10-01", "2025-07-08"]

    def test_une_consolidation_intermediaire_change_l_empreinte(self):
        """Le défaut visé : avec la seule date maximale, une consolidation insérée
        entre deux autres ne changeait rien de ce que la veille regardait."""
        avant = cw.fedlex_fingerprint(self.CO)
        apres = cw.fedlex_fingerprint(self.CO + ["2026-11-15"])
        self.assertEqual(max(self.CO), "2027-07-01")  # le maximum ne bouge pas…
        self.assertNotEqual(avant, apres)             # …l'empreinte, si

    def test_une_consolidation_retiree_ou_deplacee_change_l_empreinte(self):
        avant = cw.fedlex_fingerprint(self.CO)
        self.assertNotEqual(avant, cw.fedlex_fingerprint(self.CO[:-1]))
        deplacee = self.CO[:-1] + ["2025-07-09"]
        # même nombre de dates, même maximum : seul le condensé le voit
        self.assertNotEqual(avant, cw.fedlex_fingerprint(deplacee))

    def test_l_ordre_et_les_doublons_ne_comptent_pas(self):
        self.assertEqual(
            cw.fedlex_fingerprint(self.CO),
            cw.fedlex_fingerprint(list(reversed(self.CO)) + self.CO[:2]),
        )

    def test_le_format_se_lit_dans_un_ticket(self):
        empreinte = cw.fedlex_fingerprint(self.CO)
        self.assertTrue(empreinte.startswith("2027-07-01 · 5 consolidations · "), empreinte)

    def test_aucune_consolidation_est_une_panne_pas_un_changement(self):
        with self.assertRaises(cw.CheckError):
            cw.fedlex_fingerprint([], "220")


if __name__ == "__main__":
    unittest.main()
