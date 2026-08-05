package handlers

// Réinitialisation d'accès par l'administrateur.
//
// Le cas réel : quelqu'un a oublié son mot de passe, ou son téléphone est perdu
// avec l'application d'authentification dessus. Sans cette route, la seule issue
// serait de supprimer le compte — ce que le produit refuse, parce que les
// écritures portent l'identifiant de leur auteur et que l'effacer casserait la
// traçabilité du CO art. 957a al. 2 ch. 5.
//
// # Ce que « réinitialiser » fait exactement
//
// L'ancien mot de passe est REMPLACÉ, pas révélé : personne, pas même
// l'administrateur, ne peut lire celui qui existait. Le nouveau est temporaire
// et devra être changé à la connexion suivante, comme à la création d'un compte
// — il a circulé par un canal qui n'est pas fait pour ça, et tant qu'il vaut,
// une action tracée sous ce compte ne prouve pas qui l'a faite.
//
// Les sessions ouvertes tombent. Une réinitialisation sert souvent à reprendre
// la main sur un compte dont on craint qu'il ait fuité : laisser vivre les
// sessions existantes viderait le geste de son sens.
//
// # Ce qu'elle ne fait PAS
//
// Elle ne retire pas le second facteur. Un administrateur qui pourrait, d'un
// clic, remettre à zéro le mot de passe ET le second facteur d'un autre compte
// pourrait s'y substituer entièrement — le second facteur ne protégerait alors
// plus de rien face à lui. Le retrait du second facteur est une action séparée,
// tracée séparément, et qui dit ce qu'elle fait.
//
// # Pourquoi on ne se réinitialise pas soi-même
//
// Un administrateur connecté n'en a aucun besoin : il change son mot de passe.
// La seule chose que le geste apporterait est un mot de passe temporaire qui
// s'affiche à l'écran — c'est-à-dire une occasion de le laisser traîner.

import (
	"crypto/rand"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// ResetPassword POST /api/v1/users/:id/reset-password
func (h *UsersHandler) ResetPassword(c *gin.Context) {
	targetID := c.Param("id")

	actor := ""
	if claims := mw.GetClaims(c); claims != nil {
		actor = claims.UserID
	}
	if actor == targetID {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "réinitialisez plutôt votre propre mot de passe depuis votre compte : " +
				"cette action afficherait un mot de passe temporaire à l'écran sans rien " +
				"vous apporter"})
		return
	}

	// Le compte doit exister ET être actif. Réinitialiser l'accès d'un compte
	// désactivé donnerait un mot de passe utilisable à quelqu'un qui n'a plus le
	// droit d'entrer, et laisserait croire que l'accès a été rendu.
	var email string
	var active int
	sel := db.Rebind(`SELECT email, is_active FROM users WHERE id = ?`, h.usePostgres)
	if err := h.db.QueryRowContext(c.Request.Context(), sel, targetID).Scan(&email, &active); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "compte introuvable"})
		return
	}
	if active != 1 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "ce compte est désactivé : réactivez-le d'abord si vous voulez lui " +
				"rendre l'accès"})
		return
	}

	temp, err := newTemporaryPassword()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hash, err := security.HashPassword(temp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	upd := db.Rebind(`
		UPDATE users SET password_hash = ?, must_change_password = 1, updated_at = ?
		WHERE id = ?`, h.usePostgres)
	res, err := h.db.ExecContext(c.Request.Context(), upd, hash, time.Now().UTC(), targetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "compte introuvable"})
		return
	}

	// Les sessions ouvertes tombent : une réinitialisation sert souvent à
	// reprendre la main sur un compte qu'on craint compromis.
	revoke := db.Rebind(`DELETE FROM refresh_tokens WHERE user_id = ?`, h.usePostgres)
	if _, err := h.db.ExecContext(c.Request.Context(), revoke, targetID); err != nil {
		_ = err // non bloquant : le mot de passe est déjà remplacé
	}

	h.recordSecurityEvent(c, "user_password_reset", targetID, "")

	c.JSON(http.StatusOK, gin.H{
		"temporary_password": temp,
		"email":              email,
		"warning": "Ce mot de passe ne s'affichera plus. Transmettez-le de vive voix plutôt " +
			"que par message : il sera remplacé à la première connexion, mais tant qu'il " +
			"vaut, il ouvre le compte.",
		"message": "Accès réinitialisé. Les sessions ouvertes de ce compte sont fermées, et " +
			"la personne devra choisir son propre mot de passe avant de pouvoir faire quoi " +
			"que ce soit.",
	})
}

// RemoveMFA DELETE /api/v1/users/:id/mfa
//
// Le téléphone perdu, sans code de secours sous la main. Le geste est séparé de
// la réinitialisation du mot de passe, et tracé à part : d'un seul clic qui
// ferait les deux, un administrateur pourrait se substituer entièrement à
// n'importe quel compte.
func (h *UsersHandler) RemoveMFA(c *gin.Context) {
	targetID := c.Param("id")

	actor := ""
	if claims := mw.GetClaims(c); claims != nil {
		actor = claims.UserID
	}
	if actor == targetID {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "retirez votre propre second facteur depuis votre compte : le mot de " +
				"passe y est redemandé, ce qui est justement la protection à laquelle vous " +
				"renoncez"})
		return
	}

	if _, err := h.db.ExecContext(c.Request.Context(),
		db.Rebind(`DELETE FROM user_mfa WHERE user_id = ?`, h.usePostgres),
		targetID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if _, err := h.db.ExecContext(c.Request.Context(),
		db.Rebind(`DELETE FROM mfa_recovery_codes WHERE user_id = ?`, h.usePostgres),
		targetID); err != nil {
		_ = err
	}

	// Les sessions tombent aussi : sinon la session ouverte du compte
	// continuerait de valoir sans que rien ne réclame la nouvelle inscription.
	if _, err := h.db.ExecContext(c.Request.Context(),
		db.Rebind(`DELETE FROM refresh_tokens WHERE user_id = ?`, h.usePostgres),
		targetID); err != nil {
		_ = err
	}

	h.recordSecurityEvent(c, "user_mfa_removed", targetID, "")
	c.JSON(http.StatusOK, gin.H{
		"message": "Second facteur retiré. Si ce compte est administrateur, il devra en " +
			"inscrire un nouveau avant de pouvoir travailler.",
	})
}

// newTemporaryPassword tire un mot de passe temporaire qui satisfait déjà les
// règles imposées par ValidateUserPassword.
//
// # Pourquoi il est tiré ici et pas choisi par l'administrateur
//
// Un mot de passe temporaire choisi à la main est « Bienvenue2026 » — sur toutes
// les installations, pour tous les comptes. Celui-ci a près de 80 bits
// d'entropie et ne vaut que jusqu'à la première connexion.
//
// # L'alphabet
//
// Sans I, l, O, 0, 1 : ce mot de passe se dicte au téléphone ou se recopie
// depuis un écran, et la confusion entre un zéro et un O est la première cause
// d'échec. Les groupes séparés par un tiret se lisent, et le tiret compte comme
// caractère sans appartenir à aucune classe exigée.
func newTemporaryPassword() (string, error) {
	const (
		lower  = "abcdefghijkmnpqrstuvwxyz"
		upper  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
		digits = "23456789"
	)
	all := lower + upper + digits

	pick := func(set string) (byte, error) {
		b := make([]byte, 1)
		if _, err := rand.Read(b); err != nil {
			return 0, err
		}
		return set[int(b[0])%len(set)], nil
	}

	// Un caractère de chaque classe d'abord : le tirage uniforme peut, rarement,
	// ne produire aucune majuscule, et le mot de passe serait alors refusé par
	// la règle que le produit impose ailleurs.
	var out []byte
	for _, set := range []string{lower, upper, digits} {
		ch, err := pick(set)
		if err != nil {
			return "", err
		}
		out = append(out, ch)
	}
	for len(out) < 15 {
		ch, err := pick(all)
		if err != nil {
			return "", err
		}
		out = append(out, ch)
	}

	// Mélange de Fisher-Yates à l'aide du générateur cryptographique : sans lui,
	// les trois premiers caractères seraient toujours minuscule, majuscule,
	// chiffre — dans cet ordre, sur tous les mots de passe émis.
	for i := len(out) - 1; i > 0; i-- {
		b := make([]byte, 1)
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		j := int(b[0]) % (i + 1)
		out[i], out[j] = out[j], out[i]
	}

	s := string(out)
	grouped := s[:5] + "-" + s[5:10] + "-" + s[10:]

	// Ceinture et bretelles : le mot de passe émis doit satisfaire la règle que
	// le produit impose au titulaire. Si un jour l'une des deux change sans
	// l'autre, mieux vaut échouer ici que remettre un mot de passe que le
	// serveur refusera ensuite.
	if err := ValidateUserPassword(grouped); err != nil {
		return "", err
	}
	return grouped, nil
}

// tempPasswordAlphabetIsUnambiguous existe pour que le test puisse vérifier
// l'absence des caractères confondables sans dupliquer la liste.
var confusableCharacters = strings.Split("IlO01", "")
