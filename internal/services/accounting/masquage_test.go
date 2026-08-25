package accounting

// Le masquage des données personnelles dans la piste d'audit (nLPD art. 6).
//
// Ces tests tiennent les DEUX bords. Masquer trop peu laisse le nom et
// l'adresse privée d'un indépendant en clair dans une table conservée dix ans ;
// masquer trop rend la piste illisible et pousse à la contourner. Une règle qui
// ne serait vérifiée que d'un côté dériverait vers l'autre.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

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

// Une raison sociale contenant un guillemet ne casse plus le masquage.
//
// C'est le défaut M-1 du second audit. Le masquage textuel terminait la valeur
// sur `[^"]*"`, donc au premier guillemet ÉCHAPPÉ produit par json.Marshal :
// l'état devenait du JSON invalide ET un fragment du nom de l'indépendant
// restait en clair dans une table conservée dix ans. « Au "Bon" Vin Sàrl » est
// une raison sociale suisse banale — il ne faut pas d'attaquant pour l'atteindre.
func TestUnGuillemetDansLaRaisonSocialeNeCassePasLeMasquage(t *testing.T) {
	etat := map[string]any{
		"company_name": `Au "Bon" Vin Sàrl`,
		"iban":         "CH9300762011623852957",
		"legal_form":   "sarl",
	}

	brut, err := json.Marshal(masquerEtat(etat))
	if err != nil {
		t.Fatalf("encodage: %v", err)
	}

	// 1. Le résultat doit rester du JSON lisible : c'est ce que l'écran d'audit
	//    parse pour afficher « champs modifiés ».
	var relu map[string]any
	if err := json.Unmarshal(brut, &relu); err != nil {
		t.Fatalf("l'état masqué n'est pas du JSON valide : %v\n  %s", err, brut)
	}

	// 2. Aucun fragment du nom ne doit subsister.
	for _, fragment := range []string{"Bon", "Vin", "Sàrl"} {
		if strings.Contains(string(brut), fragment) {
			t.Errorf("le fragment %q reste en clair : %s", fragment, brut)
		}
	}
	if relu["company_name"] != "[MASKED]" {
		t.Errorf("company_name = %v, attendu [MASKED]", relu["company_name"])
	}
	if relu["iban"] != "[MASKED]" {
		t.Errorf("iban = %v, attendu [MASKED]", relu["iban"])
	}
	// 3. Ce qui n'est pas personnel traverse intact.
	if relu["legal_form"] != "sarl" {
		t.Errorf("legal_form = %v, attendu « sarl » — le masquage déborde", relu["legal_form"])
	}
}

// Le masquage structurel couvre ce que le motif textuel laissait passer.
func TestLeMasquageCouvreLesValeursNonTextuellesEtLaCasse(t *testing.T) {
	cas := []struct {
		nom     string
		etat    map[string]any
		masques []string
		intacts []string
	}{
		{
			nom:     "valeur numérique sur une clé sensible",
			etat:    map[string]any{"phone": 41791234567, "invoice_number": 42},
			masques: []string{"phone"},
			intacts: []string{"invoice_number"},
		},
		{
			nom:     "clés en casse différente",
			etat:    map[string]any{"Email": "a@b.ch", "IBAN": "CH93", "customerName": "Dupont"},
			masques: []string{"Email", "IBAN", "customerName"},
		},
		{
			nom:     "liste de valeurs sur une clé sensible",
			etat:    map[string]any{"name": []any{"Jean", "Dupont"}, "tags": []any{"a", "b"}},
			masques: []string{"name"},
			intacts: []string{"tags"},
		},
		{
			nom: "objet imbriqué",
			etat: map[string]any{
				"contact": map[string]any{"email": "a@b.ch", "role": "client"},
			},
		},
		{
			nom:     "champs_modifies traverse — c'est la clé de l'audit différentiel",
			etat:    map[string]any{CleChampsModifies: []any{"iban", "company_name"}},
			intacts: []string{CleChampsModifies},
		},
	}

	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			out := masquerEtat(c.etat)
			for _, k := range c.masques {
				if out[k] != "[MASKED]" {
					t.Errorf("%s = %#v, attendu [MASKED]", k, out[k])
				}
			}
			for _, k := range c.intacts {
				if fmt.Sprint(out[k]) != fmt.Sprint(c.etat[k]) {
					t.Errorf("%s a été modifié : %#v (attendu %#v)", k, out[k], c.etat[k])
				}
			}
		})
	}

	// L'objet imbriqué se vérifie à part : la valeur est une carte.
	out := masquerEtat(map[string]any{
		"contact": map[string]any{"email": "a@b.ch", "role": "client"},
	})
	sous, ok := out["contact"].(map[string]any)
	if !ok {
		t.Fatalf("contact n'est plus un objet : %#v", out["contact"])
	}
	if sous["email"] != "[MASKED]" {
		t.Errorf("l'e-mail imbriqué n'est pas masqué : %#v", sous["email"])
	}
	if sous["role"] != "client" {
		t.Errorf("le rôle imbriqué a été modifié : %#v", sous["role"])
	}
}

// L'état de l'appelant n'est jamais modifié par le masquage.
func TestLeMasquageNeMutePasSonEntree(t *testing.T) {
	etat := map[string]any{"company_name": "Dupont", "legal_form": "sarl"}
	_ = masquerEtat(etat)
	if etat["company_name"] != "Dupont" {
		t.Errorf("l'entrée a été mutée : %#v", etat["company_name"])
	}
}

// Le découpage des clés en mots, sur les formes réellement rencontrées.
func TestLeDecoupageDesClesEnMots(t *testing.T) {
	cas := map[string]bool{
		// Sensibles
		"iban": true, "IBAN": true, "qr_iban": true, "QRIban": true,
		"company_name": true, "companyName": true, "CompanyName": true,
		"address_street": true, "Email": true, "password_hash": true,
		"IBANNumber": true, "supplier_email": true,
		// Ne le sont pas — c'est la moitié qui compte autant
		"number": false, "document_type": false, "invoice_number": false,
		"champs_modifies": false, "status": false, "created_at": false,
		"amount": false, "vat_status": false,
	}
	for cle, attendu := range cas {
		if got := estChampSensible(cle); got != attendu {
			t.Errorf("estChampSensible(%q) = %v, attendu %v (mots: %v)",
				cle, got, attendu, motsDeLaCle(cle))
		}
	}
}
