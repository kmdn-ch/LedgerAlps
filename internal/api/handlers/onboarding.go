package handlers

// La mise en route — ce qu'il faut avoir réglé avant qu'une facture suisse
// tienne debout.
//
// # Ce qui manquait
//
// Une installation neuve s'ouvre sur quatre compteurs à zéro. Rien ne dit que
// sans adresse structurée le bulletin QR sera refusé au guichet, ni que sans
// IBAN le PDF sortira sans section de paiement. L'information EXISTAIT — le
// contrôle de cohérence la produit déjà — mais dans Paramètres → Maintenance →
// Diagnostic, c'est-à-dire à trois clics d'un endroit où un débutant ne va
// jamais. Ce n'est pas un manque de fonction, c'est un manque de placement.
//
// # Pourquoi la liste se calcule ICI
//
// Parce que les règles sont ici. « Cette adresse suffit-elle à une QR-facture »
// se répond avec SIX IG v2.4 §4.2.2, et « cet IBAN est-il valide » avec le
// contrôle de clé ISO 13616 que porte `compliance.ValidateIBAN`. Recopier ces
// règles dans un composant React garantirait qu'elles divergent : c'est
// exactement ce qui est arrivé à la recherche IDE, écrite deux fois, corrigée
// une seule.
//
// # Pourquoi aucune phrase ne sort d'ici
//
// La réponse ne porte que des ÉTATS et des noms de champs. Les phrases vivent
// au catalogue du frontend, qui les rend dans les quatre langues. Un libellé
// rédigé ici ressortirait en français sur un écran allemand : l'intercepteur de
// traduction ne réécrit que `error` et `message`, et il a raison — ce sont les
// seuls champs dont la nature soit certaine.

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
	"github.com/kmdn-ch/ledgeralps/internal/core/zefix"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Les clés d'étape. Le frontend les traduit ; elles ne changent jamais, sous
// peine de faire disparaître une ligne de la liste sans que rien n'échoue.
const (
	ÉtapeIdentité = "identity"
	ÉtapeIDE      = "uid"
	ÉtapeIBAN     = "iban"
	ÉtapeClient   = "customer"
	ÉtapeFacture  = "invoice"
)

// ÉtapeMiseEnRoute est une case à cocher, et ce qui l'empêche de l'être.
type ÉtapeMiseEnRoute struct {
	Clé  string `json:"key"`
	Fait bool   `json:"done"`
	// Manquants nomme les CHAMPS qui bloquent, jamais une phrase. « Il manque
	// quelque chose » envoie chercher ; « il manque la localité » fait agir.
	Manquants []string `json:"missing,omitempty"`
}

// MiseEnRoute est l'état complet de la liste.
type MiseEnRoute struct {
	Étapes  []ÉtapeMiseEnRoute `json:"steps"`
	Faites  int                `json:"done_count"`
	Total   int                `json:"total"`
	Terminé bool               `json:"complete"`
}

type OnboardingHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewOnboardingHandler(database *sql.DB, usePostgres bool) *OnboardingHandler {
	return &OnboardingHandler{db: database, usePostgres: usePostgres}
}

// GetOnboarding GET /api/v1/onboarding
//
// Lecture seule et sans effet de bord : c'est un constat, pas une étape d'un
// assistant. Rien n'est mémorisé — l'état se relit des données à chaque appel,
// si bien qu'une case décochée par un effacement se redécoche d'elle-même.
func (h *OnboardingHandler) GetOnboarding(c *gin.Context) {
	ctx := c.Request.Context()

	var nom, npa, localité, pays, ide, iban string
	q := db.Rebind(`
		SELECT COALESCE(company_name,''), COALESCE(address_postal_code,''),
		       COALESCE(address_city,''), COALESCE(address_country,''),
		       COALESCE(che_number,''), COALESCE(iban,'')
		FROM company_settings LIMIT 1`, h.usePostgres)
	// Aucune fiche société : ce n'est pas une erreur, c'est le premier jour.
	// Les six variables restent vides et toutes les étapes sont à faire.
	_ = h.db.QueryRowContext(ctx, q).Scan(&nom, &npa, &localité, &pays, &ide, &iban)

	m := MiseEnRoute{}

	// ── L'identité, telle que la QR-facture l'exige ──────────────────────────
	//
	// SIX IG v2.4 §4.2.2 : l'adresse du créancier est STRUCTURÉE. Un champ
	// libre correct pour un facteur ne l'est pas pour une banque.
	var manqueIdentité []string
	if nom == "" {
		manqueIdentité = append(manqueIdentité, "company_name")
	}
	if npa == "" {
		manqueIdentité = append(manqueIdentité, "postal_code")
	}
	if localité == "" {
		manqueIdentité = append(manqueIdentité, "city")
	}
	if len(pays) != 2 {
		manqueIdentité = append(manqueIdentité, "country")
	}
	m.ajoute(ÉtapeIdentité, manqueIdentité)

	// ── Le numéro IDE ────────────────────────────────────────────────────────
	//
	// Le format seulement, sans interroger le registre : cette liste s'affiche
	// à chaque ouverture du tableau de bord, et LedgerAlps n'a pas à dépendre
	// d'un réseau pour dire ce qui manque.
	var manqueIDE []string
	switch {
	case ide == "":
		manqueIDE = append(manqueIDE, "uid_missing")
	case !zefix.ValidFormat(ide):
		manqueIDE = append(manqueIDE, "uid_invalid")
	}
	m.ajoute(ÉtapeIDE, manqueIDE)

	// ── L'IBAN ───────────────────────────────────────────────────────────────
	//
	// Un IBAN présent mais faux est PIRE qu'absent : le PDF porte alors un
	// bulletin d'apparence normale que la banque refusera.
	var manqueIBAN []string
	switch {
	case iban == "":
		manqueIBAN = append(manqueIBAN, "iban_missing")
	case compliance.ValidateIBAN(iban) != nil:
		manqueIBAN = append(manqueIBAN, "iban_invalid")
	}
	m.ajoute(ÉtapeIBAN, manqueIBAN)

	// ── Un client, puis une facture ──────────────────────────────────────────
	//
	// Les deux dernières étapes ne se règlent pas dans les paramètres : elles
	// se font. La liste se termine donc sur le geste qui est la raison d'être
	// du produit, et elle disparaît quand il a été posé.
	clients, _ := h.compte(ctx, `SELECT COUNT(*) FROM contacts`)
	if clients > 0 {
		m.ajoute(ÉtapeClient, nil)
	} else {
		m.ajoute(ÉtapeClient, []string{"no_customer"})
	}

	factures, _ := h.compte(ctx, `SELECT COUNT(*) FROM invoices`)
	if factures > 0 {
		m.ajoute(ÉtapeFacture, nil)
	} else {
		m.ajoute(ÉtapeFacture, []string{"no_invoice"})
	}

	m.Total = len(m.Étapes)
	m.Terminé = m.Faites == m.Total
	c.JSON(http.StatusOK, m)
}

// ajoute inscrit une étape et tient le compteur à jour.
//
// Le compteur se déduit des étapes plutôt que d'être incrémenté à la main :
// une étape ajoutée demain sans sa ligne `Faites++` afficherait « 4 / 6 »
// éternellement, et personne ne verrait la faute avant de compter les cases.
func (m *MiseEnRoute) ajoute(clé string, manquants []string) {
	fait := len(manquants) == 0
	if fait {
		m.Faites++
	}
	m.Étapes = append(m.Étapes, ÉtapeMiseEnRoute{Clé: clé, Fait: fait, Manquants: manquants})
}

func (h *OnboardingHandler) compte(ctx context.Context, q string) (int, error) {
	var n int
	if err := h.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
