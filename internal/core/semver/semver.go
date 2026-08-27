// Package semver compare des versions « X.Y.Z » telles que ce produit les
// écrit.
//
// Deux copies de cette fonction vivaient dans le dépôt — `internal/core/
// compliance` pour décider si un avis de conformité est déjà corrigé, et
// `internal/services/updatecheck` pour comparer la version installée à la
// dernière publiée. Elles avaient déjà DIVERGÉ sur deux points, sans que rien
// ne le signale :
//
//   - au-delà de trois segments, l'une tronquait en silence (« 1.2.3.4 » lu
//     comme « 1.2.3 »), l'autre refusait ;
//   - un segment négatif était accepté d'un côté, refusé de l'autre.
//
// C'est la politique STRICTE qui est retenue ici. Le choix n'est pas
// esthétique : la fonction sert à décider si un avis de sécurité s'applique
// encore, et sur ce chemin, refuser ce qu'on ne comprend pas fait AVERTIR
// l'utilisateur, tandis que deviner peut faire taire l'avertissement.
package semver

import (
	"strconv"
	"strings"
)

// Parse lit « v1.2.3 », « 1.2.3-rc1 » ou « 1.2 » et rend les trois nombres.
//
// Le « v » de tête est ignoré, un suffixe de pré-version (« -rc1 », « +build »)
// est écarté : 1.4.0-rc1 se compare comme 1.4.0. Une chaîne à plus de trois
// segments, un segment non numérique ou négatif rendent false — jamais une
// valeur tronquée qui aurait l'air valide.
func Parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return out, false
	}
	parts := strings.Split(v, ".")
	if len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AuMoins dit si current >= target.
//
// Les deux cas d'illisibilité sont dissymétriques, et c'est voulu :
//
//   - target illisible → false. L'appelant en déduit « pas encore corrigé »,
//     donc il AVERTIT. On préfère un avertissement de trop à un avis tu.
//   - current illisible → true. C'est le cas d'un binaire de développement
//     (`version = "dev"`), qu'on ne veut pas harceler pour des avis qu'il a
//     déjà corrigés dans son arbre de travail.
func AuMoins(current, target string) bool {
	cur, curOK := Parse(current)
	tgt, tgtOK := Parse(target)
	if !tgtOK {
		return false
	}
	if !curOK {
		return true
	}
	for i := 0; i < 3; i++ {
		if cur[i] != tgt[i] {
			return cur[i] > tgt[i]
		}
	}
	return true
}
