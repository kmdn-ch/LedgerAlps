package accounting

// Le masquage des données personnelles dans la piste d'audit (nLPD art. 6).
//
// Ces tests tiennent les DEUX bords. Masquer trop peu laisse le nom et
// l'adresse privée d'un indépendant en clair dans une table conservée dix ans ;
// masquer trop rend la piste illisible et pousse à la contourner. Une règle qui
// ne serait vérifiée que d'un côté dériverait vers l'autre.

import "testing"

func TestLesDonneesPersonnellesSontMasquees(t *testing.T) {
	cas := []struct {
		nom     string
		entree  string
		attendu string
	}{
		{"clé exacte", `{"iban":"CH9300762011623852957"}`, `{"iban":"[MASKED]"}`},
		{"courriel", `{"email":"paul@exemple.ch"}`, `{"email":"[MASKED]"}`},
		{"nom", `{"name":"Paul Dupont"}`, `{"name":"[MASKED]"}`},

		// Les variantes composées : c'est ce que la règle exacte laissait fuir.
		// Chez un indépendant, la raison sociale EST son nom, et l'adresse de
		// l'entreprise son domicile.
		{"préfixée", `{"company_name":"Dupont Menuiserie"}`, `{"company_name":"[MASKED]"}`},
		{"préfixée aussi", `{"legal_name":"Dupont SA"}`, `{"legal_name":"[MASKED]"}`},
		{"fournisseur", `{"supplier_name":"Acme SA"}`, `{"supplier_name":"[MASKED]"}`},
		{"suffixée", `{"address_street":"Rue du Lac 3"}`, `{"address_street":"[MASKED]"}`},
		{"suffixée aussi", `{"address_city":"Lausanne"}`, `{"address_city":"[MASKED]"}`},
		{"QR-IBAN", `{"qr_iban":"CH44 3199 9123 0008 8901 2"}`, `{"qr_iban":"[MASKED]"}`},

		// Espace après les deux-points : un encodeur peut le produire.
		{"avec espace", `{"iban": "CH93007620116"}`, `{"iban": "[MASKED]"}`},

		// Plusieurs champs dans le même objet.
		{
			"objet complet",
			`{"company_name":"Dupont","status":"draft","iban":"CH93","total":1500}`,
			`{"company_name":"[MASKED]","status":"draft","iban":"[MASKED]","total":1500}`,
		},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			if got := maskPersonalData(c.entree); got != c.attendu {
				t.Errorf("maskPersonalData(%s)\n  = %s\n  attendu %s", c.entree, got, c.attendu)
			}
		})
	}
}

// Ce qui n'est PAS une donnée personnelle doit rester lisible.
//
// Une piste d'audit où tout est masqué ne sert à rien : c'est le statut, le
// montant et la date qui font qu'on comprend ce qui s'est passé. Le test nomme
// les voisins dangereux — ceux qui contiennent presque un terme sensible.
func TestLeMasquageNeDeborde(t *testing.T) {
	intacts := []string{
		`{"number":"FA-2026-0001"}`,      // contient « n », pas « name »
		`{"document_type":"invoice"}`,    //
		`{"status":"booked"}`,            //
		`{"reference":"REF-1"}`,          //
		`{"currency":"CHF"}`,             //
		`{"named_range":"x"}`,            // « named » n'est pas « name »
		`{"nameless":"x"}`,               // idem
		`{"total":1500}`,                 // valeur non textuelle
		`{"champs_modifies":["status"]}`, // la liste des changements reste lisible
	}
	for _, j := range intacts {
		if got := maskPersonalData(j); got != j {
			t.Errorf("maskPersonalData(%s) = %s — masqué à tort", j, got)
		}
	}
}
