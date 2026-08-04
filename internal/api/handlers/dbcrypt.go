package handlers

// Chiffrement de la base de données.
//
// Contrairement aux sauvegardes, ce n'est PAS activé par défaut. Le chiffrement
// du disque (BitLocker, LUKS) couvre déjà les mêmes menaces, gratuitement et
// sans que LedgerAlps ait à détenir quoi que ce soit ; ce qu'il ajoute est
// étroit mais réel : la protection suit le fichier, donc une base copiée sur un
// NAS ou dans un dossier synchronisé reste illisible.
//
// Étroit ne veut pas dire gratuit. Chiffrer, c'est introduire une façon
// nouvelle de perdre dix ans de pièces à conserver (CO art. 958f). D'où la
// phrase de récupération obligatoire, et le refus d'activer sans elle.

import (
	"net/http"

	"github.com/gin-gonic/gin"
	mw "github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

type dbCryptStatus struct {
	// Encrypted : l'état du fichier sur le disque, lu à l'instant. C'est le seul
	// fait qui compte ; tout le reste l'explique.
	Encrypted bool `json:"encrypted"`
	// Configured : une clé existe sur cette machine.
	Configured bool `json:"configured"`
	// KeyAvailable : cette clé se descelle sur ce compte. Faux après un
	// changement de compte Windows ou une réinstallation.
	KeyAvailable bool   `json:"key_available"`
	HasRecovery  bool   `json:"has_recovery"`
	Sealed       bool   `json:"sealed"`
	Mechanism    string `json:"mechanism"`
	// Pending : conversion demandée, en attente du redémarrage.
	Pending string `json:"pending,omitempty"`
	// Supported : faux sur PostgreSQL, où le chiffrement se règle côté serveur.
	Supported bool `json:"supported"`
}

// GetDatabaseEncryption GET /api/v1/database/encryption
func (h *BackupsHandler) GetDatabaseEncryption(c *gin.Context) {
	st := dbCryptStatus{Supported: !h.cfg.UsePostgres()}
	if !st.Supported {
		c.JSON(http.StatusOK, st)
		return
	}

	dir := config.AppDataDir()
	keys := db.NewDatabaseKeys(dir)

	encrypted, err := db.IsDatabaseEncrypted(h.cfg.SQLitePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	st.Encrypted = encrypted
	st.Configured = keys.Configured()
	st.HasRecovery = keys.HasRecovery()
	st.Sealed = db.SecretsSealed()
	st.Mechanism = db.SecretsMechanism()
	if st.Configured {
		_, keyErr := keys.Key()
		st.KeyAvailable = keyErr == nil
	}
	if p := db.ReadPendingEncryption(dir); p != nil {
		st.Pending = p.Action
	}
	c.JSON(http.StatusOK, st)
}

// EnableDatabaseEncryption POST /api/v1/database/encryption
//
// Crée la clé et programme la conversion pour le prochain démarrage. La
// conversion elle-même ne peut pas se faire ici : elle remplace le fichier que
// ce serveur a ouvert.
func (h *BackupsHandler) EnableDatabaseEncryption(c *gin.Context) {
	if h.cfg.UsePostgres() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "sur PostgreSQL, le chiffrement au repos se règle côté serveur de base de données"})
		return
	}
	var body struct {
		RecoveryPassphrase string `json:"recovery_passphrase" binding:"required"`
		Acknowledged       bool   `json:"acknowledged"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if !body.Acknowledged {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "confirmez d'abord avoir noté la phrase de récupération ailleurs que sur cet ordinateur : " +
				"elle est le seul moyen de rouvrir cette base depuis une autre machine"})
		return
	}

	dir := config.AppDataDir()
	keys := db.NewDatabaseKeys(dir)
	if keys.Configured() {
		c.JSON(http.StatusConflict, gin.H{"error": "cette base a déjà une clé"})
		return
	}
	if _, err := keys.Create(body.RecoveryPassphrase); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	requestedBy := ""
	if claims := mw.GetClaims(c); claims != nil {
		requestedBy = claims.UserID
	}
	if _, err := db.StageEncryption(dir, db.ActionEncrypt, requestedBy); err != nil {
		// La clé existe mais rien ne l'utilisera : la retirer plutôt que de
		// laisser une machine dans un état que l'interface ne sait pas décrire.
		_ = keys.Forget()
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"pending": db.ActionEncrypt,
		"message": "Clé créée. La base sera chiffrée au prochain démarrage de LedgerAlps — " +
			"la conversion remplace le fichier que le serveur a ouvert, elle ne peut pas se faire maintenant.",
	})
}

// DisableDatabaseEncryption DELETE /api/v1/database/encryption
func (h *BackupsHandler) DisableDatabaseEncryption(c *gin.Context) {
	if c.Query("confirm") != "true" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "confirmation requise: la base sera réécrite en clair sur ce disque"})
		return
	}
	dir := config.AppDataDir()
	keys := db.NewDatabaseKeys(dir)
	if !keys.Configured() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "cette base n'est pas chiffrée"})
		return
	}
	if _, err := keys.Key(); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "la clé est illisible sur ce compte : récupérez-la d'abord avec la phrase de récupération"})
		return
	}

	requestedBy := ""
	if claims := mw.GetClaims(c); claims != nil {
		requestedBy = claims.UserID
	}
	if _, err := db.StageEncryption(dir, db.ActionDecrypt, requestedBy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"pending": db.ActionDecrypt,
		"message": "La base sera réécrite en clair au prochain démarrage de LedgerAlps.",
	})
}

// CancelDatabaseEncryption DELETE /api/v1/database/encryption/pending
func (h *BackupsHandler) CancelDatabaseEncryption(c *gin.Context) {
	dir := config.AppDataDir()
	p := db.ReadPendingEncryption(dir)
	if p == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "aucune conversion en attente"})
		return
	}
	db.ClearPendingEncryption(dir)
	// Annuler un chiffrement jamais appliqué doit aussi retirer la clé : la
	// laisser ferait chiffrer la base au démarrage suivant par la réconciliation,
	// c'est-à-dire exactement ce que l'utilisateur vient d'annuler.
	if p.Action == db.ActionEncrypt {
		keys := db.NewDatabaseKeys(dir)
		if encrypted, err := db.IsDatabaseEncrypted(h.cfg.SQLitePath); err == nil && !encrypted {
			_ = keys.Forget()
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "conversion annulée"})
}

// RecoverDatabaseKey POST /api/v1/database/encryption/recover
//
// Le cas « nouveau PC, Windows réinstallé, profil recréé » : la clé scellée ne
// se descelle plus, mais la phrase de récupération enveloppe la même clé.
func (h *BackupsHandler) RecoverDatabaseKey(c *gin.Context) {
	var body struct {
		RecoveryPassphrase string `json:"recovery_passphrase" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	keys := db.NewDatabaseKeys(config.AppDataDir())
	if _, err := keys.Recover(body.RecoveryPassphrase); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Clé retrouvée et rescellée à ce compte. Redémarrez LedgerAlps.",
	})
}
