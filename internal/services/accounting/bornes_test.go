package accounting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/compliance"
)

func ptr(v float64) *float64 { return &v }

// Audit 9, M-06 et A-05 : le service acceptait 100.0009 contre 100.00 (tolérance de
// 0,001), des montants à trois décimales, négatifs, de 1e300. Le journal est
// immuable : ce qui est comptabilisé ne se défait plus.
func TestValidateDoubleEntryExigeLEgaliteAuCentime(t *testing.T) {
	ok := func(d, c float64) []LineInput {
		return []LineInput{{AccountID: "a", DebitAmount: ptr(d)}, {AccountID: "b", CreditAmount: ptr(c)}}
	}
	if err := validateDoubleEntry(ok(100, 100)); err != nil {
		t.Fatalf("écriture équilibrée refusée: %v", err)
	}
	for _, c := range []struct {
		nom  string
		deb  float64
		cred float64
	}{
		{"0.0009 d'écart", 100.0009, 100},
		{"0.001 d'écart", 100.001, 100},
		{"0.01 d'écart", 100.01, 100},
	} {
		if err := validateDoubleEntry(ok(c.deb, c.cred)); err == nil {
			t.Errorf("%s : écriture déséquilibrée acceptée", c.nom)
		}
	}
}

func TestValidateDoubleEntryRefuseLesMontantsAbsurdes(t *testing.T) {
	for _, c := range []struct {
		nom string
		v   float64
	}{
		{"trois décimales", 100.123},
		{"négatif", -50},
		{"1e15", 1e15},
		{"1e300", 1e300},
	} {
		lines := []LineInput{{AccountID: "a", DebitAmount: ptr(c.v)}, {AccountID: "b", CreditAmount: ptr(c.v)}}
		if err := validateDoubleEntry(lines); err == nil {
			t.Errorf("%s : accepté", c.nom)
		}
	}
	both := []LineInput{{AccountID: "a", DebitAmount: ptr(10), CreditAmount: ptr(10)}, {AccountID: "b", CreditAmount: ptr(0)}}
	if err := validateDoubleEntry(both); err == nil {
		t.Error("une ligne débitée ET créditée est acceptée")
	}
	zero := []LineInput{{AccountID: "a", DebitAmount: ptr(0)}, {AccountID: "b", CreditAmount: ptr(0)}}
	if err := validateDoubleEntry(zero); err == nil {
		t.Error("des lignes à zéro sont acceptées")
	}
}

// Audit 9, A-03 : l'exercice se crée d'après la date, sans borne.
func TestEnsureFiscalPeriodRefuseLesDatesAbsurdes(t *testing.T) {
	_, database, _, _ := newAccountingDB(t)
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, d := range []time.Time{
		time.Date(26, 10, 30, 0, 0, 0, 0, time.UTC),
		time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1900, 5, 5, 0, 0, 0, 0, time.UTC),
		time.Date(2999, 6, 1, 0, 0, 0, 0, time.UTC),
		time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		_, err := EnsureFiscalPeriod(context.Background(), tx, false, d)
		var hors compliance.ErrDateHorsBornes
		if !errors.As(err, &hors) {
			t.Errorf("%s : %v, attendu ErrDateHorsBornes", d.Format("2006-01-02"), err)
		}
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM fiscal_years`).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d exercice(s) créé(s) par des dates absurdes (%v)", n, err)
	}
	if _, err := EnsureFiscalPeriod(context.Background(), tx, false, time.Now()); err != nil {
		t.Errorf("la date du jour est refusée: %v", err)
	}
}
