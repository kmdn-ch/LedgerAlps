package accounting

// Traçabilité des actions sur les documents.
//
// La chaîne d'empreintes du CO art. 957a al. 2 ch. 5 ne couvrait que le
// journal. Les factures, les contacts et les paiements portaient un
// `created_by_id` — qui a créé la ligne — et rien d'autre : impossible de
// savoir qui avait envoyé une facture, qui l'avait annulée, qui avait modifié
// un IBAN de client.
//
// Or c'est exactement ce que la traçabilité veut dire. « Qui a créé » sans
// « qui a modifié » laisse hors trace tout ce qui arrive à une pièce après sa
// naissance — et une pièce comptable vit longtemps.
//
// # Pourquoi le même journal que les écritures
//
// Un second registre, non chaîné, aurait été plus simple à écrire et sans
// valeur : sa force serait celle d'une table qu'on peut modifier. Les actions
// sur les documents entrent donc dans la MÊME chaîne d'empreintes, avec le même
// numéro de séquence et la même vérification. Altérer une trace casse la chaîne
// exactement comme pour une écriture.
//
// # La transaction
//
// L'écriture d'audit vit dans la transaction de l'action. Hors transaction, une
// coupure entre les deux laisserait soit une action sans trace — le cas qui
// compte — soit une trace sans action.

import (
	"context"
	"fmt"
)

// Actions tracées sur les documents. Nommées ici plutôt qu'en chaînes libres
// dans chaque appel : une faute de frappe produirait une action que personne ne
// retrouverait en filtrant.
const (
	ActionDocumentCreated    = "document_created"
	ActionDocumentUpdated    = "document_updated"
	ActionDocumentTransition = "document_transition"
	ActionCreditNoteIssued   = "credit_note_issued"
	ActionContactCreated     = "contact_created"
	ActionContactUpdated     = "contact_updated"
	ActionContactAnonymised  = "contact_anonymised"
	ActionPaymentRecorded    = "payment_recorded"
	ActionBankEntryMatched   = "bank_entry_matched"
)

// TableInvoices et TableContacts nomment les registres suivis.
const (
	TableInvoices = "invoices"
	TableContacts = "contacts"
	TablePayments = "payments"
	TableBankRecs = "bank_entries"
)

// RecordDocumentAction ajoute un maillon pour une action sur un document.
//
// `userID` vide est refusé : une trace sans auteur ne trace rien, et
// l'accepter en silence ferait croire à une couverture qui n'existe pas. Les
// chemins internes qui n'ont pas d'utilisateur — migrations, tâches de
// démarrage — n'ont pas à passer par ici.
func RecordDocumentAction(
	ctx context.Context,
	tx execQuerier,
	usePostgres bool,
	table, userID, action, recordID, ipAddress string,
	state map[string]any,
) error {
	if userID == "" {
		return fmt.Errorf("trace refusée: aucun auteur pour %s sur %s/%s", action, table, recordID)
	}
	if state == nil {
		state = map[string]any{}
	}
	_, _, err := AppendAuditEntryFor(ctx, tx, usePostgres,
		table, userID, action, recordID, ipAddress, state)
	return err
}
