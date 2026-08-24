package accounting

// L'audit différentiel : ce qu'une action a REMPLACÉ, pas seulement ce qu'elle
// a laissé.
//
// # Ce qui manquait
//
// Le journal capturait l'état APRÈS chaque action, jamais celui d'avant. On
// savait donc qu'une facture valait 1500.- après modification, sans pouvoir
// dire qu'elle valait 1000.- avant. Pour la fiche entreprise, c'était pire :
// « qui a changé l'IBAN » — le compte qui reçoit les virements de tous les
// clients — se réduisait à un drapeau `iban_modifie: true` posé à la main dans
// un seul gestionnaire, que rien n'obligeait les autres à poser.
//
// # Pourquoi un type nommé plutôt que deux paramètres
//
// `avant` et `apres` sont tous deux des `map[string]any`. Passés côte à côte,
// rien — ni le compilateur, ni la relecture — n'empêche de les intervertir. Or
// une piste d'audit inversée est pire qu'une piste absente : elle affirme le
// contraire de ce qui s'est produit, et elle le fait avec l'autorité d'une
// chaîne d'empreintes valide. Des champs nommés rendent la faute impossible.
//
// # Le masquage, et ce qu'il coûte
//
// `maskPersonalData` remplace la valeur des champs personnels (nLPD art. 6) :
// `email`, `name`, `address`, `phone`, `iban`, `qr_iban`, `password_hash`. Un
// IBAN modifié donne donc `[MASKED]` avant ET `[MASKED]` après — le
// changement devient invisible, précisément dans le cas qui motive la
// fonctionnalité.
//
// D'où `ChampsModifies` : la liste des champs qui ont bougé est calculée sur
// les valeurs BRUTES, avant masquage, et voyage avec le maillon. On sait donc
// QUE l'IBAN a changé, et qui l'a changé, sans stocker aucun des deux IBAN.
// C'est plus utile que deux valeurs masquées, et cela conserve MOINS de données
// personnelles — la minimisation de la nLPD est ici alignée avec l'utilité, pas
// en tension avec elle.

import (
	"encoding/json"
	"sort"
)

// CleChampsModifies est la clé sous laquelle la liste des champs modifiés est
// jointe à l'état « après ».
//
// Elle vit dans l'état plutôt que dans une colonne à part parce qu'elle doit
// entrer dans l'empreinte : une liste des changements que l'on pourrait
// réécrire sans casser la chaîne ne prouverait rien.
const CleChampsModifies = "champs_modifies"

// Transition décrit ce qu'une action a remplacé et ce qu'elle a laissé.
//
// `Avant` nul signifie « rien ne précédait » — une création. Ce n'est pas la
// même chose qu'un état vide, et la colonne `before_state` est justement
// nullable pour tenir la distinction : une création écrit NULL, une
// modification depuis un état vide écrirait `{}`.
type Transition struct {
	Avant map[string]any
	Apres map[string]any
}

// Creation décrit une action qui n'a rien remplacé.
func Creation(apres map[string]any) Transition {
	return Transition{Apres: apres}
}

// Modification décrit une action qui a remplacé un état par un autre.
func Modification(avant, apres map[string]any) Transition {
	return Transition{Avant: avant, Apres: apres}
}

// Suppression décrit une action dont il ne reste que ce qui précédait.
//
// C'est le seul cas où la trace survit à la pièce : après elle, il n'existe
// plus rien d'autre. L'état antérieur y est donc la seule information qui
// subsiste, et la raison la plus forte de le conserver.
func Suppression(avant map[string]any) Transition {
	return Transition{Avant: avant, Apres: map[string]any{}}
}

// champsModifies rend, triés, les noms des champs dont la valeur diffère.
//
// Le tri n'est pas cosmétique : l'ordre des clés d'une map Go est délibérément
// aléatoire, et une liste non triée produirait un JSON différent à chaque
// exécution — donc une empreinte différente pour un même événement, et une
// chaîne que sa propre vérification déclarerait rompue.
//
// La comparaison passe par l'encodage JSON de chaque valeur : elle traite ainsi
// les structures imbriquées et les types numériques sans que l'appelant ait à
// s'en soucier, et elle compare exactement ce qui sera stocké.
func champsModifies(avant, apres map[string]any) []string {
	if avant == nil {
		return nil
	}
	vus := make(map[string]struct{}, len(avant)+len(apres))
	for k := range avant {
		vus[k] = struct{}{}
	}
	for k := range apres {
		vus[k] = struct{}{}
	}

	var modifies []string
	for k := range vus {
		// La clé technique ne se compare pas à elle-même : elle est produite
		// ici, pas fournie par l'appelant.
		if k == CleChampsModifies {
			continue
		}
		a, okA := avant[k]
		b, okB := apres[k]
		// Un champ absent d'un seul côté a changé : apparu ou disparu.
		if okA != okB || !memeValeur(a, b) {
			modifies = append(modifies, k)
		}
	}
	sort.Strings(modifies)
	return modifies
}

// memeValeur compare deux valeurs par leur forme JSON.
//
// En cas d'échec d'encodage — une valeur non sérialisable —, on répond
// « différentes ». Signaler un changement qui n'a pas eu lieu fait relire une
// ligne pour rien ; taire un changement réel laisse passer ce que la piste
// existe pour montrer. Entre les deux, le doute penche du côté qui alerte.
func memeValeur(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ja) == string(jb)
}

// avecChampsModifies rend une COPIE de l'état « après » portant la liste des
// champs qui ont bougé.
//
// Une copie, parce que la map appartient à l'appelant : la modifier en place
// ferait apparaître une clé technique dans la réponse HTTP qu'il s'apprête à
// écrire, ou dans la structure qu'il réutilise pour autre chose.
func avecChampsModifies(t Transition) map[string]any {
	modifies := champsModifies(t.Avant, t.Apres)
	if len(modifies) == 0 {
		// Rien n'a bougé, ou c'est une création : ne rien ajouter. Une liste
		// vide dans chaque maillon de création serait du bruit dans une table
		// que l'on conserve dix ans (CO art. 958f).
		return t.Apres
	}
	copie := make(map[string]any, len(t.Apres)+1)
	for k, v := range t.Apres {
		copie[k] = v
	}
	copie[CleChampsModifies] = modifies
	return copie
}
