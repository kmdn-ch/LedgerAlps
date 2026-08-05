package handlers

// Changement du mot de passe temporaire.
//
// Un administrateur qui crée un compte choisit un mot de passe pour quelqu'un
// d'autre, puis le lui transmet — par message, par téléphone, sur un papier. Ce
// mot de passe est donc connu d'au moins deux personnes et a voyagé par un
// canal qui n'est pas fait pour ça.
//
// Tant qu'il n'est pas remplacé, l'administrateur peut se connecter au nom de
// l'autre. Les actions seraient alors tracées sous un compte qui n'est pas
// celui de leur auteur réel — ce qui vide de son sens la traçabilité que le
// CO art. 957a al. 2 ch. 5 exige, et rend le journal d'audit trompeur plutôt
// qu'incomplet.
//
// # Le blocage est technique, pas cosmétique
//
// Un compte marqué ne peut RIEN faire d'autre que changer son mot de passe.
// Toutes les routes de l'API le refusent, y compris en lecture — cacher les
// écrans n'aurait fermé aucune porte, l'adresse restant tapable et l'appel
// réseau restant faisable.
//
// # L'exigence de robustesse
//
// La même que pour les phrases de passe des sauvegardes : douze caractères,
// minuscule, majuscule, chiffre. Plus court que le seuil des sauvegardes — une
// connexion est derrière une limitation de tentatives et un verrouillage, une
// sauvegarde ne l'est pas — mais bien au-delà des huit caractères acceptés à
// l'installation, parce qu'ici c'est le logiciel qui impose, pas l'utilisateur
// qui choisit.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// MinUserPasswordLength est le plancher d'un mot de passe de connexion choisi
// par son titulaire.
const MinUserPasswordLength = 12

// ValidateUserPassword dit pourquoi un mot de passe est trop faible, ou rien.
//
// Le message nomme ce qui manque. « Mot de passe trop faible » oblige à
// deviner, et la plupart des gens devinent en ajoutant un chiffre à la fin.
func ValidateUserPassword(p string) error {
	var lower, upper, digit bool
	for _, r := range p {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	var missing []string
	if len([]rune(p)) < MinUserPasswordLength {
		missing = append(missing, fmt.Sprintf("%d caractères au minimum", MinUserPasswordLength))
	}
	if !lower {
		missing = append(missing, "une minuscule")
	}
	if !upper {
		missing = append(missing, "une majuscule")
	}
	if !digit {
		missing = append(missing, "un chiffre")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("il manque : %s", strings.Join(missing, ", "))
}

// ChangePassword POST /api/v1/auth/change-password
//
// Accessible à un compte marqué « doit changer son mot de passe » — c'est même
// la seule chose qu'il puisse faire — comme à un compte ordinaire qui veut
// changer le sien.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var body struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	claims := mw.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session invalide"})
		return
	}

	ctx := c.Request.Context()
	var hash string
	q := db.Rebind(`SELECT password_hash FROM users WHERE id = ? AND is_active = 1`, h.cfg.UsePostgres())
	if err := h.db.QueryRowContext(ctx, q, claims.UserID).Scan(&hash); err == sql.ErrNoRows {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ce compte n'est plus actif"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// L'ancien mot de passe est vérifié même sur un compte marqué : sans cela,
	// un jeton volé suffirait à s'approprier définitivement le compte.
	if !security.CheckPassword(hash, body.CurrentPassword) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "le mot de passe actuel est incorrect"})
		return
	}
	if body.NewPassword == body.CurrentPassword {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "le nouveau mot de passe doit être différent de l'actuel : " +
				"c'est justement celui que quelqu'un d'autre connaît"})
		return
	}
	if err := ValidateUserPassword(body.NewPassword); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "mot de passe trop faible — " + err.Error()})
		return
	}

	newHash, err := security.HashPassword(body.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	upd := db.Rebind(`
		UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ?
		WHERE id = ?`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(ctx, upd, newHash, time.Now().UTC(), claims.UserID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	// Les autres sessions du compte tombent. Le mot de passe temporaire a
	// circulé ; si quelqu'un s'en était servi, sa session doit mourir avec lui.
	revoke := db.Rebind(`DELETE FROM refresh_tokens WHERE user_id = ?`, h.cfg.UsePostgres())
	if _, err := h.db.ExecContext(ctx, revoke, claims.UserID); err != nil {
		_ = err // non bloquant : le mot de passe est déjà changé
	}

	recordSecurityEvent(ctx, h.db, h.cfg.UsePostgres(),
		"password_changed", c.ClientIP(), "compte="+claims.UserID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Mot de passe changé. Les autres sessions de ce compte ont été fermées.",
	})
}

// recordSecurityEvent trace un événement de sécurité sans faire échouer
// l'action qu'il accompagne : la journalisation ne doit pas devenir un moyen de
// bloquer un changement de mot de passe.
func recordSecurityEvent(_ context.Context, database *sql.DB,
	usePostgres bool, eventType, ip, detail string) {
	q := db.Rebind(`
		INSERT INTO security_events (id, event_type, ip_address, detail, created_at)
		VALUES (?, ?, ?, ?, ?)`, usePostgres)
	_, _ = database.Exec(q, db.NewID(), eventType, ip, detail, time.Now().UTC())
}
