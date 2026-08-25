package accounting

// L'audit différentiel : ce que la piste doit dire d'une modification.

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestChampsModifiesNommeCeQuiABouge(t *testing.T) {
	cas := []struct {
		nom     string
		avant   map[string]any
		apres   map[string]any
		attendu []string
	}{
		{
			nom:     "un seul champ",
			avant:   map[string]any{"status": "draft", "total": 1000.0},
			apres:   map[string]any{"status": "sent", "total": 1000.0},
			attendu: []string{"status"},
		},
		{
			nom:     "rien n'a bougé",
			avant:   map[string]any{"status": "sent", "total": 1000.0},
			apres:   map[string]any{"status": "sent", "total": 1000.0},
			attendu: nil,
		},
		{
			nom:     "un champ apparaît",
			avant:   map[string]any{"status": "booked"},
			apres:   map[string]any{"status": "booked", "journal_entry_id": "e1"},
			attendu: []string{"journal_entry_id"},
		},
		{
			nom:     "un champ disparaît",
			avant:   map[string]any{"status": "booked", "vat_number": "CHE-1"},
			apres:   map[string]any{"status": "booked"},
			attendu: []string{"vat_number"},
		},
		{
			nom:     "plusieurs, rendus triés",
			avant:   map[string]any{"iban": "CH11", "city": "Nyon", "total": 10.0},
			apres:   map[string]any{"iban": "CH93", "city": "Morges", "total": 10.0},
			attendu: []string{"city", "iban"},
		},
		{
			// Une création n'a rien remplacé : la liste n'a pas de sens, et une
			// liste de « tous les champs » à chaque création serait du bruit
			// dans une table conservée dix ans.
			nom:     "création : pas de liste",
			avant:   nil,
			apres:   map[string]any{"status": "draft"},
			attendu: nil,
		},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			got := champsModifies(c.avant, c.apres)
			if !reflect.DeepEqual(got, c.attendu) {
				t.Errorf("champsModifies = %v, attendu %v", got, c.attendu)
			}
		})
	}
}

// L'ordre des clés d'une map Go est délibérément aléatoire. Une liste non
// triée produirait un JSON différent d'une exécution à l'autre pour un MÊME
// événement — donc une empreinte différente, et une chaîne que sa propre
// vérification déclarerait rompue.
func TestLaListeEstStableEntreDeuxAppels(t *testing.T) {
	avant := map[string]any{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6}
	apres := map[string]any{"a": 9, "b": 9, "c": 9, "d": 9, "e": 9, "f": 9}

	premier, _ := json.Marshal(champsModifies(avant, apres))
	for i := 0; i < 50; i++ {
		suivant, _ := json.Marshal(champsModifies(avant, apres))
		if string(suivant) != string(premier) {
			t.Fatalf("la liste varie entre deux appels : %s puis %s", premier, suivant)
		}
	}
}

// LE test qui porte la fonctionnalité : un IBAN masqué des deux côtés doit
// quand même se signaler comme ayant changé.
//
// C'est le cas qui motive tout le reste. `maskPersonalData` remplace la valeur
// de `iban` par « [MASKED] » : avant ET après deviennent identiques, et la
// modification disparaît de la piste — précisément celle dont on veut la trace,
// puisque cet IBAN est le compte qui reçoit les virements de tous les clients.
func TestUnIBANModifieSeVoitMalgreLeMasquage(t *testing.T) {
	avant := map[string]any{"iban": "CH1100000000000000000", "currency": "CHF"}
	apres := map[string]any{"iban": "CH9300762011623852957", "currency": "CHF"}

	etat := avecChampsModifies(Modification(avant, apres))

	brut, err := json.Marshal(etat)
	if err != nil {
		t.Fatalf("encodage: %v", err)
	}
	masque := maskPersonalData(string(brut))

	// Les deux IBAN ont bien disparu…
	for _, secret := range []string{"CH1100000000000000000", "CH9300762011623852957"} {
		if contient(masque, secret) {
			t.Errorf("l'IBAN %s subsiste en clair dans la piste : %s", secret, masque)
		}
	}
	// … mais le fait qu'il ait changé, non.
	if !contient(masque, `"champs_modifies":["iban"]`) {
		t.Errorf("le changement d'IBAN ne se voit pas dans la piste : %s", masque)
	}
	// Et un champ inchangé ne doit pas être signalé.
	if contient(masque, "currency\",\"") && contient(masque, `"currency"]`) {
		t.Errorf("un champ inchangé est signalé comme modifié : %s", masque)
	}
}

// Une création ne porte pas de liste : il n'y a rien à comparer.
func TestUneCreationNePorteAucuneListe(t *testing.T) {
	etat := avecChampsModifies(Creation(map[string]any{"status": "draft"}))
	if _, present := etat[CleChampsModifies]; present {
		t.Error("une création porte une liste de champs modifiés")
	}
}

// L'état de l'appelant ne doit pas être modifié en place : il s'en sert souvent
// pour construire la réponse HTTP juste après.
func TestLEtatDeLAppelantNEstPasModifie(t *testing.T) {
	apres := map[string]any{"status": "sent"}
	_ = avecChampsModifies(Modification(map[string]any{"status": "draft"}, apres))
	if _, pollue := apres[CleChampsModifies]; pollue {
		t.Error("la map de l'appelant a été enrichie en place")
	}
}

func contient(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
