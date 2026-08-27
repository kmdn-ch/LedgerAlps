package invoicing

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
)

// Comptabiliser une facture à l'émission change ce que contient le journal.
// Deux façons de se tromper coûtent cher, et les deux sont couvertes ici :
// écrire deux fois — le produit et la TVA due doublent — et écrire chez
// quelqu'un qui saisissait déjà à la main, ce qui produit le même doublon en
// silence sur toute une comptabilité.

func postingService(t *testing.T, autoPost bool) (*Service, *sql.DB, string) {
	t.Helper()
	s, database, contactID := newGuardDB(t, "CHE-123.456.789 TVA")
	s.accountingSvc = accounting.New(database, false)
	if _, err := database.Exec(`UPDATE company_settings SET auto_post_invoices = ?`, boolToInt(autoPost)); err != nil {
		t.Fatal(err)
	}
	return s, database, contactID
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// balance renvoie (débit, crédit) cumulés d'un compte, écritures comptabilisées.
func balance(t *testing.T, database *sql.DB, number string) (float64, float64) {
	t.Helper()
	var d, c sql.NullFloat64
	err := database.QueryRow(`
		SELECT COALESCE(SUM(jl.debit_amount),0), COALESCE(SUM(jl.credit_amount),0)
		FROM journal_lines jl
		JOIN journal_entries je ON je.id = jl.entry_id
		JOIN accounts a ON a.id = jl.account_id
		WHERE a.code = ? AND je.status = 'posted'`, number).Scan(&d, &c)
	if err != nil {
		t.Fatal(err)
	}
	return d.Float64, c.Float64
}

func TestUneFactureEnvoyeeEstComptabilisee(t *testing.T) {
	s, database, contactID := postingService(t, true)
	invID := makeInvoice(t, s, contactID, 1000, 8.1)

	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("envoi: %v", err)
	}

	// 1000.- hors taxe, 81.- de TVA, 1081.- dus par le client.
	if d, _ := balance(t, database, accountReceivables); d != 1081 {
		t.Errorf("débiteurs au débit = %.2f, attendu 1081.00", d)
	}
	if _, c := balance(t, database, accountRevenue); c != 1000 {
		t.Errorf("produits au crédit = %.2f, attendu 1000.00", c)
	}
	if _, c := balance(t, database, accountVATDue); c != 81 {
		t.Errorf("TVA due au crédit = %.2f, attendu 81.00", c)
	}

	var linked string
	if err := database.QueryRow(`SELECT COALESCE(journal_entry_id,'') FROM invoices WHERE id=?`, invID).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked == "" {
		t.Fatal("la facture n'est pas liée à son écriture : elle serait comptabilisée une seconde fois")
	}
}

// L'idempotence est ce qui rend l'automatisme sûr. Sans elle, un double clic
// double le chiffre d'affaires et la TVA due.
func TestUnDocumentNEstComptabiliseQuUneFois(t *testing.T) {
	s, database, contactID := postingService(t, true)
	invID := makeInvoice(t, s, contactID, 500, 0)

	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}
	// Rappel direct, comme le ferait un second clic ou une reprise après erreur.
	for i := 0; i < 3; i++ {
		if _, err := s.PostIssuedDocument(context.Background(), invID, DocumentTypeInvoice,
			"F-1", "u1", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), ""); err != nil {
			t.Fatalf("rappel %d: %v", i+1, err)
		}
	}

	if d, _ := balance(t, database, accountReceivables); d != 500 {
		t.Fatalf("débiteurs = %.2f après quatre appels, attendu 500.00 — le produit a été compté plusieurs fois", d)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM journal_entries WHERE status='posted'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("%d écritures comptabilisées, attendu 1", n)
	}
}

// Le cas qui protège les installations existantes : réglage éteint, rien ne
// bouge au journal. Quelqu'un qui saisissait à la main ne doit pas découvrir
// des doublons après une mise à jour.
func TestReglageEteintAucuneEcriture(t *testing.T) {
	s, database, contactID := postingService(t, false)
	invID := makeInvoice(t, s, contactID, 1000, 8.1)

	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := database.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d écritures créées alors que la comptabilisation automatique est éteinte", n)
	}
	var linked string
	database.QueryRow(`SELECT COALESCE(journal_entry_id,'') FROM invoices WHERE id=?`, invID).Scan(&linked)
	if linked != "" {
		t.Fatal("la facture est liée à une écriture qui ne devrait pas exister")
	}
}

// La migration doit éteindre le réglage pour les fiches DÉJÀ présentes, et le
// laisser actif pour celles créées ensuite. C'est ce qui fait qu'une mise à jour
// ne double aucune comptabilité tout en donnant le bon défaut aux nouveaux.
//
// Le SQL de la migration est joué tel quel sur une table qui porte déjà une
// fiche : c'est le seul montage qui reproduit une mise à jour.
func TestLaMigrationEteintLeReglagePourLesFichesExistantes(t *testing.T) {
	tmp, err := os.CreateTemp("", "ledgeralps-mig-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := sql.Open(db.SQLiteDriver, "file:"+filepath.ToSlash(tmp.Name()))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Une installation d'avant la migration : la colonne n'existe pas.
	if _, err := database.Exec(`CREATE TABLE company_settings (id TEXT PRIMARY KEY, company_name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO company_settings VALUES ('cs','Existante Sarl')`); err != nil {
		t.Fatal(err)
	}

	migration, err := db.MigrationSQL("0018_auto_post_invoices")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(migration); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var on int
	if err := database.QueryRow(`SELECT auto_post_invoices FROM company_settings WHERE id='cs'`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 0 {
		t.Fatal("réglage actif sur une fiche existante : la mise à jour doublerait les écritures")
	}

	// Une fiche créée APRÈS la migration prend la valeur par défaut, active.
	if _, err := database.Exec(`INSERT INTO company_settings (id, company_name) VALUES ('cs2','Nouvelle Sarl')`); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT auto_post_invoices FROM company_settings WHERE id='cs2'`).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != 1 {
		t.Fatal("réglage éteint sur une installation neuve : la comptabilisation ne démarrerait jamais")
	}
}

// La note de crédit passe l'écriture inverse — c'est tout l'objet du point, et
// l'ordre était imposé : sans l'écriture de la facture, contrepasser produirait
// un produit négatif sans contrepartie.
func TestUneNoteDeCreditContrepasse(t *testing.T) {
	s, database, contactID := postingService(t, true)
	invID := makeInvoice(t, s, contactID, 1000, 8.1)
	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}

	note, err := s.CreateCreditNote(context.Background(), invID, "u1", CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("note de crédit: %v", err)
	}
	if err := s.Transition(context.Background(), note.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("envoi de la note: %v", err)
	}

	// Facture puis note du même montant : tout se neutralise.
	d, c := balance(t, database, accountReceivables)
	if d != c {
		t.Errorf("débiteurs : débit %.2f ≠ crédit %.2f — la contrepassation ne solde pas", d, c)
	}
	rd, rc := balance(t, database, accountRevenue)
	if rd != rc {
		t.Errorf("produits : débit %.2f ≠ crédit %.2f", rd, rc)
	}
	vd, vc := balance(t, database, accountVATDue)
	if vd != vc {
		t.Errorf("TVA due : débit %.2f ≠ crédit %.2f — la TVA resterait due sur une facture annulée", vd, vc)
	}
}

// Sans TVA, pas de ligne de TVA. Une ligne à zéro encombre le grand livre sans
// rien apprendre, et fait croire à un assujettissement qui n'existe pas.
func TestPasDeLigneDeTVAQuandIlNYEnAPas(t *testing.T) {
	s, database, contactID := postingService(t, true)
	invID := makeInvoice(t, s, contactID, 400, 0)
	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}
	var n int
	err := database.QueryRow(`
		SELECT COUNT(*) FROM journal_lines jl
		JOIN accounts a ON a.id = jl.account_id WHERE a.code = ?`, accountVATDue).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d ligne(s) de TVA sur une facture à 0 %%", n)
	}
}

// Une offre n'est pas une opération : personne ne doit rien tant qu'elle n'est
// pas acceptée.
func TestUneOffreNEstPasComptabilisee(t *testing.T) {
	s, database, contactID := postingService(t, true)
	quote, err := s.CreateInvoice(context.Background(), "u1", CreateInvoiceRequest{
		DocumentType: DocumentTypeQuote,
		ContactID:    contactID,
		IssueDate:    time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:      time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:     "CHF",
		Lines:        []LineInput{{Description: "Devis", Quantity: 1, UnitPrice: 900, VATRate: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(context.Background(), quote.ID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}
	var n int
	database.QueryRow(`SELECT COUNT(*) FROM journal_entries`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d écriture(s) pour une offre envoyée", n)
	}
}

// L'écriture est équilibrée — c'est la condition de toute comptabilité en
// partie double, et la seule que le journal ne peut pas rattraper ensuite.
func TestLEcritureEstEquilibree(t *testing.T) {
	s, database, contactID := postingService(t, true)
	invID := makeInvoice(t, s, contactID, 1234.55, 8.1)
	if err := s.Transition(context.Background(), invID, models.InvoiceStatusSent); err != nil {
		t.Fatal(err)
	}
	var d, c float64
	err := database.QueryRow(`
		SELECT COALESCE(SUM(debit_amount),0), COALESCE(SUM(credit_amount),0)
		FROM journal_lines jl JOIN journal_entries je ON je.id = jl.entry_id
		WHERE je.status = 'posted'`).Scan(&d, &c)
	if err != nil {
		t.Fatal(err)
	}
	if d != c {
		t.Fatalf("débit %.2f ≠ crédit %.2f", d, c)
	}
}

// ─── Réouverture d'une facture annulée (audit 4, C-3) ────────────────────────
//
// `cancelled → draft` est une transition autorisée : elle sert à corriger une
// facture envoyée par erreur, puis à la renvoyer. Mais l'annulation ne
// réécrivait jamais `invoices.journal_entry_id`, qui continuait de désigner
// l'écriture d'ORIGINE — celle que l'extourne venait justement de neutraliser.
//
// Au renvoi, deux garde-fous indépendants (`TransitionBy` et
// `PostIssuedDocument`) lisaient ce même lien, toujours non vide, et en
// concluaient chacun « déjà comptabilisé ». La version corrigée partait donc
// au client sans qu'aucune écriture ne la porte, pendant que la déclaration
// TVA — qui agrège la table `invoices`, pas le journal — l'incluait bien.
// Le bilan et la TVA divergeaient en silence sur ce document.

func TestUneFactureRouverteEtCorrigeeEstRecomptabilisee(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	actor := Actor{UserID: "u1", IP: "127.0.0.1"}

	invID := makeInvoice(t, s, contactID, 1000, 0)

	// 1. Émission : première écriture, 1'000 au débit des débiteurs.
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusSent, actor); err != nil {
		t.Fatalf("émission: %v", err)
	}
	if d, _ := balance(t, database, accountReceivables); d != 1000 {
		t.Fatalf("après émission, débiteurs = %.2f, attendu 1000", d)
	}

	// 2. Annulation : extourne automatique, solde net nul.
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusCancelled, actor); err != nil {
		t.Fatalf("annulation: %v", err)
	}
	d, c := balance(t, database, accountReceivables)
	if d-c != 0 {
		t.Fatalf("après annulation, solde débiteurs = %.2f, attendu 0 (extourne)", d-c)
	}

	// 3. Réouverture pour correction.
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusDraft, actor); err != nil {
		t.Fatalf("réouverture: %v", err)
	}

	// 4. Correction du montant : 1'000 → 1'200.
	if _, err := s.UpdateInvoiceBy(ctx, invID, CreateInvoiceRequest{
		ContactID: contactID,
		IssueDate: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		DueDate:   time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		Currency:  "CHF",
		Lines:     []LineInput{{Description: "Prestation corrigée", Quantity: 1, UnitPrice: 1200, VATRate: 0}},
	}, actor); err != nil {
		t.Fatalf("correction: %v", err)
	}

	// 5. Renvoi : une NOUVELLE écriture doit porter les 1'200 corrigés.
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusSent, actor); err != nil {
		t.Fatalf("renvoi: %v", err)
	}

	d, c = balance(t, database, accountReceivables)
	if solde := d - c; solde != 1200 {
		t.Errorf("après renvoi, solde débiteurs = %.2f, attendu 1200 — "+
			"la version corrigée n'a pas été portée au journal", solde)
	}
	if _, cr := balance(t, database, accountRevenue); cr-1000 != 1200 {
		// 1000 (émission) + 1200 (renvoi) au crédit, moins 1000 extourné au débit.
		t.Errorf("produits nets = %.2f, attendu 1200", cr-1000)
	}
}

// L'annulation seule — de loin le cas courant — ne doit RIEN changer au lien
// comptable : la facture reste annulée, et l'archive légale (CO art. 958f)
// doit continuer de montrer quelle écriture l'avait portée.
func TestUneAnnulationSeuleConserveLeLienComptable(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	actor := Actor{UserID: "u1", IP: "127.0.0.1"}

	invID := makeInvoice(t, s, contactID, 500, 0)
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusSent, actor); err != nil {
		t.Fatalf("émission: %v", err)
	}
	var lienApresEmission string
	if err := database.QueryRow(
		`SELECT COALESCE(journal_entry_id,'') FROM invoices WHERE id = ?`, invID,
	).Scan(&lienApresEmission); err != nil {
		t.Fatal(err)
	}
	if lienApresEmission == "" {
		t.Fatal("aucune écriture liée après émission")
	}

	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusCancelled, actor); err != nil {
		t.Fatalf("annulation: %v", err)
	}

	var lienApresAnnulation string
	if err := database.QueryRow(
		`SELECT COALESCE(journal_entry_id,'') FROM invoices WHERE id = ?`, invID,
	).Scan(&lienApresAnnulation); err != nil {
		t.Fatal(err)
	}
	if lienApresAnnulation != lienApresEmission {
		t.Errorf("le lien comptable a changé à l'annulation (%q → %q) : "+
			"une facture annulée doit garder la trace de l'écriture qui l'a portée",
			lienApresEmission, lienApresAnnulation)
	}
}

// Cas limite : une facture PARTIELLEMENT PAYÉE, annulée puis rouverte.
//
// `TransitionBy` ne garde pas sur `amount_paid` (seul `UpdateInvoiceBy` le
// fait), donc ce chemin est atteignable. Avant le correctif, le renvoi ne
// produisait aucune écriture : les livres montraient un produit nul alors que
// la facture était « envoyée » pour 1'000 avec 400 encaissés — un état que
// personne ne peut expliquer. Le correctif rétablit la créance à l'émission,
// ce qui redonne des livres cohérents avec le document.
func TestUneFacturePartiellementPayeeRouverteEstRecomptabilisee(t *testing.T) {
	s, database, contactID := postingService(t, true)
	ctx := context.Background()
	actor := Actor{UserID: "u1", IP: "127.0.0.1"}

	invID := makeInvoice(t, s, contactID, 1000, 0)
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusSent, actor); err != nil {
		t.Fatalf("émission: %v", err)
	}
	// Un encaissement partiel a eu lieu. On pose seulement le montant : le
	// mouvement de trésorerie appartient au service des paiements, pas ici.
	if _, err := database.Exec(`UPDATE invoices SET amount_paid = 400 WHERE id = ?`, invID); err != nil {
		t.Fatal(err)
	}

	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusCancelled, actor); err != nil {
		t.Fatalf("annulation: %v", err)
	}
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusDraft, actor); err != nil {
		t.Fatalf("réouverture: %v", err)
	}
	if err := s.TransitionBy(ctx, invID, models.InvoiceStatusSent, actor); err != nil {
		t.Fatalf("renvoi: %v", err)
	}

	// Émission (1000) − extourne (1000) + réémission (1000) = 1000 au débit net.
	d, c := balance(t, database, accountReceivables)
	if solde := d - c; solde != 1000 {
		t.Errorf("solde débiteurs = %.2f, attendu 1000 — la réémission doit "+
			"rétablir la créance que l'extourne avait annulée", solde)
	}
	if _, cr := balance(t, database, accountRevenue); cr-1000 != 1000 {
		t.Errorf("produits nets = %.2f, attendu 1000 — des livres à zéro sur une "+
			"facture envoyée seraient inexplicables", cr-1000)
	}
}
