package iso20022

// Le relevé camt.053 est ce qui fait ENTRER un encaissement dans les livres :
// une écriture au journal se rapproche d'une ligne parsée ici. Rien n'appelait
// ParseCamt053 avant ces tests. La forme est celle publiée par les banques
// suisses (SIX Interbank Clearing) sur un relevé réel, réduite aux éléments
// que le code lit.

import (
	"strings"
	"testing"
)

// releveSquelette produit un camt.053 minimal portant les <Ntry> donnés en
// paramètre, pour ne pas répéter l'enveloppe (Document/BkToCstmrStmt/Stmt)
// dans chaque test.
func releveSquelette(entrees string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08">
  <BkToCstmrStmt>
    <GrpHdr>
      <MsgId>RELEVE-2026-03-01</MsgId>
      <CreDtTm>2026-03-01T06:00:00</CreDtTm>
    </GrpHdr>
    <Stmt>
      <Id>STMT-0001</Id>
      <Acct><Id><IBAN>CH9300762011623852957</IBAN></Id></Acct>
      ` + entrees + `
    </Stmt>
  </BkToCstmrStmt>
</Document>`)
}

// Une écriture BOOK complète, en crédit : un client qui paie une facture par
// virement QR, avec référence structurée. C'est le cas le plus fréquent que
// le rapprochement bancaire doit reconnaître.
const entreeCreditQR = `<Ntry>
        <Amt Ccy="CHF">1250.50</Amt>
        <CdtDbtInd>CRDT</CdtDbtInd>
        <Sts><Cd>BOOK</Cd></Sts>
        <BookgDt><Dt>2026-03-01</Dt></BookgDt>
        <ValDt><Dt>2026-03-01</Dt></ValDt>
        <AcctSvcrRef>REF-BANQUE-001</AcctSvcrRef>
        <NtryDtls>
          <TxDtls>
            <Refs>
              <EndToEndId>FACT-2026-042</EndToEndId>
              <AcctSvcrRef>REF-BANQUE-001</AcctSvcrRef>
            </Refs>
            <RmtInf>
              <Strd>
                <CdtrRefInf><Ref>210000000003139471430009017</Ref></CdtrRefInf>
              </Strd>
            </RmtInf>
            <RltdPties>
              <Dbtr><Pty><Nm>Client Genève SA</Nm></Pty></Dbtr>
              <DbtrAcct><Id><IBAN>CH56 0483 5012 3456 7800 9</IBAN></Id></DbtrAcct>
            </RltdPties>
          </TxDtls>
        </NtryDtls>
      </Ntry>`

func TestParseCamt053EcritureCreditAvecReferenceQR(t *testing.T) {
	entries, err := ParseCamt053(releveSquelette(entreeCreditQR))
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d entrée(s), attendu 1", len(entries))
	}
	e := entries[0]

	if e.Amount != 1250.50 {
		t.Errorf("montant = %v, attendu 1250.50", e.Amount)
	}
	if e.Currency != "CHF" {
		t.Errorf("devise = %q, attendu CHF", e.Currency)
	}
	if !e.IsCredit {
		t.Error("IsCredit = false, attendu true (CdtDbtInd = CRDT)")
	}
	if e.BankRef != "REF-BANQUE-001" {
		t.Errorf("référence bancaire = %q", e.BankRef)
	}
	if e.EndToEndRef != "FACT-2026-042" {
		t.Errorf("EndToEndId = %q", e.EndToEndRef)
	}
	if e.QRReference != "210000000003139471430009017" {
		t.Errorf("référence QR = %q", e.QRReference)
	}
	if e.BookingDate.Format("2006-01-02") != "2026-03-01" {
		t.Errorf("date de comptabilisation = %v", e.BookingDate)
	}
	if e.ValueDate.Format("2006-01-02") != "2026-03-01" {
		t.Errorf("date de valeur = %v", e.ValueDate)
	}

	// En crédit, le CONTREPARTIE est le débiteur — celui qui a payé — et non
	// le créancier, qui est le titulaire du compte relevé. Se tromper de côté
	// afficherait le nom du titulaire lui-même comme expéditeur du paiement.
	if e.CounterpartName != "Client Genève SA" {
		t.Errorf("nom de la contrepartie = %q, attendu le débiteur", e.CounterpartName)
	}
	if e.CounterpartIBAN != "CH5604835012345678009" {
		t.Errorf("IBAN de la contrepartie = %q (espaces non retirés ?)", e.CounterpartIBAN)
	}
}

// Une écriture au débit, sans référence structurée — un virement fournisseur
// ordinaire, avec seulement un motif en texte libre.
const entreeDebitLibre = `<Ntry>
        <Amt Ccy="CHF">890.00</Amt>
        <CdtDbtInd>DBIT</CdtDbtInd>
        <Sts><Cd>BOOK</Cd></Sts>
        <BookgDt><Dt>2026-03-02</Dt></BookgDt>
        <ValDt><Dt>2026-03-02</Dt></ValDt>
        <AcctSvcrRef>REF-BANQUE-002</AcctSvcrRef>
        <NtryDtls>
          <TxDtls>
            <Refs><EndToEndId>PAIE-2026-018</EndToEndId></Refs>
            <RmtInf><Ustrd>Facture fournisseur mars 2026</Ustrd></RmtInf>
            <RltdPties>
              <Cdtr><Pty><Nm>Bois Suisse SA</Nm></Pty></Cdtr>
              <CdtrAcct><Id><IBAN>CH1234567890123456789</IBAN></Id></CdtrAcct>
            </RltdPties>
          </TxDtls>
        </NtryDtls>
      </Ntry>`

func TestParseCamt053EcritureDebitAvecMotifLibre(t *testing.T) {
	entries, err := ParseCamt053(releveSquelette(entreeDebitLibre))
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d entrée(s), attendu 1", len(entries))
	}
	e := entries[0]

	if e.IsCredit {
		t.Error("IsCredit = true, attendu false (CdtDbtInd = DBIT)")
	}
	if e.Unstructured != "Facture fournisseur mars 2026" {
		t.Errorf("motif libre = %q", e.Unstructured)
	}
	if e.QRReference != "" {
		t.Errorf("référence QR = %q, attendu vide (aucune remise structurée)", e.QRReference)
	}

	// En débit, la contrepartie est le CRÉANCIER — celui qui a été payé.
	if e.CounterpartName != "Bois Suisse SA" {
		t.Errorf("nom de la contrepartie = %q, attendu le créancier", e.CounterpartName)
	}
	if e.CounterpartIBAN != "CH1234567890123456789" {
		t.Errorf("IBAN de la contrepartie = %q", e.CounterpartIBAN)
	}
}

// Une écriture PDNG (en attente) ne doit PAS entrer dans les livres : elle
// peut encore être annulée par la banque, et la comptabiliser reviendrait à
// enregistrer un encaissement qui n'a pas eu lieu.
func TestParseCamt053IgnoreLesEcrituresNonComptabilisees(t *testing.T) {
	entreeEnAttente := strings.Replace(entreeCreditQR, "<Sts><Cd>BOOK</Cd></Sts>", "<Sts><Cd>PDNG</Cd></Sts>", 1)
	entries, err := ParseCamt053(releveSquelette(entreeEnAttente))
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("%d entrée(s) retenue(s) pour une écriture PDNG, attendu 0 : %+v", len(entries), entries)
	}
}

// Un relevé qui mélange une écriture en attente et deux écritures
// comptabilisées ne doit retenir QUE les deux comptabilisées, dans leur ordre
// d'apparition — le tri est ce sur quoi le rapprochement s'appuie ensuite.
func TestParseCamt053MelangeEcrituresRetientSeulementBOOK(t *testing.T) {
	enAttente := strings.Replace(entreeCreditQR, "<Sts><Cd>BOOK</Cd></Sts>", "<Sts><Cd>PDNG</Cd></Sts>", 1)
	releve := releveSquelette(enAttente + entreeDebitLibre + entreeCreditQR)

	entries, err := ParseCamt053(releve)
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("%d entrée(s), attendu 2 (l'écriture PDNG doit être écartée)", len(entries))
	}
	if entries[0].IsCredit {
		t.Error("la première entrée devrait être le débit (ordre d'apparition)")
	}
	if !entries[1].IsCredit {
		t.Error("la seconde entrée devrait être le crédit (ordre d'apparition)")
	}
}

// Un montant illisible ne doit pas faire échouer tout le relevé — une seule
// ligne malformée ne doit pas priver le comptable des cent autres du même
// fichier — mais elle ne doit pas non plus entrer dans les livres avec une
// valeur inventée.
func TestParseCamt053IgnoreUneEcritureAuMontantIllisible(t *testing.T) {
	entreeCassee := strings.Replace(entreeCreditQR, `<Amt Ccy="CHF">1250.50</Amt>`, `<Amt Ccy="CHF">n/a</Amt>`, 1)
	releve := releveSquelette(entreeCassee + entreeDebitLibre)

	entries, err := ParseCamt053(releve)
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("%d entrée(s), attendu 1 (seule l'écriture au montant lisible)", len(entries))
	}
	if entries[0].BankRef != "REF-BANQUE-002" {
		t.Errorf("l'entrée retenue n'est pas la bonne : %+v", entries[0])
	}
}

// Un relevé sans aucune écriture est un résultat valide — un compte inactif
// pendant la période — et non une erreur.
func TestParseCamt053ReleveSansEcritureNeRendPasDErreur(t *testing.T) {
	entries, err := ParseCamt053(releveSquelette(""))
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("%d entrée(s) pour un relevé vide, attendu 0", len(entries))
	}
}

// Un document qui n'est pas du XML doit être signalé comme une erreur, pas
// silencieusement rendre zéro écriture — les deux situations ne demandent pas
// la même réaction : « rien à importer » contre « ce fichier n'est pas ce
// qu'on attend ».
func TestParseCamt053RefuseUnDocumentIllisible(t *testing.T) {
	if _, err := ParseCamt053([]byte("ceci n'est pas du XML")); err == nil {
		t.Fatal("aucune erreur pour un document qui n'est pas du XML")
	}
}

// Plusieurs <Stmt> dans un même document — un relevé multi-comptes, publié par
// certaines banques dans un seul fichier — doivent tous être parcourus.
func TestParseCamt053PlusieursReleves(t *testing.T) {
	doc := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<Document xmlns="urn:iso:std:iso:20022:tech:xsd:camt.053.001.08">
  <BkToCstmrStmt>
    <GrpHdr><MsgId>M</MsgId><CreDtTm>2026-03-01T06:00:00</CreDtTm></GrpHdr>
    <Stmt>
      <Id>STMT-A</Id>
      <Acct><Id><IBAN>CH9300762011623852957</IBAN></Id></Acct>
      ` + entreeCreditQR + `
    </Stmt>
    <Stmt>
      <Id>STMT-B</Id>
      <Acct><Id><IBAN>CH1234567890123456789</IBAN></Id></Acct>
      ` + entreeDebitLibre + `
    </Stmt>
  </BkToCstmrStmt>
</Document>`)

	entries, err := ParseCamt053(doc)
	if err != nil {
		t.Fatalf("ParseCamt053: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("%d entrée(s) sur deux relevés, attendu 2", len(entries))
	}
}

// La date de comptabilisation se lit au format court (AAAA-MM-JJ) comme au
// format horodaté — certaines banques envoient l'un, certaines l'autre — et
// une date absente ou illisible laisse simplement le champ à zéro plutôt que
// de faire échouer l'écriture entière.
func TestParseDateFormatsAcceptesEtDateAbsente(t *testing.T) {
	cas := []struct {
		nom      string
		entrée   string
		attendue string // "" si on attend une date zéro
	}{
		{"date courte", "2026-03-01", "2026-03-01"},
		{"date-heure sans fuseau", "2026-03-01T14:30:00", "2026-03-01"},
		{"date-heure UTC", "2026-03-01T14:30:00Z", "2026-03-01"},
		{"absente", "", ""},
		{"illisible", "pas une date", ""},
	}
	for _, c := range cas {
		t.Run(c.nom, func(t *testing.T) {
			got := parseDate(c.entrée)
			if c.attendue == "" {
				if !got.IsZero() {
					t.Errorf("parseDate(%q) = %v, attendu une date zéro", c.entrée, got)
				}
				return
			}
			if got.Format("2006-01-02") != c.attendue {
				t.Errorf("parseDate(%q) = %v, attendu %s", c.entrée, got, c.attendue)
			}
		})
	}
}
