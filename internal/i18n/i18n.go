// Package i18n rend les textes que LedgerAlps adresse à l'utilisateur dans sa
// langue : refus du serveur, factures PDF, exports CSV, attestation.
//
// # Pourquoi le français sert de clé
//
// Les messages existent déjà, écrits en français, et ils servent à trois
// choses à la fois : ils s'affichent à l'écran, ils partent au journal du
// serveur, et une trentaine de tests les comparent au caractère près.
// Introduire des clés symboliques (`err.tvaAbsente`) aurait demandé de
// réécrire les trois d'un coup.
//
// La clé est donc la phrase française elle-même. Trois conséquences, toutes
// souhaitables :
//
//   - Le repli est le comportement d'aujourd'hui. Une phrase sans traduction
//     ressort en français, ce que l'utilisateur voyait déjà — jamais une clé
//     nue du genre « err.tvaAbsente », qui n'aide personne.
//   - Le code reste lisible. `i18n.T(l, "aucune facture sélectionnée")` se lit
//     sans ouvrir le catalogue.
//   - Les journaux et les tests ne bougent pas : ils lisent le français, qui
//     reste la valeur produite par défaut.
//
// # Ce que ce paquet ne fait PAS
//
// Il ne devine pas la langue. Elle vient de la requête (voir Langue), donc du
// sélecteur de l'interface. Un document produit hors requête — l'attestation
// écrite au démarrage — sort en français, et c'est délibéré : personne ne l'a
// demandée dans une autre langue.
package i18n

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

// Lang est une des quatre langues de l'interface.
type Lang string

const (
	FR Lang = "fr"
	DE Lang = "de"
	IT Lang = "it"
	EN Lang = "en"
)

// Défaut est la langue du produit quand rien n'est demandé.
const Défaut = FR

// Valide ramène une valeur quelconque à une langue connue.
//
// Elle accepte les étiquettes régionales — « de-CH », « en-GB » — parce que
// c'est ce qu'un navigateur envoie, et que refuser « de-CH » pour n'accepter
// que « de » afficherait du français à un Suisse alémanique.
func Valide(s string) Lang {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_"); i > 0 {
		s = s[:i]
	}
	switch Lang(s) {
	case FR, DE, IT, EN:
		return Lang(s)
	}
	return Défaut
}

// Langue lit la langue demandée par la requête.
//
// # Pourquoi l'en-tête et non un paramètre d'URL
//
// `Accept-Language` s'applique à TOUTES les routes sans que chacune ait à
// penser à transmettre `?lang=`. Le client le pose une fois, dans
// l'intercepteur axios, à partir du sélecteur. Une route ajoutée demain est
// couverte sans rien faire — c'est exactement le motif qui a laissé passer des
// écrans non traduits côté frontend.
//
// Le paramètre `lang` reste accepté : la route des avis de conformité l'utilise
// depuis toujours, et un téléchargement déclenché par une navigation directe ne
// peut pas poser d'en-tête.
func Langue(c *gin.Context) Lang {
	if c == nil {
		return Défaut
	}
	if q := c.Query("lang"); q != "" {
		return Valide(q)
	}
	return Valide(première(c.GetHeader("Accept-Language")))
}

// première rend la première étiquette d'un en-tête Accept-Language.
//
// « de-CH,de;q=0.9,fr;q=0.8 » vaut « de-CH ». On ne trie pas par facteur de
// qualité : le client est notre propre interface, qui n'envoie qu'une valeur.
// Un navigateur qui en enverrait plusieurs verra sa préférence principale
// respectée, ce qui suffit.
func première(entête string) string {
	if entête == "" {
		return ""
	}
	if i := strings.IndexByte(entête, ','); i >= 0 {
		entête = entête[:i]
	}
	if i := strings.IndexByte(entête, ';'); i >= 0 {
		entête = entête[:i]
	}
	return entête
}

// T traduit une phrase française dans la langue demandée.
//
// Les arguments suivent les verbes de `fmt` : la phrase française porte les
// mêmes que sa traduction, et un écart se voit tout de suite à l'écran plutôt
// que de disparaître silencieusement.
func T(l Lang, fr string, args ...any) string {
	texte := fr
	if l != FR {
		if trads, ok := catalogue[fr]; ok {
			if t := trads[l]; t != "" {
				texte = t
			}
		}
	}
	if len(args) == 0 {
		return texte
	}
	return fmt.Sprintf(texte, args...)
}

// Traduit dit si une phrase est au catalogue.
//
// Sert au test qui vérifie la couverture : sans lui, il faudrait exporter la
// table entière et se fier à la discipline de qui la lit.
func Traduit(fr string) bool {
	_, ok := catalogue[fr]
	return ok
}

// Phrases rend toutes les phrases du catalogue. Réservé aux tests.
func Phrases() []string {
	out := make([]string, 0, len(catalogue))
	for fr := range catalogue {
		out = append(out, fr)
	}
	return out
}

// Traductions rend les quatre langues d'une phrase. Réservé aux tests.
func Traductions(fr string) map[Lang]string {
	return catalogue[fr]
}
