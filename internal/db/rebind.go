package db

import (
	"strconv"
	"strings"
)

// Rebind converts ? placeholders to $1, $2, ... for PostgreSQL.
// Returns the query unchanged for SQLite.
//
// # Ce qui n'est PAS un paramètre
//
// La substitution ignore ce qui n'est pas du code SQL : les chaînes littérales
// ('…'), les identifiants entre guillemets ("…"), et les commentaires (-- et
// /* … */). Un point d'interrogation y est un CARACTÈRE, pas un emplacement.
//
// La forme précédente remplaçait tous les `?` sans distinction. Aucune requête
// du dépôt n'en contient dans un littéral aujourd'hui — le défaut était donc
// latent — mais il aurait suffi d'un message d'erreur stocké en base, d'un
// LIKE '%?%' ou d'un commentaire SQL portant une question pour que la requête
// parte à PostgreSQL avec un `$n` de trop et un décalage de TOUS les
// paramètres suivants. L'erreur ne se serait pas vue à la compilation, et sur
// une requête d'écriture elle aurait écrit la bonne valeur dans la mauvaise
// colonne.
//
// # Limite connue
//
// Les chaînes à dollar de PostgreSQL ($$…$$, $tag$…$tag$) ne sont pas
// reconnues : le dépôt n'en emploie aucune, et les traiter demanderait de
// distinguer un délimiteur d'un `$n` déjà écrit. Si l'on en introduit un jour,
// c'est ici qu'il faudra revenir.
func Rebind(query string, usePostgres bool) string {
	if !usePostgres {
		return query
	}

	idx := 0
	var b strings.Builder
	b.Grow(len(query) + 16)

	for i := 0; i < len(query); {
		switch {
		// ── Chaîne littérale : '…', où '' est une apostrophe échappée ────────
		case query[i] == '\'':
			j := i + 1
			for j < len(query) {
				if query[j] != '\'' {
					j++
					continue
				}
				// Deux apostrophes de suite = une apostrophe DANS la chaîne.
				if j+1 < len(query) && query[j+1] == '\'' {
					j += 2
					continue
				}
				j++ // l'apostrophe fermante
				break
			}
			b.WriteString(query[i:j])
			i = j

		// ── Identifiant entre guillemets : "…", où "" est un guillemet ───────
		case query[i] == '"':
			j := i + 1
			for j < len(query) {
				if query[j] != '"' {
					j++
					continue
				}
				if j+1 < len(query) && query[j+1] == '"' {
					j += 2
					continue
				}
				j++
				break
			}
			b.WriteString(query[i:j])
			i = j

		// ── Commentaire de ligne : -- jusqu'au saut de ligne ─────────────────
		case query[i] == '-' && i+1 < len(query) && query[i+1] == '-':
			j := i + 2
			for j < len(query) && query[j] != '\n' {
				j++
			}
			b.WriteString(query[i:j])
			i = j

		// ── Commentaire de bloc : /* … */, IMBRICABLE ────────────────────────
		//
		// PostgreSQL imbrique les commentaires de bloc, contrairement au SQL
		// standard. S'arrêter au premier « */ » sortirait trop tôt d'un
		// commentaire imbriqué et traiterait la suite comme du code.
		case query[i] == '/' && i+1 < len(query) && query[i+1] == '*':
			profondeur := 1
			j := i + 2
			for j < len(query) && profondeur > 0 {
				switch {
				case query[j] == '/' && j+1 < len(query) && query[j+1] == '*':
					profondeur++
					j += 2
				case query[j] == '*' && j+1 < len(query) && query[j+1] == '/':
					profondeur--
					j += 2
				default:
					j++
				}
			}
			b.WriteString(query[i:j])
			i = j

		// ── Un vrai emplacement de paramètre ─────────────────────────────────
		case query[i] == '?':
			idx++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(idx))
			i++

		default:
			b.WriteByte(query[i])
			i++
		}
	}
	return b.String()
}
