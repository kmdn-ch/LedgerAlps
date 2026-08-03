package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ─── Harnais ─────────────────────────────────────────────────────────────────

func newAuditDB(t *testing.T) (*AuditHandler, *sql.DB) {
	t.Helper()
	tmp, err := os.CreateTemp("", "ledgeralps-audit-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	cfg := &config.Config{SQLitePath: tmp.Name(), Host: "127.0.0.1"}
	database, err := db.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := db.Migrate(database, false); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// audit_logs.user_id porte une clé étrangère vers users : une chaîne de
	// test doit donc reposer sur un utilisateur réel, comme en production.
	if _, err := database.Exec(
		`INSERT INTO users (id, email, name, password_hash, is_admin) VALUES (?, ?, ?, ?, ?)`,
		"u1", "audit@example.test", "Comptable de test", "x", true,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return NewAuditHandler(database, false), database
}

// seedChain écrit n entrées valablement chaînées, exactement comme le fait le
// service de comptabilité à la comptabilisation d'une écriture.
func seedChain(t *testing.T, database *sql.DB, n int) {
	t.Helper()
	prev := ""
	base := time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)

	for i := 1; i <= n; i++ {
		var (
			id        = fmt.Sprintf("a%03d", i)
			recordID  = fmt.Sprintf("entry-%03d", i)
			after     = fmt.Sprintf(`{"entry_id":%q,"status":"posted"}`, recordID)
			createdAt = base.Add(time.Duration(i) * time.Hour)
		)
		entryHash := security.ComputeEntryHash(
			"u1", "post", "journal_entries", recordID, "", after, "127.0.0.1", createdAt,
		)
		var prevPtr *string
		if prev != "" {
			p := prev
			prevPtr = &p
		}
		if _, err := database.Exec(`
			INSERT INTO audit_logs
			    (id, user_id, action, table_name, record_id,
			     before_state, after_state, ip_address,
			     entry_hash, prev_hash, sequence_number, created_at, hash_version)
			VALUES (?, 'u1', 'post', 'journal_entries', ?, NULL, ?, '127.0.0.1', ?, ?, ?, ?, 2)`,
			id, recordID, after, entryHash, prevPtr, i, createdAt,
		); err != nil {
			t.Fatalf("seed entry %d: %v", i, err)
		}
		prev = entryHash
	}
}

func runChainVerify(t *testing.T, h *AuditHandler) (int, ChainReport) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/verify-chain", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.VerifyAuditChain(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/verify-chain", nil))

	var rep ChainReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatalf("invalid JSON (%d): %s", w.Code, w.Body.String())
	}
	return w.Code, rep
}

func breakKinds(rep ChainReport) map[string]bool {
	kinds := map[string]bool{}
	for _, b := range rep.Breaks {
		kinds[b.Kind] = true
	}
	return kinds
}

// ─── Chaîne intacte ──────────────────────────────────────────────────────────

func TestVerifyChainAcceptsAnIntactChain(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 5)

	code, rep := runChainVerify(t, h)
	if code != http.StatusOK || !rep.Verified {
		t.Fatalf("chaîne intacte refusée : code=%d verified=%v breaks=%+v", code, rep.Verified, rep.Breaks)
	}
	if rep.Entries != 5 || rep.FirstSeq != 1 || rep.LastSeq != 5 {
		t.Fatalf("comptage = %d entrées, séquence %d→%d ; attendu 5, 1→5", rep.Entries, rep.FirstSeq, rep.LastSeq)
	}
}

// Une base vide est une chaîne valide : aucune écriture n'a encore été
// comptabilisée. Signaler une rupture ici apprendrait à ignorer l'écran.
func TestVerifyChainAcceptsAnEmptyChain(t *testing.T) {
	h, _ := newAuditDB(t)

	code, rep := runChainVerify(t, h)
	if code != http.StatusOK || !rep.Verified || rep.Entries != 0 {
		t.Fatalf("base vide signalée comme rompue : code=%d %+v", code, rep)
	}
}

// ─── Altérations ─────────────────────────────────────────────────────────────

// Modifier le contenu d'une entrée : c'est le seul cas que la vérification
// entrée par entrée détecte déjà.
func TestVerifyChainDetectsAnAlteredEntry(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 5)

	if _, err := database.Exec(
		`UPDATE audit_logs SET after_state = ? WHERE sequence_number = 3`,
		`{"entry_id":"entry-003","status":"draft"}`,
	); err != nil {
		t.Fatal(err)
	}

	code, rep := runChainVerify(t, h)
	if code != http.StatusConflict || rep.Verified {
		t.Fatalf("contenu modifié non détecté : code=%d %+v", code, rep)
	}
	if !breakKinds(rep)["entry_altered"] {
		t.Fatalf("type de rupture attendu entry_altered, obtenu %+v", rep.Breaks)
	}
}

// Supprimer une entrée au milieu. C'est le cas qui justifie ce point d'entrée :
// l'empreinte propre de chaque entrée restante reste parfaitement valide, donc
// vérifier les entrées une à une ne verrait rien.
func TestVerifyChainDetectsADeletedEntry(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 5)

	if _, err := database.Exec(`DELETE FROM audit_logs WHERE sequence_number = 3`); err != nil {
		t.Fatal(err)
	}

	// D'abord la démonstration : chaque entrée survivante se vérifie seule.
	rows, err := database.Query(`SELECT id FROM audit_logs ORDER BY sequence_number`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()

	gin.SetMode(gin.TestMode)
	single := gin.New()
	single.GET("/:id/verify", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.VerifyAuditLog(c)
	})
	for _, id := range ids {
		w := httptest.NewRecorder()
		single.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/"+id+"/verify", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("l'entrée %s devrait se vérifier seule : %d %s", id, w.Code, w.Body.String())
		}
	}

	// La chaîne, elle, doit voir le trou.
	code, rep := runChainVerify(t, h)
	if code != http.StatusConflict || rep.Verified {
		t.Fatalf("suppression non détectée : code=%d %+v", code, rep)
	}
	kinds := breakKinds(rep)
	if !kinds["sequence_gap"] || !kinds["link_broken"] {
		t.Fatalf("attendu sequence_gap et link_broken, obtenu %+v", rep.Breaks)
	}
}

// Supprimer le DÉBUT de la chaîne. Toutes les entrées restantes se chaînent
// correctement entre elles : seuls le prev_hash orphelin de la nouvelle
// première entrée et son numéro de séquence trahissent la troncature.
func TestVerifyChainDetectsATruncatedHead(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 5)

	if _, err := database.Exec(`DELETE FROM audit_logs WHERE sequence_number <= 2`); err != nil {
		t.Fatal(err)
	}

	code, rep := runChainVerify(t, h)
	if code != http.StatusConflict || rep.Verified {
		t.Fatalf("troncature du début non détectée : code=%d %+v", code, rep)
	}
	if !breakKinds(rep)["anchor_invalid"] {
		t.Fatalf("attendu anchor_invalid, obtenu %+v", rep.Breaks)
	}
}

// Supprimer la FIN de la chaîne reste invisible, et doit le rester : rien dans
// une chaîne d'empreintes ne distingue « les trois dernières écritures ont été
// effacées » de « il n'y a pas encore eu de quatrième écriture ». C'est la
// sauvegarde qui répond à cette question, pas la chaîne — l'écran doit donc se
// garder d'affirmer que la comptabilité est complète.
func TestVerifyChainCannotDetectATruncatedTail(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 5)

	if _, err := database.Exec(`DELETE FROM audit_logs WHERE sequence_number > 2`); err != nil {
		t.Fatal(err)
	}

	code, rep := runChainVerify(t, h)
	if code != http.StatusOK || !rep.Verified {
		t.Fatalf("une chaîne tronquée en fin reste cohérente ; ce test documente la limite : %d %+v", code, rep)
	}
	if rep.LastSeq != 2 {
		t.Fatalf("dernier numéro = %d, attendu 2", rep.LastSeq)
	}
}

// ─── Entrées antérieures au correctif d'empreinte ────────────────────────────

// Une base écrite avant la v1.4.6 contient des entrées dont l'empreinte propre
// ne peut pas être recalculée : les valeurs ayant servi au calcul n'ont pas été
// persistées. Les compter comme rompues afficherait « vos livres ont été
// altérés » à un utilisateur qui n'a rien fait — l'avertissement le plus cher
// qui soit, puisqu'il détruit la crédibilité de tous les suivants.
func TestVerifyChainDoesNotAccuseLegacyEntries(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 4)
	if _, err := database.Exec(`UPDATE audit_logs SET hash_version = 1`); err != nil {
		t.Fatal(err)
	}

	code, rep := runChainVerify(t, h)
	if code != http.StatusOK || !rep.Verified {
		t.Fatalf("des entrées anciennes sont présentées comme altérées : code=%d %+v", code, rep.Breaks)
	}
	if rep.Legacy != 4 {
		t.Fatalf("legacy_entries = %d, attendu 4 — l'écran doit pouvoir le dire", rep.Legacy)
	}
}

// Ce que les entrées anciennes conservent : leur chaînage. Une suppression doit
// donc rester détectable même sur une base entièrement en ancien format, sans
// quoi la tolérance ci-dessus reviendrait à ne plus rien vérifier du tout.
func TestVerifyChainStillDetectsDeletionAmongLegacyEntries(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 5)
	if _, err := database.Exec(`UPDATE audit_logs SET hash_version = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM audit_logs WHERE sequence_number = 3`); err != nil {
		t.Fatal(err)
	}

	code, rep := runChainVerify(t, h)
	if code != http.StatusConflict || rep.Verified {
		t.Fatalf("suppression invisible sur une base ancienne : code=%d %+v", code, rep)
	}
	kinds := breakKinds(rep)
	if !kinds["sequence_gap"] || !kinds["link_broken"] {
		t.Fatalf("attendu sequence_gap et link_broken, obtenu %+v", rep.Breaks)
	}
}

// La vérification d'une entrée isolée doit répondre « non vérifiable », pas
// « intégrité en échec ». Un 409 ici serait une accusation.
func TestVerifySingleLegacyEntryReportsNotVerifiable(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 1)
	if _, err := database.Exec(`UPDATE audit_logs SET hash_version = 1`); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/:id/verify", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.VerifyAuditLog(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/a001/verify", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, attendu 200 : une entrée non recalculable n'est pas une entrée corrompue — %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if v, ok := body["verifiable"].(bool); !ok || v {
		t.Fatalf("réponse = %v, attendu verifiable=false", body)
	}
	if _, accuses := body["recomputed_hash"]; accuses {
		t.Fatalf("la réponse compare des empreintes alors qu'elle ne le peut pas : %v", body)
	}
}

// ─── Accès ───────────────────────────────────────────────────────────────────

func TestVerifyChainRequiresAdmin(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 2)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/verify-chain", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: false})
		h.VerifyAuditChain(c)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/verify-chain", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("un non-administrateur a obtenu %d : %s", w.Code, w.Body.String())
	}
}

// ─── Liste ───────────────────────────────────────────────────────────────────

// L'ordre par défaut reste croissant : c'est celui dans lequel la chaîne a été
// écrite, et le seul qui ait un sens pour une lecture d'intégrité. L'écran
// demande explicitement l'ordre inverse.
func TestListAuditLogsOrder(t *testing.T) {
	h, database := newAuditDB(t)
	seedChain(t, database, 4)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/audit-logs", func(c *gin.Context) {
		c.Set("claims", &security.Claims{UserID: "u1", IsAdmin: true})
		h.ListAuditLogs(c)
	})

	seqOf := func(query string) []float64 {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/audit-logs"+query, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d : %s", w.Code, w.Body.String())
		}
		var body struct {
			Items []struct {
				SequenceNumber float64 `json:"sequence_number"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if body.Total != 4 {
			t.Fatalf("total = %d, attendu 4", body.Total)
		}
		out := make([]float64, len(body.Items))
		for i, it := range body.Items {
			out[i] = it.SequenceNumber
		}
		return out
	}

	if got := seqOf(""); got[0] != 1 || got[3] != 4 {
		t.Fatalf("ordre par défaut = %v, attendu croissant", got)
	}
	if got := seqOf("?order=desc"); got[0] != 4 || got[3] != 1 {
		t.Fatalf("order=desc = %v, attendu décroissant", got)
	}
	// Le paramètre est concaténé dans le SQL : une valeur inconnue doit être
	// ramenée au défaut, jamais transmise.
	if got := seqOf("?order=" + url.QueryEscape("DESC; DROP TABLE audit_logs--")); got[0] != 1 {
		t.Fatalf("valeur inconnue non ramenée au défaut : %v", got)
	}
	var still int
	if err := database.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&still); err != nil || still != 4 {
		t.Fatalf("la table a souffert : %d entrées, err=%v", still, err)
	}
}
