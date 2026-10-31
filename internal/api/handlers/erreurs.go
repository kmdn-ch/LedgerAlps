package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// msgErreurInterne est la phrase rendue quand une défaillance du serveur n'a
// pas de message plus utile à donner. Elle est au catalogue i18n.
const msgErreurInterne = "erreur interne : la cause est consignée au journal du serveur"

// erreurInterne répond 500 SANS révéler la cause.
//
// Renvoyer `err.Error()` au client exposait ce qu'il n'a pas à savoir : noms de
// tables et de colonnes, chemins de fichiers du poste, texte brut du pilote
// SQL (CWE-209). Une personne mal intentionnée y lit le schéma ; une personne
// de bonne foi n'y lit rien d'utilisable. La cause part au journal du serveur,
// avec la route, de quoi retrouver la ligne ; le client reçoit une phrase
// stable, traduite par le catalogue.
//
// `message` est la phrase adressée à l'utilisateur ; elle ne doit JAMAIS être
// construite avec la cause.
func erreurInterne(c *gin.Context, message string, cause error) {
	log.Printf("ERROR: %s %s : %s : %v", c.Request.Method, c.FullPath(), message, cause)
	c.JSON(http.StatusInternalServerError, gin.H{"error": message})
}

// marquesTechniques sont des fragments qu'aucun message métier ne contient et
// que les pilotes de base de données, eux, produisent.
//
// « syntax error » n'y figure PAS (audit 9, M-02). Il y était, et un relevé
// camt.053 malformé — « XML syntax error on line 1 » — répondait 500 au lieu de
// 422 : l'analyseur XML dit la même chose que le moteur SQL, et c'est le premier
// qui parle ici. Le moteur SQL, lui, s'annonce par « sql logic error ».
var marquesTechniques = []string{
	"sqlite", "sql:", "sql logic error", "pgx", "constraint failed", "unique constraint",
	"foreign key", "no such table", "no such column",
	"database is locked", "database disk image", "unable to open database",
	"disk i/o error", "driver:",
}

// detailTechnique dit si le message d'une erreur laisse voir le moteur de base
// de données ou le poste.
func detailTechnique(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, m := range marquesTechniques {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// refus répond `status` avec le message de l'erreur — SAUF si ce message laisse
// voir le moteur de base de données (« UNIQUE constraint failed:
// invoices.invoice_number », nom de table et de colonne compris). Dans ce cas
// c'est une défaillance du serveur, pas un refus : 500, cause au journal.
//
// Les services enveloppent volontiers une erreur de base (`fmt.Errorf("insert
// invoice: %w", err)`), et un gestionnaire qui répond 422 sur sa branche par
// défaut la rendait telle quelle au client (CWE-209).
func refus(c *gin.Context, status int, err error) {
	if detailTechnique(err) {
		erreurInterne(c, msgErreurInterne, err)
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
