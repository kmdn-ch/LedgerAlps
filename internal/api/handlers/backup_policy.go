package handlers

// La phrase de passe des sauvegardes automatiques.
//
// Le point d'entrée existe parce que le comportement par défaut était de ne
// rien chiffrer, sans le dire. Il ne suffisait pas de renverser le défaut : une
// phrase de passe que l'utilisateur ne peut pas produire après une panne de
// disque transforme dix ans de pièces à conserver (CO art. 958f) en dix ans de
// pièces perdues. Le compromis retenu est donc : LedgerAlps la retient pour lui
// à chaque démarrage, l'utilisateur la note pour le jour où la machine n'est
// plus là.

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// GetBackupPolicy GET /api/v1/backups/policy
func (h *BackupsHandler) GetBackupPolicy(c *gin.Context) {
	policy := db.NewBackupPolicy(config.AppDataDir())
	c.JSON(http.StatusOK, policy.Status(db.BackupDir()))
}

// SetBackupPolicy PUT /api/v1/backups/policy
//
// Le corps porte une confirmation explicite que la phrase a été notée. Ce n'est
// pas une formalité : c'est le seul moment où quelqu'un peut encore la noter.
// Après, LedgerAlps ne peut plus la lui montrer — elle est scellée, et la
// desceller pour l'afficher annulerait l'intérêt de la sceller.
func (h *BackupsHandler) SetBackupPolicy(c *gin.Context) {
	var body struct {
		Passphrase string `json:"passphrase" binding:"required"`
		// EncryptExisting demande de chiffrer aussi les copies déjà écrites.
		EncryptExisting bool `json:"encrypt_existing"`
		// Acknowledged : « je l'ai notée ailleurs que sur cet ordinateur ».
		Acknowledged bool `json:"acknowledged"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if !body.Acknowledged {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "confirmez d'abord avoir noté cette phrase de passe ailleurs que sur cet ordinateur : " +
				"sans elle, personne ne peut ouvrir vos sauvegardes, vous non plus"})
		return
	}

	policy := db.NewBackupPolicy(config.AppDataDir())
	if err := policy.Set(body.Passphrase); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"message": "Les prochaines sauvegardes automatiques seront chiffrées."}

	if body.EncryptExisting {
		// Argon2id est lent par construction, et il y a jusqu'à quatorze copies :
		// le délai par défaut d'une requête couperait au milieu.
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
		defer cancel()

		converted, err := db.EncryptExisting(ctx, db.BackupDir(), body.Passphrase)
		resp["encrypted_existing"] = converted
		if err != nil {
			// La phrase est enregistrée, une partie des copies est convertie :
			// répondre 500 laisserait croire que rien n'a été fait.
			resp["warning"] = "certaines copies existantes n'ont pas pu être chiffrées: " + err.Error()
			c.JSON(http.StatusOK, resp)
			return
		}
		if len(converted) > 0 {
			resp["message"] = "Phrase de passe enregistrée, et les copies existantes ont été chiffrées."
		}
	}

	c.JSON(http.StatusOK, resp)
}

// ClearBackupPolicy DELETE /api/v1/backups/policy
//
// Revient à des sauvegardes en clair. Une confirmation est exigée parce que
// c'est une protection qu'on retire, et que le résultat — des copies lisibles
// sans clé — n'est visible nulle part avant qu'il soit trop tard.
func (h *BackupsHandler) ClearBackupPolicy(c *gin.Context) {
	if c.Query("confirm") != "true" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "confirmation requise: les prochaines sauvegardes seront écrites en clair"})
		return
	}
	policy := db.NewBackupPolicy(config.AppDataDir())
	if err := policy.Clear(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Phrase de passe retirée. Les prochaines sauvegardes seront écrites en clair. " +
			"Les copies déjà chiffrées le restent et exigent toujours l'ancienne phrase.",
	})
}
