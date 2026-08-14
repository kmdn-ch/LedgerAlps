// Package authz décide qui a le droit de faire quoi.
//
// Le cas central en Suisse est celui-ci : donner un accès à sa fiduciaire sans
// lui donner les clés. Aujourd'hui il n'y a qu'un interrupteur — administrateur
// ou non — si bien que partager l'accès revient à partager le compte, avec le
// droit de modifier les livres et d'effacer les sauvegardes.
//
// # Trois décisions de conception, et la raison de chacune
//
// **Le rôle est lu dans la base à chaque requête, jamais dans le jeton.** Un
// jeton d'accès vit une heure. Si le rôle y était inscrit, rétrograder ou
// désactiver quelqu'un le laisserait agir avec ses anciens droits jusqu'à
// l'expiration — une heure pendant laquelle on croit avoir coupé l'accès. La
// base est locale, la lecture est un accès par clé primaire ; le coût est nul
// et toute une classe de privilèges périmés disparaît. Le jeton ne prouve plus
// que l'identité.
//
// **Refus par défaut.** Une permission inconnue, un rôle inconnu, un rôle vide :
// tout cela vaut « non ». Ajouter une route sans y penser ne doit pas l'ouvrir
// à tout le monde — c'est la façon la plus courante de créer un trou, parce que
// rien ne le signale.
//
// **Une seconde barrière indépendante de la première.** Les permissions par
// route dépendent de ce que le développeur a déclaré ; oublier une déclaration
// est humain. Un filtre global refuse donc toute méthode d'écriture à un rôle
// en lecture seule, quelle que soit la route. Les deux barrières tombent
// rarement ensemble.
package authz

import "net/http"

// Role est ce qu'un compte est autorisé à faire.
type Role string

const (
	// RoleAdmin : tout, y compris les comptes, les sauvegardes et la sécurité.
	RoleAdmin Role = "admin"
	// RoleAccountant : tient les livres. Ne touche ni aux comptes utilisateurs,
	// ni aux sauvegardes, ni aux réglages de sécurité.
	RoleAccountant Role = "accountant"
	// RoleViewer : consulte et exporte, n'écrit rien. C'est le rôle de la
	// fiduciaire à qui l'on ouvre les livres sans lui donner les clés.
	RoleViewer Role = "viewer"
)

// Valid reports whether r is a role this product knows.
//
// Une valeur inconnue — fichier bricolé, migration ratée, base restaurée d'une
// version future — n'est jamais traitée comme un rôle permissif.
func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleAccountant, RoleViewer:
		return true
	}
	return false
}

// Label rend le rôle tel qu'on l'affiche.
func (r Role) Label() string {
	switch r {
	case RoleAdmin:
		return "Administrateur"
	case RoleAccountant:
		return "Comptable"
	case RoleViewer:
		return "Lecture seule"
	}
	return "Inconnu"
}

// Permission nomme une capacité, pas une route. Les routes changent, les
// capacités beaucoup moins — et raisonner sur « qui peut écrire au journal »
// est possible, alors que raisonner sur trente chemins d'URL ne l'est pas.
type Permission string

const (
	// PermRead : consulter et exporter. Tout compte actif l'a.
	PermRead Permission = "read"
	// PermWriteDocuments : factures, offres, contacts, paiements.
	PermWriteDocuments Permission = "write_documents"
	// PermWriteAccounting : journal, plan comptable, exercices, TVA.
	PermWriteAccounting Permission = "write_accounting"
	// PermManage : administration COMPTABLE — clôture d'exercice, réglages de
	// l'entreprise, contrôle d'intégrité, création de sauvegardes, effacement
	// des données personnelles (nLPD art. 6 al. 4).
	//
	// C'est le métier du comptable, pas celui de l'administrateur du logiciel.
	// Ces actions étaient réservées à PermAdmin, si bien qu'un comptable ne
	// pouvait pas boucler son exercice ni vérifier ses propres livres — il
	// devait demander à quelqu'un dont le rôle est de gérer des mots de passe.
	PermManage Permission = "manage"
	// PermAdmin : sécurité du logiciel et comptes utilisateurs. Chiffrement de
	// la base, restauration d'une sauvegarde, réseau et TLS, clé de signature,
	// journal de sécurité, création et rôles des comptes.
	//
	// La frontière est celle-ci : PermManage administre la COMPTABILITÉ,
	// PermAdmin administre le LOGICIEL et QUI Y ACCÈDE. Tout ce dont l'abus ne
	// se répare pas, ou qui donne accès à autre chose que des chiffres, reste
	// ici.
	PermAdmin Permission = "admin"
)

// grants dit ce que chaque rôle peut faire. Une table, pas une cascade de
// conditions : on la lit d'un regard, et un droit ajouté par erreur se voit.
var grants = map[Role]map[Permission]bool{
	RoleAdmin: {
		PermRead: true, PermWriteDocuments: true, PermWriteAccounting: true,
		PermManage: true, PermAdmin: true,
	},
	// Le comptable fait TOUT sur la comptabilité — y compris clôturer
	// l'exercice, régler la fiche entreprise et contrôler l'intégrité des
	// livres. Il ne touche ni à la sécurité du logiciel ni aux comptes des
	// utilisateurs.
	RoleAccountant: {
		PermRead: true, PermWriteDocuments: true, PermWriteAccounting: true,
		PermManage: true,
	},
	RoleViewer: {
		PermRead: true,
	},
}

// RequiresSecondFactor dit si un rôle doit inscrire un second facteur.
//
// Administrateur ET comptable : le premier détient les clés de l'installation,
// le second écrit dans les livres — un mot de passe volé sur l'un ou l'autre
// permet de fabriquer une comptabilité.
//
// La lecture seule en est dispensée. Ce rôle ne peut RIEN modifier : le pire
// qu'un mot de passe volé permette est de lire des chiffres, ce qui est déjà
// couvert par le chiffrement de la base et du disque. Lui imposer un téléphone
// coûterait plus qu'il ne protège — et c'est le rôle qu'on donne à sa
// fiduciaire, à qui l'on ne dicte pas son équipement.
func RequiresSecondFactor(r Role) bool {
	return r == RoleAdmin || r == RoleAccountant
}

// Can reports whether the role carries the permission.
//
// Refus par défaut : un rôle absent de la table ou une permission absente du
// rôle valent « non ». Aucun chemin ne rend « oui » sans une entrée explicite.
func Can(r Role, p Permission) bool {
	perms, ok := grants[r]
	if !ok {
		return false
	}
	return perms[p]
}

// IsWriteMethod reports whether an HTTP method changes state.
//
// Sert à la seconde barrière : un rôle en lecture seule ne passe aucune de ces
// méthodes, quelle que soit la route et quoi qu'ait déclaré — ou oublié de
// déclarer — le développeur.
//
// La liste est en « tout sauf » plutôt qu'en énumération des méthodes
// d'écriture : une méthode inhabituelle inconnue de cette fonction doit compter
// comme une écriture, pas passer parce qu'elle n'était pas prévue.
func IsWriteMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}

// RoleFromLegacyAdmin traduit l'ancien interrupteur booléen.
//
// Avant les rôles, un compte était administrateur ou ne l'était pas. Le second
// cas devient « comptable » et non « lecture seule » : ces comptes écrivaient
// des factures, et les rétrograder en lecture seule casserait des installations
// existantes du jour au lendemain. Élargir un droit acquis serait pire —
// personne ne devient administrateur par une migration.
func RoleFromLegacyAdmin(isAdmin bool) Role {
	if isAdmin {
		return RoleAdmin
	}
	return RoleAccountant
}
