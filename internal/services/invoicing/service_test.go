package invoicing

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/models"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-invoicing-*.db")
	if err != nil {
		t.Fatalf("temp db: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	database, err := db.Open(&config.Config{SQLitePath: tmp.Name()})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

type fixture struct {
	svc       *Service
	db        *sql.DB
	userID    string
	contactID string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	database := newTestDB(t)

	userID := db.NewID()
	if _, err := database.Exec(
		`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "conv-"+userID[:8]+"@example.ch", "Test", "x"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	contactID := db.NewID()
	if _, err := database.Exec(
		`INSERT INTO contacts (id, name, contact_type) VALUES (?, ?, 'customer')`,
		contactID, "Client SA"); err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	return &fixture{svc: New(database, false), db: database, userID: userID, contactID: contactID}
}

func (f *fixture) create(t *testing.T, docType string) *models.Invoice {
	t.Helper()
	issued := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	inv, err := f.svc.CreateInvoice(context.Background(), f.userID, CreateInvoiceRequest{
		DocumentType: docType,
		ContactID:    f.contactID,
		IssueDate:    issued,
		DueDate:      issued.AddDate(0, 0, 30),
		Currency:     "CHF",
		Lines: []LineInput{
			{Description: "Prestation A", Quantity: 2, UnitPrice: 500, VATRate: 8.1, Sequence: 1},
			{Description: "Prestation B", Quantity: 1, UnitPrice: 250, VATRate: 8.1, Sequence: 2},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice(%s): %v", docType, err)
	}
	return inv
}

// sentQuote is the only state a quote can legitimately be converted from.
func (f *fixture) sentQuote(t *testing.T) *models.Invoice {
	t.Helper()
	q := f.create(t, DocumentTypeQuote)
	if err := f.svc.Transition(context.Background(), q.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send quote: %v", err)
	}
	return q
}

// ─── Conversion ───────────────────────────────────────────────────────────────

// The offer must survive. The client already holds a copy of it, so replacing
// the record would leave them citing a reference that no longer exists here —
// the link CO art. 958f al. 3 requires to stay guaranteed.
func TestConvertKeepsTheQuoteAndCreatesASeparateInvoice(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)

	inv, err := f.svc.ConvertQuote(context.Background(), quote.ID, f.userID, ConvertQuoteRequest{})
	if err != nil {
		t.Fatalf("ConvertQuote: %v", err)
	}

	if inv.ID == quote.ID {
		t.Fatal("conversion reused the quote's record instead of creating an invoice")
	}
	if !strings.HasPrefix(inv.InvoiceNumber, "FA-") {
		t.Errorf("invoice number = %q, want an FA- number", inv.InvoiceNumber)
	}
	if inv.DocumentType != DocumentTypeInvoice {
		t.Errorf("document_type = %q, want invoice", inv.DocumentType)
	}
	if inv.ConvertedFromID == nil || *inv.ConvertedFromID != quote.ID {
		t.Error("the invoice does not point back at the offer it came from")
	}

	// The offer is still there, unchanged in number and type, marked accepted.
	var number, docType, outcome string
	if err := f.db.QueryRow(
		`SELECT invoice_number, document_type, COALESCE(quote_outcome, '') FROM invoices WHERE id = ?`,
		quote.ID).Scan(&number, &docType, &outcome); err != nil {
		t.Fatalf("reload quote: %v", err)
	}
	if number != quote.InvoiceNumber || docType != DocumentTypeQuote {
		t.Errorf("the offer changed identity: number=%q type=%q", number, docType)
	}
	if outcome != QuoteOutcomeAccepted {
		t.Errorf("quote_outcome = %q, want accepted", outcome)
	}
}

func TestConvertCopiesEveryLineAndTotals(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)

	inv, err := f.svc.ConvertQuote(context.Background(), quote.ID, f.userID, ConvertQuoteRequest{})
	if err != nil {
		t.Fatalf("ConvertQuote: %v", err)
	}

	var lineCount int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM invoice_lines WHERE invoice_id = ?`, inv.ID).
		Scan(&lineCount); err != nil {
		t.Fatalf("count lines: %v", err)
	}
	if lineCount != 2 {
		t.Errorf("invoice has %d lines, want 2 — it must bill exactly what was offered", lineCount)
	}
	if inv.TotalAmount != quote.TotalAmount {
		t.Errorf("invoice total %.2f != quote total %.2f", inv.TotalAmount, quote.TotalAmount)
	}
}

// Converting twice would invoice the same work twice.
func TestConvertTwiceIsRefused(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)

	if _, err := f.svc.ConvertQuote(context.Background(), quote.ID, f.userID, ConvertQuoteRequest{}); err != nil {
		t.Fatalf("first conversion: %v", err)
	}
	_, err := f.svc.ConvertQuote(context.Background(), quote.ID, f.userID, ConvertQuoteRequest{})
	if !errors.Is(err, ErrQuoteAlreadyConv) {
		t.Errorf("second conversion returned %v, want ErrQuoteAlreadyConv", err)
	}
}

func TestConvertRejectsNonQuotes(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send invoice: %v", err)
	}
	_, err := f.svc.ConvertQuote(context.Background(), inv.ID, f.userID, ConvertQuoteRequest{})
	if !errors.Is(err, ErrNotAQuote) {
		t.Errorf("converting an invoice returned %v, want ErrNotAQuote", err)
	}
}

// A draft has not reached the client, so there is nothing to bill against.
func TestConvertRejectsDraftQuote(t *testing.T) {
	f := newFixture(t)
	quote := f.create(t, DocumentTypeQuote)
	_, err := f.svc.ConvertQuote(context.Background(), quote.ID, f.userID, ConvertQuoteRequest{})
	if !errors.Is(err, ErrQuoteNotConvertible) {
		t.Errorf("converting a draft returned %v, want ErrQuoteNotConvertible", err)
	}
}

func TestConvertRejectsUnknownID(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.ConvertQuote(context.Background(), db.NewID(), f.userID, ConvertQuoteRequest{})
	if !errors.Is(err, ErrInvoiceNotFound) {
		t.Errorf("got %v, want ErrInvoiceNotFound", err)
	}
}

// ─── State machine ────────────────────────────────────────────────────────────

// Nobody owes anything on an offer. Marking one paid used to be possible,
// which put it in the receivables and in the VAT declaration.
func TestQuoteCannotBecomePaid(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)

	err := f.svc.Transition(context.Background(), quote.ID, models.InvoiceStatusPaid)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("sent quote → paid returned %v, want ErrInvalidTransition", err)
	}
}

func TestInvoiceCanStillBecomePaid(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	ctx := context.Background()
	if err := f.svc.Transition(ctx, inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := f.svc.Transition(ctx, inv.ID, models.InvoiceStatusPaid); err != nil {
		t.Errorf("an invoice must still be payable: %v", err)
	}
}

// ─── Outcome ──────────────────────────────────────────────────────────────────

func TestSetQuoteOutcomeRecordsRefusal(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)

	if err := f.svc.SetQuoteOutcome(context.Background(), quote.ID, QuoteOutcomeRefused); err != nil {
		t.Fatalf("SetQuoteOutcome: %v", err)
	}
	var outcome string
	if err := f.db.QueryRow(`SELECT COALESCE(quote_outcome, '') FROM invoices WHERE id = ?`, quote.ID).
		Scan(&outcome); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if outcome != QuoteOutcomeRefused {
		t.Errorf("quote_outcome = %q, want refused", outcome)
	}
}

// "accepted" is reached by producing the invoice, never by flipping a field —
// otherwise an offer could read as accepted with no invoice behind it.
func TestAcceptedOutcomeCannotBeSetDirectly(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)
	if err := f.svc.SetQuoteOutcome(context.Background(), quote.ID, QuoteOutcomeAccepted); !errors.Is(err, ErrInvalidQuoteOutcome) {
		t.Errorf("got %v, want ErrInvalidQuoteOutcome", err)
	}
}

func TestSetQuoteOutcomeRejectsGarbageAndNonQuotes(t *testing.T) {
	f := newFixture(t)
	quote := f.sentQuote(t)
	if err := f.svc.SetQuoteOutcome(context.Background(), quote.ID, "peut-être"); !errors.Is(err, ErrInvalidQuoteOutcome) {
		t.Errorf("unknown outcome returned %v, want ErrInvalidQuoteOutcome", err)
	}
	inv := f.create(t, DocumentTypeInvoice)
	if err := f.svc.SetQuoteOutcome(context.Background(), inv.ID, QuoteOutcomeRefused); !errors.Is(err, ErrNotAQuote) {
		t.Errorf("outcome on an invoice returned %v, want ErrNotAQuote", err)
	}
}

// ─── Numbering ────────────────────────────────────────────────────────────────

// Offers and invoices draw from separate yearly sequences, so converting an
// offer never disturbs the invoice numbering.
func TestQuotesAndInvoicesUseSeparateSequences(t *testing.T) {
	f := newFixture(t)
	q1 := f.create(t, DocumentTypeQuote)
	i1 := f.create(t, DocumentTypeInvoice)
	q2 := f.create(t, DocumentTypeQuote)

	if !strings.HasPrefix(q1.InvoiceNumber, "OF-2026-") || !strings.HasPrefix(q2.InvoiceNumber, "OF-2026-") {
		t.Errorf("quote numbers = %q, %q", q1.InvoiceNumber, q2.InvoiceNumber)
	}
	if !strings.HasPrefix(i1.InvoiceNumber, "FA-2026-") {
		t.Errorf("invoice number = %q", i1.InvoiceNumber)
	}
	if q1.InvoiceNumber == q2.InvoiceNumber {
		t.Error("two offers received the same number")
	}
}

// ─── « En retard » ────────────────────────────────────────────────────────────

// "overdue" was offered as a status by the UI but never existed server-side:
// the CHECK constraint forbids it and validTransitions has no path to it, so
// the "Marquer en retard" button could only ever fail. It is a derived state —
// an unpaid invoice past its due date — and must stay one.
func TestOverdueIsNotAStatusTheServerAccepts(t *testing.T) {
	f := newFixture(t)
	inv := f.create(t, DocumentTypeInvoice)
	ctx := context.Background()
	if err := f.svc.Transition(ctx, inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send: %v", err)
	}

	err := f.svc.Transition(ctx, inv.ID, models.InvoiceStatus("overdue"))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("sent → overdue returned %v, want ErrInvalidTransition", err)
	}

	// And the invoice must be untouched: a rejected transition that still wrote
	// would leave a status the CHECK constraint forbids.
	var status string
	if err := f.db.QueryRow(`SELECT status FROM invoices WHERE id = ?`, inv.ID).Scan(&status); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if status != string(models.InvoiceStatusSent) {
		t.Errorf("status = %q after a refused transition, want sent", status)
	}
}

// ─── Notes de crédit ──────────────────────────────────────────────────────────

func (f *fixture) sentInvoice(t *testing.T) *models.Invoice {
	t.Helper()
	inv := f.create(t, DocumentTypeInvoice)
	if err := f.svc.Transition(context.Background(), inv.ID, models.InvoiceStatusSent); err != nil {
		t.Fatalf("send invoice: %v", err)
	}
	return inv
}

// A credit note that references nothing leaves a controller unable to reach the
// invoice whose VAT it reduced (CO art. 957a al. 2 ch. 5). LTVA art. 27 al. 4
// calls the correction "un document qui mentionne et annule la facture
// d'origine" — the mention is this link.
func TestCreditNoteLinksToTheInvoiceItCorrects(t *testing.T) {
	f := newFixture(t)
	inv := f.sentInvoice(t)

	note, err := f.svc.CreateCreditNote(context.Background(), inv.ID, f.userID, CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("CreateCreditNote: %v", err)
	}
	if note.CorrectsInvoiceID == nil || *note.CorrectsInvoiceID != inv.ID {
		t.Error("the credit note does not point at the invoice it cancels")
	}
	if !strings.HasPrefix(note.InvoiceNumber, "NC-") {
		t.Errorf("number = %q, want an NC- number", note.InvoiceNumber)
	}
	if note.TotalAmount != inv.TotalAmount {
		t.Errorf("full credit = %.2f, want %.2f", note.TotalAmount, inv.TotalAmount)
	}
	// The invoice itself is untouched: a correction adds a document, it does
	// not rewrite the one being corrected.
	var status string
	if err := f.db.QueryRow(`SELECT status FROM invoices WHERE id = ?`, inv.ID).Scan(&status); err != nil {
		t.Fatalf("reload invoice: %v", err)
	}
	if status != string(models.InvoiceStatusSent) {
		t.Errorf("invoice status = %q, want sent", status)
	}
}

// The guard the link makes possible: nothing used to stop crediting more than
// was ever invoiced.
func TestCreditNotesCannotExceedTheInvoiceTotal(t *testing.T) {
	f := newFixture(t)
	inv := f.sentInvoice(t)
	ctx := context.Background()

	if _, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{}); err != nil {
		t.Fatalf("first credit note: %v", err)
	}
	_, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{})
	if !errors.Is(err, ErrCreditExceedsInvoice) {
		t.Errorf("crediting the invoice twice returned %v, want ErrCreditExceedsInvoice", err)
	}
}

// Partial credits must accumulate against the same ceiling, not each be
// measured against the full invoice on its own.
func TestPartialCreditsAccumulate(t *testing.T) {
	f := newFixture(t)
	inv := f.sentInvoice(t) // 1250.00 HT + 8.1% = 1351.25
	ctx := context.Background()

	half := []LineInput{{Description: "Retour partiel", Quantity: 1, UnitPrice: 600, VATRate: 8.1, Sequence: 1}}
	if _, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{Lines: half}); err != nil {
		t.Fatalf("first partial credit: %v", err)
	}
	if _, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{Lines: half}); err != nil {
		t.Fatalf("second partial credit should still fit: %v", err)
	}
	// A third would push the total past the invoice.
	_, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{Lines: half})
	if !errors.Is(err, ErrCreditExceedsInvoice) {
		t.Errorf("third partial credit returned %v, want ErrCreditExceedsInvoice", err)
	}
}

// A cancelled credit note credits nothing, so it must free its share again.
func TestCancelledCreditNoteFreesItsAmount(t *testing.T) {
	f := newFixture(t)
	inv := f.sentInvoice(t)
	ctx := context.Background()

	note, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("credit note: %v", err)
	}
	if err := f.svc.Transition(ctx, note.ID, models.InvoiceStatusCancelled); err != nil {
		t.Fatalf("cancel credit note: %v", err)
	}
	if _, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{}); err != nil {
		t.Errorf("after cancelling, the invoice should be creditable again: %v", err)
	}
}

func TestCreditNoteRejectsQuotesAndCreditNotes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	quote := f.sentQuote(t)
	if _, err := f.svc.CreateCreditNote(ctx, quote.ID, f.userID, CreateCreditNoteRequest{}); !errors.Is(err, ErrNotAnInvoice) {
		t.Errorf("crediting an offer returned %v, want ErrNotAnInvoice", err)
	}

	inv := f.sentInvoice(t)
	note, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{})
	if err != nil {
		t.Fatalf("credit note: %v", err)
	}
	if _, err := f.svc.CreateCreditNote(ctx, note.ID, f.userID, CreateCreditNoteRequest{}); !errors.Is(err, ErrNotAnInvoice) {
		t.Errorf("crediting a credit note returned %v, want ErrNotAnInvoice", err)
	}
}

// A draft was never issued and a cancelled invoice is already void — neither
// carries a VAT debt to correct.
func TestCreditNoteRejectsDraftAndCancelledInvoices(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	draft := f.create(t, DocumentTypeInvoice)
	if _, err := f.svc.CreateCreditNote(ctx, draft.ID, f.userID, CreateCreditNoteRequest{}); !errors.Is(err, ErrInvoiceNotCreditable) {
		t.Errorf("crediting a draft returned %v, want ErrInvoiceNotCreditable", err)
	}

	cancelled := f.create(t, DocumentTypeInvoice)
	if err := f.svc.Transition(ctx, cancelled.ID, models.InvoiceStatusCancelled); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := f.svc.CreateCreditNote(ctx, cancelled.ID, f.userID, CreateCreditNoteRequest{}); !errors.Is(err, ErrInvoiceNotCreditable) {
		t.Errorf("crediting a cancelled invoice returned %v, want ErrInvoiceNotCreditable", err)
	}
}

// A paid invoice is the ordinary case for a refund.
func TestPaidInvoiceCanBeCredited(t *testing.T) {
	f := newFixture(t)
	inv := f.sentInvoice(t)
	ctx := context.Background()
	if err := f.svc.Transition(ctx, inv.ID, models.InvoiceStatusPaid); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if _, err := f.svc.CreateCreditNote(ctx, inv.ID, f.userID, CreateCreditNoteRequest{}); err != nil {
		t.Errorf("a paid invoice must be creditable: %v", err)
	}
}
