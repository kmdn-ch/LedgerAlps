package i18n

// Les messages qui ne sont PAS destinés à l'utilisateur.
//
// # Pourquoi une liste explicite plutôt qu'une devinette
//
// La première version de ce paquet décidait « est-ce du français ? » à partir
// des accents et de quelques mots-outils. « identifiants incorrects » n'a ni
// accent ni mot-outil : le message le plus vu du produit est passé à travers,
// et rien ne l'a signalé.
//
// La règle est donc inversée. TOUT message qui part vers l'utilisateur doit
// figurer au catalogue, et ce qui n'y va pas se déclare ici, un par un, avec
// sa raison. Une exception qu'il faut écrire est une exception qu'on relit ;
// une devinette, non.
//
// # Ce qui a le droit d'y figurer
//
//   - Ce qui échoue au DÉMARRAGE, avant qu'aucune requête n'existe : chargement
//     du flux de conformité, vérification de sa signature. Personne ne les lit
//     dans une langue, ils vont au journal.
//   - Ce qui appartient à l'OUTIL EN LIGNE DE COMMANDE, dont l'interface est
//     l'anglais.
//   - Les échecs d'INFRASTRUCTURE dont le texte sert à retrouver la ligne :
//     « rows error », « scan error ». Les traduire ferait perdre le seul point
//     d'entrée qu'on a en lisant un rapport d'incident.
//
// Ce qui n'y a PAS sa place : un refus que l'utilisateur peut provoquer en
// cliquant.
var diagnostic = map[string]string{
	// ── Chargement du flux de conformité, au démarrage ────────────────────────
	"advisory %d has no id":               "chargement du flux, au démarrage",
	"advisory %q has no source_url":       "chargement du flux, au démarrage",
	"advisory %q has unknown severity %q": "chargement du flux, au démarrage",
	"advisory feed schema version %d is not supported by this build (expects %d)": "chargement du flux, au démarrage",
	"advisory feed signature verification failed — refusing to load":              "chargement du flux, au démarrage",
	// La liste blanche des tables rattachables refuse un identifiant que le
	// PROGRAMME lui passe, pas l'utilisateur : aucun clic ne peut l'atteindre,
	// et le message doit rester lisible dans le journal de démarrage.
	"table non rattachable à un exercice: %q/%q — cette fonction n'accepte que des identifiants figés dans le programme, jamais une valeur reçue": "erreur de programmation, au démarrage",

	"invalid advisory public key length %d": "chargement du flux, au démarrage",

	// ── Outil en ligne de commande ────────────────────────────────────────────
	"automatic backup is only supported for SQLite; use pg_dump for PostgreSQL": "outil en ligne de commande",
	"could not find a free backup filename in %s":                               "outil en ligne de commande",

	// ── Infrastructure : le texte sert à retrouver la ligne ───────────────────
	"rows error":                    "défaut d'infrastructure, va au journal",
	"scan error":                    "défaut d'infrastructure, va au journal",
	"unexpected signing method: %v": "défaut d'infrastructure, va au journal",
	"integrity check reported: %s":  "sortie brute de SQLite, à recopier telle quelle",
	"integrity_check: %s":           "sortie brute de SQLite, à recopier telle quelle",

	// ── Le logo : cette erreur ne sort jamais telle quelle ────────────────────
	//
	// `ajusterLogo` la rend au gestionnaire, qui la remplace par une phrase
	// traduite. La garder en français ne coûte rien et dit, à la lecture d'un
	// journal, ce qui n'allait pas dans l'adresse de données reçue.
	"adresse de données sans virgule": "cause interne, remplacée par le gestionnaire",

	// ── Appels réseau sortants : le code HTTP est la donnée utile ─────────────
	"update check: HTTP %d":                                  "diagnostic d'appel sortant",
	"update check: endpoint returned a draft or pre-release": "diagnostic d'appel sortant",
	"zefix GET: HTTP %d":                                     "diagnostic d'appel sortant",
	"zefix search: HTTP %d":                                  "diagnostic d'appel sortant",
}

// EstDiagnostic dit si une phrase est délibérément hors catalogue.
func EstDiagnostic(fr string) bool {
	_, ok := diagnostic[fr]
	return ok
}
