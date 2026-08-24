package invoicing

// Traçabilité des actions sur les documents.
//
// Une facture portait `created_by_id` — qui l'a créée — et rien d'autre. Qui
// l'a envoyée, qui l'a annulée, qui a corrigé son montant : rien. Or une pièce
// comptable vit longtemps, et l'essentiel de ce qui lui arrive lui arrive après
// sa naissance.
//
// Les actions entrent dans la MÊME chaîne d'empreintes que les écritures au
// journal (CO art. 957a al. 2 ch. 5), avec le même numéro de séquence et la
// même vérification. Un second registre non chaîné aurait été plus simple et
// sans valeur : sa force serait celle d'une table qu'on peut modifier.
//
// # Ce que la trace ne garantit pas
//
// Elle est écrite APRÈS l'action, pas dans sa transaction. Une coupure entre
// les deux laisse l'action sans trace. C'est une limite réelle et elle est
// énoncée plutôt que masquée : la refermer demanderait de faire descendre la
// transaction depuis chaque appelant, et une trace absente se voit — un trou
// dans la numérotation de séquence n'existe pas, mais l'action, elle, apparaît
// dans les livres sans maillon correspondant.

import (
	"context"
	"log"

	accsvc "github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Actor est qui agit, et depuis où.
//
// Vide signifie « pas d'auteur connu » : les appels internes et les tests ne
// tracent alors rien, plutôt que d'écrire une trace anonyme. Une trace sans
// auteur ne trace rien, et en accepter ferait croire à une couverture qui
// n'existe pas.
type Actor struct {
	UserID string
	IP     string
}

// record écrit un maillon pour une action sur un document.
//
// Un échec n'annule pas l'action : la facture EST envoyée, et la refuser après
// coup pour un défaut de journalisation laisserait le document dans un état que
// personne n'a voulu. L'échec est journalisé — c'est lui qui alerte, pas une
// erreur rendue à l'utilisateur qui n'y peut rien.
func (s *Service) record(ctx context.Context, a Actor, action, recordID string, state map[string]any) {
	if a.UserID == "" {
		return
	}
	// DANS une transaction : le maillon précédent est lu puis le suivant est
	// inséré, et hors transaction deux écritures concurrentes se partagent le
	// même numéro de séquence. La chaîne se fourche, et la vérification
	// annonce des livres altérés que personne n'a touchés.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("WARNING: action %s sur le document %s non tracée: %v", action, recordID, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if err := accsvc.RecordDocumentAction(ctx, tx, s.usePostgres,
		accsvc.TableInvoices, a.UserID, action, recordID, a.IP, state); err != nil {
		log.Printf("WARNING: action %s sur le document %s non tracée: %v", action, recordID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("WARNING: action %s sur le document %s non tracée: %v", action, recordID, err)
	}
}
