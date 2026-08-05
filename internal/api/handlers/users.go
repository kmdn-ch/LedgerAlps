package handlers

// Gestion des comptes et des rôles.
//
// Le cas central : donner un accès à sa fiduciaire sans lui donner les clés.
//
// # Les trois refus qui comptent
//
// **On ne retire pas le dernier administrateur.** Ni en le rétrogradant, ni en
// le désactivant, ni en le supprimant. Sans cette règle, un clic suffit à
// rendre l'installation inadministrable — plus personne ne peut créer de compte,
// restaurer une sauvegarde, ni rendre à quiconque le droit de le faire. Il n'y
// a pas de « mot de passe administrateur » derrière pour rattraper.
//
// **On ne change pas son propre rôle.** Pas parce qu'un administrateur pourrait
// s'élever — il est déjà au maximum — mais parce qu'il peut se rétrograder par
// mégarde et perdre l'accès. La règle du dernier administrateur ne couvre pas le
// cas où il en reste deux et que celui qui clique se coupe l'herbe sous le pied.
//
// **On ne supprime pas un compte, on le désactive.** Les écritures et les
// documents portent l'identifiant de leur auteur : supprimer la ligne casserait
// la traçabilité que le CO art. 957a al. 2 ch. 5 exige. Un compte désactivé ne
// peut plus rien faire — le contrôle des droits lit son état à chaque requête —
// mais son nom reste lisible sur ce qu'il a écrit.

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

type UsersHandler struct {
	db          *sql.DB
	usePostgres bool
}

func NewUsersHandler(database *sql.DB, usePostgres bool) *UsersHandler {
	return &UsersHandler{db: database, usePostgres: usePostgres}
}

type userView struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	RoleLabel string    `json:"role_label"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// ListUsers GET /api/v1/users
func (h *UsersHandler) ListUsers(c *gin.Context) {
	q := db.Rebind(`SELECT id, email, name, role, is_active, created_at
	                FROM users ORDER BY created_at`, h.usePostgres)
	rows, err := h.db.QueryContext(c.Request.Context(), q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	defer rows.Close()

	items := []userView{}
	for rows.Next() {
		var u userView
		var active int
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &active, &u.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		u.IsActive = active == 1
		u.RoleLabel = authz.Role(u.Role).Label()
		items = append(items, u)
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// CreateUser POST /api/v1/users
func (h *UsersHandler) CreateUser(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required,email"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required"`
		Role     string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	role := authz.Role(body.Role)
	if !role.Valid() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "rôle inconnu"})
		return
	}
	// Le mot de passe d'un compte partagé avec un tiers mérite au moins la même
	// exigence que celui de l'administrateur.
	if len([]rune(body.Password)) < 8 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "le mot de passe doit contenir au moins 8 caractères"})
		return
	}

	hash, err := security.HashPassword(body.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}

	id := db.NewID()
	now := time.Now().UTC()
	// is_admin reste tenu à jour : une base restaurée dans une version
	// antérieure aux rôles doit rester administrable.
	q := db.Rebind(`
		INSERT INTO users (id, email, name, password_hash, role, is_admin, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`, h.usePostgres)
	if _, err := h.db.ExecContext(c.Request.Context(), q,
		id, strings.TrimSpace(body.Email), strings.TrimSpace(body.Name), hash,
		string(role), boolToSQL(role == authz.RoleAdmin), now, now); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "cette adresse e-mail est déjà utilisée"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	h.recordSecurityEvent(c, "user_created", id, "rôle "+string(role))
	c.JSON(http.StatusCreated, gin.H{
		"id": id, "email": body.Email, "name": body.Name,
		"role": string(role), "role_label": role.Label(), "is_active": true,
	})
}

// ErrLastAdmin est le refus qui protège l'installation d'elle-même.
var ErrLastAdmin = errors.New(
	"c'est le dernier administrateur : le retirer rendrait cette installation inadministrable, " +
		"sans possibilité de créer un compte, de restaurer une sauvegarde ou de rendre le droit de le faire")

// UpdateUserRole PUT /api/v1/users/:id/role
func (h *UsersHandler) UpdateUserRole(c *gin.Context) {
	targetID := c.Param("id")
	var body struct {
		Role string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	role := authz.Role(body.Role)
	if !role.Valid() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "rôle inconnu"})
		return
	}

	actor := ""
	if claims := mw.GetClaims(c); claims != nil {
		actor = claims.UserID
	}
	if actor == targetID {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "vous ne pouvez pas changer votre propre rôle : une rétrogradation par " +
				"mégarde vous couperait l'accès. Demandez à un autre administrateur."})
		return
	}

	// Rétrograder le dernier administrateur est refusé.
	if role != authz.RoleAdmin {
		last, err := h.isLastAdmin(c, targetID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if last {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": ErrLastAdmin.Error()})
			return
		}
	}

	q := db.Rebind(`UPDATE users SET role = ?, is_admin = ?, updated_at = ? WHERE id = ?`, h.usePostgres)
	res, err := h.db.ExecContext(c.Request.Context(), q,
		string(role), boolToSQL(role == authz.RoleAdmin), time.Now().UTC(), targetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "compte introuvable"})
		return
	}

	h.recordSecurityEvent(c, "user_role_changed", targetID, "nouveau rôle "+string(role))
	c.JSON(http.StatusOK, gin.H{
		"message": "Rôle mis à jour. Il s'applique immédiatement : les droits sont relus à " +
			"chaque requête, sans attendre l'expiration d'une session.",
		"role": string(role), "role_label": role.Label(),
	})
}

// SetUserActive PUT /api/v1/users/:id/active
func (h *UsersHandler) SetUserActive(c *gin.Context) {
	targetID := c.Param("id")
	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	actor := ""
	if claims := mw.GetClaims(c); claims != nil {
		actor = claims.UserID
	}
	if actor == targetID && !body.IsActive {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "vous ne pouvez pas désactiver votre propre compte"})
		return
	}

	if !body.IsActive {
		last, err := h.isLastAdmin(c, targetID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
			return
		}
		if last {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": ErrLastAdmin.Error()})
			return
		}
	}

	q := db.Rebind(`UPDATE users SET is_active = ?, updated_at = ? WHERE id = ?`, h.usePostgres)
	res, err := h.db.ExecContext(c.Request.Context(), q,
		boolToSQL(body.IsActive), time.Now().UTC(), targetID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "compte introuvable"})
		return
	}

	// Les jetons de rafraîchissement du compte désactivé sont révoqués : sans
	// cela, le cookie déjà posé continuerait de produire des jetons d'accès.
	// Le contrôle des droits les rejetterait — il lit l'état à chaque requête —
	// mais laisser vivre une session révoquée est une imprécision inutile.
	if !body.IsActive {
		revoke := db.Rebind(`DELETE FROM refresh_tokens WHERE user_id = ?`, h.usePostgres)
		if _, err := h.db.ExecContext(c.Request.Context(), revoke, targetID); err != nil {
			_ = err // non bloquant : le compte est déjà inactif
		}
	}

	action := "user_deactivated"
	if body.IsActive {
		action = "user_reactivated"
	}
	h.recordSecurityEvent(c, action, targetID, "")

	msg := "Compte désactivé. L'accès est coupé immédiatement, y compris pour une session ouverte."
	if body.IsActive {
		msg = "Compte réactivé."
	}
	c.JSON(http.StatusOK, gin.H{"message": msg, "is_active": body.IsActive})
}

// isLastAdmin reports whether removing this account's admin rights would leave
// the installation with none.
//
// Le compte lui-même doit être administrateur ET actif pour compter : un
// administrateur désactivé n'administre rien, et le retirer du décompte
// éviterait de croire qu'il reste un recours.
func (h *UsersHandler) isLastAdmin(c *gin.Context, userID string) (bool, error) {
	var isAdminActive int
	q := db.Rebind(`SELECT COUNT(*) FROM users WHERE id = ? AND role = 'admin' AND is_active = 1`, h.usePostgres)
	if err := h.db.QueryRowContext(c.Request.Context(), q, userID).Scan(&isAdminActive); err != nil {
		return false, err
	}
	if isAdminActive == 0 {
		return false, nil // ce compte n'est pas un administrateur actif
	}

	var total int
	countQ := db.Rebind(`SELECT COUNT(*) FROM users WHERE role = 'admin' AND is_active = 1`, h.usePostgres)
	if err := h.db.QueryRowContext(c.Request.Context(), countQ).Scan(&total); err != nil {
		return false, err
	}
	return total <= 1, nil
}

// recordSecurityEvent trace les changements de droits.
//
// Un changement de rôle est exactement ce qu'on veut pouvoir reconstituer après
// coup — qui a donné quoi à qui, et quand. Un échec d'écriture ici ne défait pas
// l'action : la journalisation ne doit pas devenir un moyen de bloquer
// l'administration.
func (h *UsersHandler) recordSecurityEvent(c *gin.Context, action, targetID, detail string) {
	actor := ""
	if claims := mw.GetClaims(c); claims != nil {
		actor = claims.UserID
	}
	q := db.Rebind(`
		INSERT INTO security_events (id, event_type, ip_address, detail, created_at)
		VALUES (?, ?, ?, ?, ?)`, h.usePostgres)
	_, _ = h.db.ExecContext(c.Request.Context(), q,
		db.NewID(), action, c.ClientIP(),
		strings.TrimSpace("auteur="+actor+" cible="+targetID+" "+detail), time.Now().UTC())
}
