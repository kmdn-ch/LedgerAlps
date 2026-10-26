package invoicing

// L'identité du destinataire : relevée sur le brouillon, vérifiée et figée à
// l'émission, jamais réécrite ensuite.
//
// La migration 0014 promettait une identité « figée à l'émission ». Le code la
// copiait à la création du brouillon et ne la relisait jamais, ce qui laissait
// quatre trous que ces tests ferment un à un :
//
//   - changer le client d'un brouillon laissait l'identité de l'ancien ;
//   - compléter la fiche d'un client après coup ne servait à rien : le brouillon
//     gardait l'adresse d'origine, et si elle était vide il ne pouvait plus partir ;
//   - une offre convertie et une note de crédit n'avaient aucune identité figée ;
//   - rien ne refusait, côté serveur, d'envoyer à un client sans adresse (audit 5, M-1).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

// identite lit l'identité figée sur un document, telle que le PDF l'imprime.
type identite struct {
	nom, rue, npa, ville string
	reconstituee         int
}

func (f *fixture) identite(t *testing.T, documentID string) identite {
	t.Helper()
	var i identite
	if err := f.db.QueryRow(`
		SELECT COALESCE(recipient_name,''), COALESCE(recipient_address,''),
		       COALESCE(recipient_postal_code,''), COALESCE(recipient_city,''),
		       recipient_backfilled
		FROM invoices WHERE id = ?`, documentID).
		Scan(&i.nom, &i.rue, &i.npa, &i.ville, &i.reconstituee); err != nil {
		t.Fatalf("lire l'identité de %s: %v", documentID, err)
	}
	return i
}

// nouveauContact crée un client ; adresse vide = client sans adresse.
func (f *fixture) nouveauContact(t *testing.T, nom, rue, npa, ville string) string {
	t.Helper()
	id := db.NewID()
	if _, err := f.db.Exec(`
		INSERT INTO contacts (id, name, contact_type, address, postal_code, city, country)
		VALUES (?, ?, 'customer', ?, ?, ?, 'CH')`, id, nom, rue, npa, ville); err != nil {
		t.Fatalf("seed contact %q: %v", nom, err)
	}
	return id
}

func (f *fixture) modifier(t *testing.T, documentID, contactID string, docType string) {
	t.Helper()
	issued := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := f.svc.UpdateInvoice(context.Background(), documentID, CreateInvoiceRequest{
		DocumentType: docType,
		ContactID:    contactID,
		IssueDate:    issued,
		DueDate:      issued.AddDate(0, 0, 30),
		Currency:     "CHF",
		Lines:        []LineInput{{Description: "Prestation", Quantity: 1, UnitPrice: 100, Sequence: 1}},
	}); err != nil {
		t.Fatalf("UpdateInvoice: %v", err)
	}
}

func (f *fixture) facture(t *testing.T, contactID string, docType string) *models.Invoice {
	t.Helper()
	issued := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	inv, err := f.svc.CreateInvoice(context.Background(), f.userID, CreateInvoiceRequest{
		DocumentType: docType,
		ContactID:    contactID,
		IssueDate:    issued,
		DueDate:      issued.AddDate(0, 0, 30),
		Currency:     "CHF",
		Lines:        []LineInput{{Description: "Prestation", Quantity: 1, UnitPrice: 100, Sequence: 1}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice(%s): %v", docType, err)
	}
	return inv
}

// ─── Brouillon : l'identité suit le client ───────────────────────────────────

// Re-adresser un brouillon à un autre client doit changer le destinataire
// imprimé. `contact_id` était réécrit, l'identité figée non : la facture partait
// au nom du premier client alors que l'enregistrement désignait le second.
func TestChangerLeClientDUnBrouillonChangeLeDestinataireImprime(t *testing.T) {
	f := newFixture(t)
	autre := f.nouveauContact(t, "Autre Client SA", "Rue B 2", "2000", "Neuchâtel")
	inv := f.create(t, DocumentTypeInvoice)

	f.modifier(t, inv.ID, autre, DocumentTypeInvoice)

	got := f.identite(t, inv.ID)
	if got.nom != "Autre Client SA" || got.rue != "Rue B 2" || got.ville != "Neuchâtel" {
		t.Fatalf("identité = %+v : le brouillon re-adressé porte encore l'ancien client", got)
	}
}

// Enregistrer de nouveau un brouillon dont le client est inchangé relit sa
// fiche : c'est ce qui permet de corriger une adresse après coup.
func TestUnBrouillonSuitLaFicheDeSonClient(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	if _, err := f.db.Exec(`UPDATE contacts SET name = 'Client SA (renommé)', city = 'Genève' WHERE id = ?`, f.contactID); err != nil {
		t.Fatal(err)
	}

	f.modifier(t, inv.ID, f.contactID, DocumentTypeInvoice)

	got := f.identite(t, inv.ID)
	if got.nom != "Client SA (renommé)" || got.ville != "Genève" {
		t.Fatalf("identité = %+v : le brouillon n'a pas relu la fiche", got)
	}
}

// ─── Émission : vérifiée et figée ────────────────────────────────────────────

// Un document ne part pas vers un client sans adresse. L'échec doit être net,
// au bon moment, et ne rien changer : le document reste un brouillon.
func TestEmissionRefuseUnClientSansAdresse(t *testing.T) {
	f := newFixture(t)
	sans := f.nouveauContact(t, "Client sans adresse", "", "", "")
	inv := f.facture(t, sans, DocumentTypeInvoice) // un brouillon incomplet est légitime

	err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent)
	if !errors.Is(err, ErrRecipientAddressIncomplete) {
		t.Fatalf("erreur = %v, attendu ErrRecipientAddressIncomplete", err)
	}

	var statut string
	if err := f.db.QueryRow(`SELECT status FROM invoices WHERE id = ?`, inv.ID).Scan(&statut); err != nil {
		t.Fatal(err)
	}
	if statut != string(models.InvoiceStatusDraft) {
		t.Fatalf("statut = %q : le refus a pourtant changé l'état du document", statut)
	}
}

// Chacun des trois champs compte : la rue, le NPA et la localité.
func TestEmissionRefuseUneAdresseOuIlManqueUnChamp(t *testing.T) {
	cas := map[string][3]string{
		"sans rue":      {"", "1000", "Lausanne"},
		"sans NPA":      {"Rue du Lac 1", "", "Lausanne"},
		"sans localité": {"Rue du Lac 1", "1000", ""},
		"espaces seuls": {"  ", "1000", "Lausanne"},
	}
	for nom, a := range cas {
		t.Run(nom, func(t *testing.T) {
			f := newFixture(t)
			c := f.nouveauContact(t, "Client", a[0], a[1], a[2])
			inv := f.facture(t, c, DocumentTypeInvoice)
			if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); !errors.Is(err, ErrRecipientAddressIncomplete) {
				t.Fatalf("erreur = %v, attendu ErrRecipientAddressIncomplete", err)
			}
		})
	}
}

// Le même refus vaut pour une offre : elle devient une facture d'un clic, et
// l'interface la bloque déjà au même titre.
func TestEmissionRefuseUneOffreSansAdresse(t *testing.T) {
	f := newFixture(t)
	sans := f.nouveauContact(t, "Client sans adresse", "", "", "")
	offre := f.facture(t, sans, DocumentTypeQuote)

	if err := f.svc.Transition(context.Background(), offre.ID, models.InvoiceStatusSent); !errors.Is(err, ErrRecipientAddressIncomplete) {
		t.Fatalf("erreur = %v, attendu ErrRecipientAddressIncomplete", err)
	}
}

// Le dernier mot est celui de la fiche au moment d'envoyer. Un client dont on
// complète l'adresse APRÈS la création du brouillon doit pouvoir être facturé
// sans rouvrir ni réenregistrer le document — sinon le brouillon créé trop tôt
// ne partirait plus jamais.
func TestEmissionRelitLaFicheCompletee(t *testing.T) {
	f := newFixture(t)
	c := f.nouveauContact(t, "Client à compléter", "", "", "")
	inv := f.facture(t, c, DocumentTypeInvoice)

	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); !errors.Is(err, ErrRecipientAddressIncomplete) {
		t.Fatalf("avant complément : erreur = %v, attendu ErrRecipientAddressIncomplete", err)
	}
	if _, err := f.db.Exec(`UPDATE contacts SET address = 'Rue Neuve 9', postal_code = '1700', city = 'Fribourg' WHERE id = ?`, c); err != nil {
		t.Fatal(err)
	}

	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("après complément : %v", err)
	}
	got := f.identite(t, inv.ID)
	if got.rue != "Rue Neuve 9" || got.npa != "1700" || got.ville != "Fribourg" {
		t.Fatalf("identité = %+v : l'émission n'a pas figé la fiche complétée", got)
	}
	if got.reconstituee != 0 {
		t.Errorf("recipient_backfilled = %d : une identité relevée à l'émission n'est pas une reconstitution", got.reconstituee)
	}
}

// Une note de crédit n'est pas gardée par la règle d'adresse (l'interface ne
// l'exige pas non plus : elle ne porte pas de bulletin de versement), et elle
// porte l'identité de la facture qu'elle corrige.
func TestUneNoteDeCreditPorteLIdentiteDeLaFactureCorrigee(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send: %v", err)
	}
	avant := f.identite(t, inv.ID)

	// Le client déménage et change de nom APRÈS l'envoi de la facture.
	if _, err := f.db.Exec(`UPDATE contacts SET name = 'Nouveau Nom SA', address = 'Rue Neuve 9', city = 'Fribourg' WHERE id = ?`, f.contactID); err != nil {
		t.Fatal(err)
	}

	note, err := f.svc.CreateCreditNote(context.Background(), inv.ID, f.userID, CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	got := f.identite(t, note.ID)
	if got.nom != avant.nom || got.rue != avant.rue || got.ville != avant.ville {
		t.Fatalf("identité de la note = %+v, attendu celle de la facture corrigée %+v", got, avant)
	}

	// Et son émission ne la réécrit pas depuis la fiche, qui a changé.
	if err := f.svc.Transition(context.Background(), note.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send credit note: %v", err)
	}
	if apres := f.identite(t, note.ID); apres != got {
		t.Fatalf("l'émission a réécrit l'identité de la note : %+v puis %+v", got, apres)
	}
}

// ─── Après l'émission : plus rien ne bouge ───────────────────────────────────

// Renommer le client, puis modifier une facture déjà envoyée sans changer de
// client, ne doit pas réécrire la pièce (CO art. 958f).
func TestUneFactureEnvoyeeNeSuitPlusLaFiche(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send: %v", err)
	}
	avant := f.identite(t, inv.ID)

	if _, err := f.db.Exec(`UPDATE contacts SET name = 'Autre Nom SA', address = 'Ailleurs 5', city = 'Sion' WHERE id = ?`, f.contactID); err != nil {
		t.Fatal(err)
	}
	f.modifier(t, inv.ID, f.contactID, DocumentTypeInvoice) // même client

	if apres := f.identite(t, inv.ID); apres != avant {
		t.Fatalf("la pièce envoyée a été réécrite : %+v puis %+v", avant, apres)
	}
}

// Mais re-adresser une facture envoyée à un autre client change son
// destinataire, et l'identité doit le suivre : sans quoi la pièce nommerait
// quelqu'un d'autre que celui qui la reçoit.
func TestRedresserUneFactureEnvoyeeChangeSonDestinataire(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send: %v", err)
	}
	autre := f.nouveauContact(t, "Autre Client SA", "Rue B 2", "2000", "Neuchâtel")

	f.modifier(t, inv.ID, autre, DocumentTypeInvoice)

	if got := f.identite(t, inv.ID); got.nom != "Autre Client SA" {
		t.Fatalf("identité = %+v : la facture re-adressée nomme encore l'ancien client", got)
	}
}

// ─── Créations dérivées : une identité dès la naissance ──────────────────────

// Une offre convertie et une note de crédit étaient insérées sans identité :
// rien ne les figeait avant le prochain démarrage du serveur, qui les
// « reconstituait » alors depuis le contact vivant et les marquait comme telles.
func TestUneOffreConvertieNaitAvecUneIdentite(t *testing.T) {
	f := newFixture(t)
	offre := f.sentQuote(t)

	converti, err := f.svc.ConvertQuote(context.Background(), offre.ID, f.userID, ConvertQuoteRequest{})
	if err != nil {
		t.Fatalf("ConvertQuote: %v", err)
	}
	got := f.identite(t, converti.ID)
	if got.nom != "Client SA" || got.rue != "Rue du Lac 1" {
		t.Fatalf("identité = %+v : la facture convertie est née sans identité", got)
	}
	if got.reconstituee != 0 {
		t.Errorf("recipient_backfilled = %d", got.reconstituee)
	}
}
