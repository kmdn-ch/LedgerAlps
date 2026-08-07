// Ce que le rôle courant autorise, à l'écran.
//
// # Pourquoi un écran doit le savoir alors que le serveur décide
//
// Le serveur refuse déjà toute écriture à un compte en lecture seule, et cela
// ne changera pas : c'est lui qui garde les livres. Mais un bouton qu'on peut
// cliquer promet une action. Le laisser en place pour répondre 403 au clic,
// c'est faire remplir un formulaire entier avant d'annoncer qu'il ne servira à
// rien — et laisser croire que la barrière est cosmétique alors qu'elle ne
// l'est pas.
//
// Une fiduciaire à qui l'on ouvre les livres doit voir un produit qui n'offre
// pas ce qu'elle n'a pas le droit de faire. L'écran et le serveur disent alors
// la même chose, et le premier explique ce que le second appliquerait.
//
// # Ce que ces crochets ne sont pas
//
// Ils ne sont PAS une mesure de sécurité. Rien de ce qui vit dans un navigateur
// n'en est une : le code est lisible, modifiable, et une requête se forge sans
// lui. La sécurité est dans `authz.DenyWritesWithoutPermission` et dans les
// permissions déclarées par route. Ceci en est le reflet fidèle, rien de plus.
//
// Le miroir doit donc rester fidèle : quand une permission change côté serveur,
// elle change ici. Deux modèles qui divergent produisent soit un bouton qui
// échoue, soit une fonction cachée à qui y avait droit.

import { useAuthStore } from '@/store/auth'
import type { UserRole } from '@/types'

/** Le rôle courant, ou null tant que la session n'est pas rétablie. */
export function useRole(): UserRole | null {
  return useAuthStore(s => s.role)
}

// enLectureSeule dit si ce rôle ne peut rien modifier.
//
// Le refus est le défaut : un rôle inconnu, une session pas encore rétablie,
// une valeur venue d'une version future sont traités comme lecture seule. Se
// tromper dans ce sens masque un bouton à quelqu'un qui y avait droit — il
// recharge la page. Se tromper dans l'autre offre une action qui sera refusée,
// après saisie.
function enLectureSeule(role: UserRole | null): boolean {
  return role !== 'admin' && role !== 'accountant'
}

/**
 * Ce compte peut-il modifier quoi que ce soit — écritures, factures, contacts,
 * dépôts de fichiers ?
 *
 * C'est le pendant exact de `authz.RoleViewer` côté serveur : le filtre global
 * y refuse toute méthode autre que GET, HEAD et OPTIONS.
 */
export function useCanWrite(): boolean {
  return !enLectureSeule(useAuthStore(s => s.role))
}

/** L'inverse, quand la phrase se lit mieux ainsi. */
export function useIsReadOnly(): boolean {
  return enLectureSeule(useAuthStore(s => s.role))
}

/**
 * Administration du logiciel : comptes utilisateurs, sécurité, sauvegardes.
 *
 * Le comptable en est exclu — il tient les livres, il ne détient pas les clés
 * de l'installation.
 */
export function useCanAdmin(): boolean {
  return useAuthStore(s => s.role) === 'admin'
}

/**
 * La phrase à afficher là où une action est retirée.
 *
 * Un bouton qui disparaît sans explication passe pour une fonction manquante.
 * Dire le rôle transforme une absence en règle — et c'est vérifiable par la
 * personne concernée, qui sait alors quoi demander à qui.
 */
export const RAISON_LECTURE_SEULE =
  'Votre compte est en lecture seule : vous pouvez tout consulter et tout ' +
  'exporter, mais rien modifier.'
