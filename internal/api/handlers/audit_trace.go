package handlers

// Tracer une action depuis un handler, sans cérémonie.
//
// # Le constat qui a mené ici
//
// La chaîne d'empreintes du CO art. 957a existait, avec sa vérification et son
// attestation, et TROIS actions y entraient : la comptabilisation d'une
// écriture, la clôture d'un exercice, et le changement de statut d'une facture.
// Le reste des constantes était déclaré et jamais appelé.
//
// Un journal à trous est pire qu'un journal absent : on le consulte en croyant
// qu'il dit tout, et l'absence d'une ligne se lit comme « cela n'a pas eu
// lieu » alors qu'elle veut dire « cela n'a jamais été écrit ».
//
// # Et la première correction n'avait corrigé que la moitié
//
// Une version antérieure de ce commentaire nommait trois constantes en disant
// qu'elles « étaient déclarées et jamais appelées » — au passé, comme si le
// nécessaire avait été fait. Il ne l'avait pas été : au moment où cette phrase
// existait, l'unique occurrence de `ActionContactUpdated` dans tout le dépôt
// ÉTAIT ce commentaire. Le second audit a compté onze actions déclarées et
// jamais écrites, dont `ActionDataExported`, dont le commentaire promettait de
// répondre à la question que pose la nLPD art. 6.
//
// Un commentaire faux coûte plus cher qu'un commentaire absent : il fait
// renoncer à vérifier. Les quinze actions sont désormais câblées, et
// `internal/frontend/audit_actions_test.go` échoue si l'une cesse de l'être —
// y compris si elle ne subsiste que dans un commentaire qui l'affirme.
//
// # Pourquoi un helper plutôt que l'appel direct
//
// Le point d'entrée existant demande une transaction, un drapeau Postgres, une
// table, un auteur, une adresse. Six paramètres à réunir dans chaque handler,
// c'est six occasions de ne pas le faire. Ici l'auteur et l'adresse sont tirés
// du contexte de la requête, qui les porte déjà.
//
// # Ce que la trace ne garantit pas
//
// Elle est écrite APRÈS l'action et hors de sa transaction. Une coupure entre
// les deux laisse l'action sans trace. La limite est réelle et énoncée plutôt
// que masquée : la refermer demanderait de faire descendre la transaction
// depuis chaque appelant. Ce qui compte est qu'un échec de journalisation
// n'annule PAS l'action — la facture est envoyée, et la refuser après coup
// pour un défaut de trace laisserait la pièce dans un état que personne n'a
// voulu.

import (
	"context"
	"database/sql"
	"log"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Actions tracées depuis les handlers.
//
// Nommées ici plutôt qu'en chaînes libres : une faute de frappe produirait une
// action que personne ne retrouverait en filtrant le journal.
const (
	ActionSupplierInvoiceCreated   = "supplier_invoice_created"
	ActionSupplierInvoiceUpdated   = "supplier_invoice_updated"
	ActionSupplierInvoiceBooked    = "supplier_invoice_booked"
	ActionSupplierInvoiceCancelled = "supplier_invoice_cancelled"
	ActionSupplierInvoiceDeleted   = "supplier_invoice_deleted"

	ActionCompanySettingsUpdated = "company_settings_updated"
	ActionBankStatementImported  = "bank_statement_imported"
	ActionPaymentFileGenerated   = "payment_file_generated"

	// Les exports disent QUI a sorti les données de la machine. C'est la
	// question que pose un incident, et celle à laquelle la nLPD art. 6
	// demande de pouvoir répondre : la finalité et la proportionnalité d'un
	// traitement s'apprécient mal quand on ignore qui a copié quoi.
	ActionDataExported = "data_exported"
)

// Tables suivies, en plus de celles nommées dans le paquet accounting.
const (
	TableSupplierInvoices = "supplier_invoices"
	TableCompanySettings  = "company_settings"
	TableExports          = "exports"
	TableBankEntries      = "bank_entries"
)

// trace écrit un maillon d'audit pour l'action en cours.
//
// Silencieuse quand la requête n'a pas d'auteur — appel interne, tâche de
// démarrage : une trace sans auteur ne trace rien, et en accepter ferait croire
// à une couverture qui n'existe pas.
func trace(c *gin.Context, database *sql.DB, usePostgres bool,
	table, action, recordID string, t accounting.Transition) {
	claims := mw.GetClaims(c)
	if claims == nil || claims.UserID == "" {
		return
	}
	traceFor(c.Request.Context(), database, usePostgres,
		claims.UserID, c.ClientIP(), table, action, recordID, t)
}

// traceFor est la même chose sans gin, pour les chemins qui n'ont qu'un
// contexte.
func traceFor(ctx context.Context, database *sql.DB, usePostgres bool,
	userID, ip, table, action, recordID string, t accounting.Transition) {
	if userID == "" {
		return
	}
	// DANS une transaction, et pas sur *sql.DB.
	//
	// AppendAuditEntryFor lit le maillon précédent puis insère le suivant, et
	// documente lui-même exiger une transaction. Hors transaction, deux
	// requêtes concurrentes lisent le MÊME prédécesseur et écrivent deux
	// maillons portant le même numéro de séquence. La vérification lit alors
	// « lien rompu » et « -1 entrée supprimée », et pose Verified=false de
	// façon définitive : le produit accuse l'utilisateur d'avoir altéré ses
	// livres alors que personne n'a rien touché.
	//
	// `execQuerier` étant satisfait par *sql.DB comme par *sql.Tx, rien ne
	// signalait l'erreur à la compilation.
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("WARNING: action %s sur %s/%s non tracée: %v",
			action, table, recordID, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if err := accounting.RecordDocumentAction(ctx, tx, usePostgres,
		table, userID, action, recordID, ip, t); err != nil {
		// Journalisé, pas remonté : l'action a eu lieu et l'utilisateur n'y peut
		// rien. C'est l'exploitant qui doit voir cette ligne, pas l'utilisateur
		// qui vient d'envoyer une facture.
		log.Printf("WARNING: action %s sur %s/%s non tracée: %v",
			action, table, recordID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("WARNING: action %s sur %s/%s non tracée: %v",
			action, table, recordID, err)
	}
}

// traceExport écrit un maillon pour une sortie de données hors de la machine.
//
// C'est la promesse que porte `ActionDataExported` : dire QUI a sorti quoi, et
// quand. La constante était déclarée depuis la v1.5.3 et n'était écrite nulle
// part — le produit ne pouvait donc pas répondre à la question que pose un
// incident, celle à laquelle la nLPD art. 6 demande de pouvoir répondre.
//
// L'état décrit l'export, pas les données exportées : le nom du fichier, le
// format et la période suffisent à reconstituer ce qui est sorti, et recopier
// la charge utile dans une table conservée dix ans irait contre l'article même
// qu'on cherche à satisfaire.
func traceExport(c *gin.Context, database *sql.DB, usePostgres bool,
	nomFichier, format string, details map[string]any) {
	etat := map[string]any{
		"fichier": nomFichier,
		"format":  format,
	}
	for k, v := range details {
		etat[k] = v
	}
	// `recordID` porte le nom du fichier : c'est par lui qu'on retrouve un
	// export dans le journal, et il n'existe pas d'autre identifiant — un
	// export n'est pas une ligne de la base.
	trace(c, database, usePostgres, TableExports,
		ActionDataExported, nomFichier, accounting.Creation(etat))
}
