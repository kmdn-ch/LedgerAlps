package handlers

// Réglages de sécurité de la session : rotation de la clé de signature et
// déconnexion sur inactivité.
//
// Les deux se lisent ensemble parce qu'ils font le même travail par deux
// chemins — borner la durée pendant laquelle une session vaut quelque chose —
// et que les régler séparément conduit à en durcir un en laissant l'autre
// grand ouvert.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
)

type securitySettingsResponse struct {
	Rotation config.JWTRotationStatus `json:"rotation"`
	// IdleLogoutMinutes vaut 0 quand la déconnexion automatique est désactivée.
	IdleLogoutMinutes int `json:"idle_logout_minutes"`
	// AccessMinutes est la durée de vie d'un jeton d'accès. Renseigné pour que
	// l'interface n'annonce pas une déconnexion plus longue que ce que le
	// serveur accorde de toute façon.
	AccessMinutes int `json:"access_minutes"`
}

// GetSecuritySettings GET /api/v1/settings/security
func (h *SystemHandler) GetSecuritySettings(c *gin.Context) {
	c.JSON(http.StatusOK, securitySettingsResponse{
		Rotation:          config.RotationStatus(h.cfg),
		IdleLogoutMinutes: h.cfg.IdleLogoutMinutes,
		AccessMinutes:     h.cfg.JWTAccessMinutes,
	})
}

// UpdateSecuritySettings PUT /api/v1/settings/security
//
// La périodicité de la rotation ne se règle plus : elle est quotidienne, point.
// Le seul réglage restant est le délai d'inactivité, et il prend effet tout de
// suite — il est appliqué par l'interface, qui relit ce réglage.
//
// Un `rotation_days` envoyé par un client resté sur une ancienne version est
// ignoré en silence plutôt que refusé : refuser ferait échouer l'enregistrement
// du délai d'inactivité, qui, lui, est valide.
func (h *SystemHandler) UpdateSecuritySettings(c *gin.Context) {
	var body struct {
		// Pointeur : distinguer « absent, ne touche pas » de « posé à zéro »,
		// qui veut dire désactiver et doit être respecté.
		IdleLogoutMinutes *int `json:"idle_logout_minutes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	if body.IdleLogoutMinutes != nil {
		if err := config.SetIdleLogoutMinutes(*body.IdleLogoutMinutes); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		h.cfg.IdleLogoutMinutes = *body.IdleLogoutMinutes
	}

	c.JSON(http.StatusOK, securitySettingsResponse{
		Rotation:          config.RotationStatus(h.cfg),
		IdleLogoutMinutes: h.cfg.IdleLogoutMinutes,
		AccessMinutes:     h.cfg.JWTAccessMinutes,
	})
}
