package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kmdn-ch/ledgeralps/internal/api/handlers"
	"github.com/kmdn-ch/ledgeralps/internal/api/middleware"
	"github.com/kmdn-ch/ledgeralps/internal/config"
	"github.com/kmdn-ch/ledgeralps/internal/core/authz"
	"github.com/kmdn-ch/ledgeralps/internal/core/tlsutil"
	"github.com/kmdn-ch/ledgeralps/internal/db"
	embeddedFrontend "github.com/kmdn-ch/ledgeralps/internal/frontend"
	"github.com/kmdn-ch/ledgeralps/internal/services/accounting"
	"github.com/kmdn-ch/ledgeralps/internal/services/banking"
	"github.com/kmdn-ch/ledgeralps/internal/services/updatecheck"
	"github.com/kmdn-ch/ledgeralps/version"
)

func main() {
	// ── 1. Load and validate configuration ────────────────────────────────────
	cfg := config.Load()

	// ── 1a. Rotation de la clé de signature ───────────────────────────────────
	//
	// Au démarrage, et jamais en cours de session : régénérer la clé invalide
	// toutes les sessions, et LedgerAlps n'enregistre aucun brouillon
	// automatique. Couper pendant une saisie ferait perdre la facture — une
	// mesure de sécurité qui fait perdre du travail est une mesure qu'on
	// désactive.
	//
	// Un échec n'empêche pas de démarrer : garder la clé un jour de plus est
	// moins grave que de laisser l'utilisateur sans ses livres.
	if rotated, err := config.MaybeRotateJWTSecret(cfg, time.Now()); err != nil {
		log.Printf("WARNING: la clé de signature n'a pas pu être régénérée: %v", err)
	} else if rotated {
		fmt.Println("LedgerAlps: clé de signature régénérée — reconnexion nécessaire")
	}

	// ── 1b. Apply a restore staged from the interface ─────────────────────────
	// Restoring swaps the database file out from under every open connection,
	// so the running server cannot do it to itself. The UI stages the restore;
	// it is applied here, before the database is opened, and only here.
	//
	// A failure must not stop the start: the user would be left with neither
	// the restored books nor the ones they had. The staged copy is cleared
	// either way, so a broken restore is not retried at every launch.
	if applied, previous, err := db.ApplyPendingRestore(context.Background(), cfg, db.BackupDir()); err != nil {
		log.Printf("WARNING: la restauration demandée a échoué, la base actuelle est conservée: %v", err)
	} else if applied != "" {
		fmt.Printf("LedgerAlps: sauvegarde restaurée depuis %s\n", applied)
		if previous != "" {
			fmt.Printf("LedgerAlps: base précédente conservée dans %s\n", previous)
		}
	}

	// ── 1c. État de chiffrement de la base ────────────────────────────────────
	//
	// Après la restauration, et avant l'ouverture : une conversion remplace le
	// fichier que chaque connexion utiliserait.
	//
	// C'est une réconciliation, pas seulement l'application d'une demande. Une
	// restauration écrit un instantané EN CLAIR par-dessus la base ; sans ce
	// passage, une installation dont le propriétaire avait activé le chiffrement
	// reviendrait en clair sans un mot, pendant que l'interface continuerait à
	// afficher « chiffrée ».
	//
	// Un échec est fatal, contrairement à la restauration : continuer
	// signifierait ouvrir en clair une base que l'utilisateur croit protégée.
	if done, err := db.ReconcileDatabaseEncryption(
		context.Background(), cfg, config.AppDataDir()); err != nil {
		log.Fatalf("FATAL: %v", err)
	} else if done != "" {
		fmt.Printf("LedgerAlps: %s\n", done)
	}

	// ── 2. Open database ──────────────────────────────────────────────────────
	database, err := db.Open(cfg)
	if err != nil {
		// Base chiffree dont la cle ne se descelle pas sur ce compte : mourir
		// ici rendrait la recuperation injoignable, alors que c'est exactement
		// la situation pour laquelle la phrase de recuperation existe.
		if errors.Is(err, db.ErrDatabaseKeyUnavailable) {
			runRecoveryServer(cfg, err)
			return
		}
		log.Fatalf("FATAL: cannot open database: %v", err)
	}
	defer database.Close()

	// ── 3. Migrations auto-embarquées ─────────────────────────────────────────
	fmt.Println("LedgerAlps: applying migrations…")
	if err := db.Migrate(database, cfg.UsePostgres()); err != nil {
		log.Fatalf("FATAL: migration failed: %v", err)
	}
	fmt.Println("LedgerAlps: migrations up-to-date.")

	// Rattachement des écritures et documents à leur exercice comptable. Le
	// champ n'était renseigné nulle part avant la v1.4.6, si bien qu'une base
	// existante contient des lignes orphelines — invisibles à la clôture, qui
	// filtre dessus. Idempotent : sans orphelin, c'est deux SELECT.
	if err := db.BackfillFiscalYears(database, cfg.UsePostgres()); err != nil {
		log.Fatalf("FATAL: fiscal year backfill failed: %v", err)
	}

	// Identité du destinataire figée sur les factures. Le PDF relisait le
	// contact vivant : renommer un client réécrivait ses factures passées.
	if err := db.BackfillInvoiceRecipients(database, cfg.UsePostgres()); err != nil {
		log.Fatalf("FATAL: invoice recipient backfill failed: %v", err)
	}

	// Rétention des données personnelles (nLPD art. 6 al. 4). Les adresses IP
	// des verrouillages de connexion s'accumulaient sans terme, alors que le
	// schéma annonçait une durée limitée. Un échec ici ne doit pas empêcher le
	// démarrage : l'utilisateur a besoin de ses livres.
	if _, err := db.ApplyRetention(database, cfg.UsePostgres(), time.Now().UTC()); err != nil {
		log.Printf("WARNING: passe de rétention échouée: %v", err)
	}

	// ── 3b. Automatic backup ──────────────────────────────────────────────────
	// LedgerAlps is local-first: this SQLite file is the only copy of records the
	// CO (art. 958f) requires be kept for ten years. Take a snapshot at startup
	// when the newest one is older than a day. A failure here must never stop the
	// server — the user still needs access to their books.
	//
	// La phrase de passe vient du coffre scellé au compte, ou de
	// BACKUP_PASSPHRASE qui reste prioritaire pour un déploiement serveur.
	// Sans elle, la copie est écrite en clair — c'est un choix légitime, mais il
	// est dit à voix haute au lieu d'être le comportement par défaut silencieux
	// qu'il était.
	//
	// Une phrase faible est signalée mais n'empêche pas la sauvegarde : refuser
	// ici laisserait l'utilisateur sans aucune copie, ce qui est pire qu'une
	// copie moins bien protégée.
	backupPolicy := db.NewBackupPolicy(config.AppDataDir())
	autoPass, passSource := backupPolicy.Passphrase()
	switch passSource {
	case db.SourceEnv:
		if err := db.ValidatePassphrase(autoPass); err != nil {
			log.Printf("WARNING: BACKUP_PASSPHRASE est faible (%v) — les sauvegardes automatiques sont chiffrées, mais moins solidement", err)
		}
	case db.SourceUnavailable:
		// Le secret existe mais n'a pas pu être descellé : compte ou machine
		// différents. Le dire, plutôt que d'écrire une copie en clair sans
		// prévenir quelqu'un qui croit ses sauvegardes protégées.
		log.Printf("WARNING: la phrase de passe des sauvegardes est illisible sur ce compte — " +
			"la sauvegarde automatique sera écrite EN CLAIR. Redéfinissez-la dans Paramètres → Maintenance → Sécurité.")
	case db.SourceNone:
		log.Printf("Les sauvegardes automatiques ne sont pas chiffrées. " +
			"Paramètres → Maintenance → Sécurité pour définir une phrase de passe.")
	}
	if path, err := db.MaybeAutoBackup(
		context.Background(), database, cfg, db.BackupDir(), db.DefaultInterval, db.DefaultKeep,
		autoPass,
	); err != nil {
		log.Printf("WARNING: automatic backup failed: %v", err)
	} else if path != "" {
		fmt.Printf("LedgerAlps: backup written to %s\n", path)
	}

	// ── 4. Gin ────────────────────────────────────────────────────────────────
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		log.Printf("PANIC recovered: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
	}))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.CORS(strings.Split(cfg.AllowedOrigins, ",")...))
	r.Use(middleware.ErrorHandler())
	if cfg.Debug {
		r.Use(gin.Logger())
	}

	// ── 5. Health ─────────────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": version.Version()})
	})

	// ── 6. Services ───────────────────────────────────────────────────────────
	accountingSvc := accounting.New(database, cfg.UsePostgres())

	// ── 7. API v1 ─────────────────────────────────────────────────────────────
	v1 := r.Group("/api/v1")

	// Auth — public endpoints.
	// Credential-checking routes sit behind a per-IP rate limiter so an attacker
	// cannot brute-force a password against a locally exposed instance.
	authHandler := handlers.NewAuthHandler(database, cfg)
	loginLimiter := middleware.NewLoginRateLimiter(
		middleware.DefaultLoginMaxAttempts,
		middleware.DefaultLoginWindow,
		middleware.DefaultLoginLockout,
	)
	// Persist lockouts so brute-force attempts are visible to an administrator.
	loginLimiter.OnLockout(func(ip string, until time.Time) {
		handlers.RecordLoginLockout(database, cfg.UsePostgres(), ip, until)
	})
	v1.POST("/auth/login", loginLimiter.Middleware(), authHandler.Login)
	v1.POST("/auth/refresh", loginLimiter.Middleware(), authHandler.Refresh)
	v1.POST("/auth/logout", authHandler.Logout)
	// Hors du groupe filtré : c'est la seule action qu'un compte au mot de passe
	// temporaire puisse faire, et la placer dans le groupe la bloquerait
	// elle-même — le compte serait alors définitivement enfermé.
	v1.POST("/auth/change-password", middleware.RequireAuth(cfg.JWTSecret), authHandler.ChangePassword)

	// Second facteur.
	//
	// La vérification est derrière la limitation de tentatives : six chiffres se
	// devinent en un million d'essais, ce qui est peu pour une machine et beaucoup
	// pour quelqu'un à qui l'on ferme la porte au bout de quelques erreurs. Elle
	// n'accepte QUE le jeton d'attente, et le filtre d'authentification refuse ce
	// jeton partout ailleurs.
	v1.POST("/auth/mfa/verify", loginLimiter.Middleware(),
		middleware.RequireMFAChallenge(cfg.JWTSecret), authHandler.MFAVerify)

	// Inscription et retrait, hors du groupe filtré pour la même raison que le
	// changement de mot de passe : un administrateur non inscrit ne peut rien
	// faire d'autre, et si ces routes étaient dans le groupe elles se
	// bloqueraient elles-mêmes — le compte serait enfermé hors de sa propre
	// installation.
	v1.GET("/auth/mfa", middleware.RequireAuth(cfg.JWTSecret), authHandler.MFAStatus)
	v1.POST("/auth/mfa/setup", middleware.RequireAuth(cfg.JWTSecret), authHandler.MFASetup)
	v1.POST("/auth/mfa/confirm", middleware.RequireAuth(cfg.JWTSecret), authHandler.MFAConfirm)
	v1.DELETE("/auth/mfa", middleware.RequireAuth(cfg.JWTSecret), authHandler.MFADisable)
	// Ordinateurs de confiance. Hors du groupe filtre pour la meme raison que le
	// reste du second facteur : ces routes doivent rester joignables pendant
	// qu'un compte est encore bloque sur l'inscription.
	v1.GET("/auth/devices", middleware.RequireAuth(cfg.JWTSecret), authHandler.ListTrustedDevices)
	v1.DELETE("/auth/devices", middleware.RequireAuth(cfg.JWTSecret), authHandler.ForgetTrustedDevices)

	v1.POST("/auth/register", loginLimiter.Middleware(), authHandler.Register)
	v1.POST("/auth/bootstrap", loginLimiter.Middleware(), authHandler.Bootstrap) // one-shot: creates first admin user

	// Swiss registry proxy — public (called from setup wizard, no auth yet)
	v1.GET("/uid-lookup", handlers.UIDLookup)

	// Protected routes — JWT required
	authorizer := middleware.NewAuthorizer(database, cfg.UsePostgres(), cfg.JWTSecret)
	api := v1.Group("")
	api.Use(middleware.RequireAuth(cfg.JWTSecret))

	// Seconde barrière, indépendante des permissions déclarées par route.
	//
	// Les gardes par route dépendent de ce qu'on a pensé à écrire ; oublier une
	// déclaration sur une nouvelle route est la façon la plus courante d'ouvrir
	// un trou, parce que rien ne le signale. Ce filtre refuse toute méthode
	// d'écriture à un rôle en lecture seule, quelle que soit la route — donc y
	// compris sur celles qui n'existent pas encore.
	api.Use(authorizer.DenyWritesWithoutPermission())

	// Un mot de passe temporaire, créé par un administrateur pour quelqu'un
	// d'autre, a circulé par un canal qui n'est pas fait pour ça. Tant qu'il n'est
	// pas remplacé, une action tracée sous ce compte ne prouve pas qui l'a faite.
	// Le compte ne peut donc RIEN faire, pas même lire.
	api.Use(authorizer.RequirePasswordChanged())

	// Un compte administrateur détient les clés de l'installation : créer des
	// comptes, restaurer une sauvegarde, déverrouiller une période. Son mot de
	// passe est la seule chose qui en sépare quelqu'un. Tant qu'un second facteur
	// n'est pas inscrit, il ne peut rien faire d'autre que l'inscrire.
	api.Use(authorizer.RequireMFAEnrolled())

	// Journal
	jh := handlers.NewJournalHandler(database, cfg.UsePostgres())
	jwh := handlers.NewJournalWriteHandler(accountingSvc, database, cfg.UsePostgres())
	api.GET("/journal", jh.ListJournal)
	api.GET("/journal/:id", jh.GetJournalEntry)
	api.POST("/journal", authorizer.Require(authz.PermWriteAccounting), jwh.CreateEntry)
	api.POST("/journal/:id/post", authorizer.Require(authz.PermWriteAccounting), jwh.PostEntry)

	// Accounts
	ah := handlers.NewAccountsHandler(database, cfg.UsePostgres())
	api.GET("/accounts", ah.ListAccounts)
	api.GET("/accounts/trial-balance", ah.TrialBalance) // BEFORE /:code to avoid shadowing
	api.GET("/accounts/:code/balance", ah.AccountBalance)
	api.POST("/accounts", ah.CreateAccount)

	// Contacts
	ch := handlers.NewContactsHandler(database, cfg.UsePostgres())
	api.GET("/contacts", authorizer.Require(authz.PermRead), ch.ListContacts)
	api.GET("/contacts/:id", ch.GetContact)
	api.POST("/contacts", ch.CreateContact)
	api.PATCH("/contacts/:id", ch.UpdateContact)
	// Anonymisation (nLPD art. 6 al. 4 et 32) : effacer les données d'une
	// personne est une décision, pas une opération de saisie.
	api.POST("/contacts/:id/anonymise", authorizer.Require(authz.PermManage), ch.AnonymiseContact)

	// Invoices
	ih := handlers.NewInvoicesHandler(database, cfg.UsePostgres(), accountingSvc)
	// Supplier invoices (factures d'achat) — source of deductible input VAT
	sih := handlers.NewSupplierInvoicesHandler(database, cfg.UsePostgres()).WithAccounting(accountingSvc)
	// Consulter les achats est une lecture ; les saisir, les comptabiliser ou les
	// effacer ne l'est pas. Les permissions sont declarees route par route, en
	// plus du filtre global qui refuse deja toute ecriture a un role en lecture
	// seule — deux barrieres qui couvrent des erreurs differentes.
	api.GET("/supplier-invoices", sih.ListSupplierInvoices)
	api.GET("/supplier-invoices/:id", sih.GetSupplierInvoice)
	api.POST("/supplier-invoices", authorizer.Require(authz.PermWriteDocuments), sih.CreateSupplierInvoice)
	// Lire le QR d'une facture deposee. Cette route ne fait que LIRE : elle
	// n'enregistre rien, ne cree aucun contact. La permission d'ecriture est
	// exigee malgre tout, parce qu'elle sert a preparer une saisie — et qu'un
	// compte en lecture seule n'a rien a preparer.
	qbh := handlers.NewQRBillHandler(database, cfg.UsePostgres())
	api.POST("/supplier-invoices/read-qr",
		authorizer.Require(authz.PermWriteDocuments), qbh.ReadSupplierBill)

	// Modifier n'est possible qu'au brouillon : une facture comptabilisee porte
	// une ecriture scellee, et la changer ferait mentir le journal.
	api.PUT("/supplier-invoices/:id", authorizer.Require(authz.PermWriteDocuments), sih.UpdateSupplierInvoice)
	api.POST("/supplier-invoices/:id/transition", authorizer.Require(authz.PermWriteAccounting), sih.TransitionSupplierInvoice)
	api.DELETE("/supplier-invoices/:id", authorizer.Require(authz.PermWriteAccounting), sih.DeleteSupplierInvoice)
	// Vider la liste des paiements sans mentir aux livres : un brouillon est
	// supprimé, une facture comptabilisée est EXTOURNÉE puis marquée annulée.
	// Tenir les livres est le métier du comptable autant que de l'administrateur.
	api.POST("/supplier-invoices/cancel",
		authorizer.Require(authz.PermWriteAccounting), sih.CancelSupplierInvoices)

	api.GET("/invoices", ih.ListInvoices)
	api.GET("/invoices/:id", ih.GetInvoice)
	api.GET("/invoices/:id/pdf", ih.GetInvoicePDF)
	api.GET("/invoices/:id/six-validation", ih.SixValidationDossier)
	// Téléchargement groupé : un PDF si un seul document, un ZIP si plusieurs.
	api.POST("/invoices/bulk-pdf", ih.BulkInvoicePDF)
	api.POST("/invoices", ih.CreateInvoice)
	api.PATCH("/invoices/:id", authorizer.Require(authz.PermWriteDocuments), ih.UpdateInvoice)
	api.POST("/invoices/:id/transition", ih.TransitionInvoice)
	// Quote lifecycle: an offer becomes an invoice by producing one, not by
	// mutating into one — both documents are kept and linked.
	api.POST("/invoices/:id/convert", ih.ConvertQuote)
	api.POST("/invoices/:id/outcome", ih.SetQuoteOutcome)
	// Une note de crédit cite la facture qu'elle annule (LTVA art. 27 al. 4)
	// et son montant est borné par ce qui a déjà été crédité.
	api.POST("/invoices/:id/credit-note", ih.CreateCreditNote)

	// Sauvegardes. Créer un instantané est sûr serveur en marche ; restaurer
	// ne l'est pas, la restauration est donc préparée puis appliquée au
	// démarrage suivant. Réservé aux administrateurs : une restauration
	// remplace toute la comptabilité.
	bh := handlers.NewBackupsHandler(database, cfg)
	// Creer une sauvegarde et lister celles qui existent releve de l'hygiene
	// comptable : le comptable doit pouvoir le faire. RESTAURER, en revanche,
	// remplace les livres par une autre version d'eux-memes, et la politique de
	// chiffrement est une fonction de securite — les deux restent a
	// l'administrateur.
	api.GET("/backups", authorizer.Require(authz.PermManage), bh.ListBackups)
	api.POST("/backups", authorizer.Require(authz.PermManage), bh.CreateBackup)
	api.POST("/backups/restore", authorizer.Require(authz.PermAdmin), bh.StageRestore)
	api.DELETE("/backups/restore", authorizer.Require(authz.PermAdmin), bh.CancelRestore)
	api.GET("/backups/policy", authorizer.Require(authz.PermAdmin), bh.GetBackupPolicy)
	api.PUT("/backups/policy", authorizer.Require(authz.PermAdmin), bh.SetBackupPolicy)
	api.DELETE("/backups/policy", authorizer.Require(authz.PermAdmin), bh.ClearBackupPolicy)
	api.GET("/database/encryption", authorizer.Require(authz.PermAdmin), bh.GetDatabaseEncryption)
	api.POST("/database/encryption", authorizer.Require(authz.PermAdmin), bh.EnableDatabaseEncryption)
	api.DELETE("/database/encryption", authorizer.Require(authz.PermAdmin), bh.DisableDatabaseEncryption)
	api.DELETE("/database/encryption/pending", authorizer.Require(authz.PermAdmin), bh.CancelDatabaseEncryption)
	api.POST("/database/encryption/recover", authorizer.Require(authz.PermAdmin), bh.RecoverDatabaseKey)
	api.PUT("/database/encryption/recovery", authorizer.Require(authz.PermAdmin), bh.ChangeRecoveryPassphrase)

	// Redémarrage — proposé uniquement pour appliquer une restauration préparée.
	// Le handler signale, main exécute : un handler ne doit pas démonter le
	// serveur depuis lequel il répond.
	restartCh := make(chan struct{}, 1)
	sysh := handlers.NewSystemHandler(restartCh, cfg, database)
	api.POST("/system/restart", authorizer.Require(authz.PermAdmin), sysh.Restart)
	// Réglages réseau. config.json n'est écrit qu'au premier lancement : sans
	// cet écran, aucune option ajoutée depuis n'est atteignable par un
	// utilisateur, et éditer du JSON dans %APPDATA% n'est pas une réponse.
	api.GET("/settings/server", authorizer.Require(authz.PermAdmin), sysh.GetServerSettings)
	api.PUT("/settings/server", authorizer.Require(authz.PermAdmin), sysh.PutServerSettings)
	api.POST("/settings/server/rotate-secret", authorizer.Require(authz.PermAdmin), sysh.RotateJWTSecret)
	api.GET("/settings/security", authorizer.Require(authz.PermAdmin), sysh.GetSecuritySettings)
	api.PUT("/settings/security", authorizer.Require(authz.PermAdmin), sysh.UpdateSecuritySettings)

	// Maintenance & Système. Lecture seule et réservé aux administrateurs : ces
	// vues montrent l'état des données et du poste, elles ne réparent rien —
	// une comptabilité incohérente se corrige par une écriture, pas par un
	// bouton (CO art. 957a al. 2 ch. 5).
	mh := handlers.NewMaintenanceHandler(database, cfg)
	api.GET("/maintenance/integrity", authorizer.Require(authz.PermManage), mh.IntegrityCheck)
	api.GET("/maintenance/health", authorizer.Require(authz.PermManage), mh.SystemHealth)

	// Fiscal years + VAT declaration (admin)
	fyh := handlers.NewFiscalYearHandler(database, cfg.UsePostgres())
	api.GET("/fiscal-years", fyh.ListFiscalYears)
	api.POST("/fiscal-years", authorizer.Require(authz.PermManage), fyh.CreateFiscalYear)
	api.POST("/fiscal-years/:id/close", authorizer.Require(authz.PermManage), fyh.CloseFiscalYear)
	api.POST("/vat/declaration", fyh.GenerateVATDeclaration)

	// VAT rates (static reference data — no DB)
	api.GET("/vat/rates", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"rates": []gin.H{
				{"code": "standard", "rate": 8.1, "label": "Taux normal (TVA 2024)"},
				{"code": "reduced", "rate": 2.6, "label": "Taux réduit (alimentation, livres)"},
				{"code": "special", "rate": 3.8, "label": "Taux spécial (hébergement)"},
			},
		})
	})

	// ISO 20022 — pain.001 export + camt.053 import
	bankingSvc := banking.New(database, cfg.UsePostgres())
	runh := handlers.NewPaymentRunHandler(database, cfg.UsePostgres())
	isoH := handlers.NewISO20022HandlerWithReconciliation(bankingSvc).WithPaymentRun(runh)
	recoh := handlers.NewReconciliationHandler(database, cfg.UsePostgres())

	// Lire ce qu'il y a a payer est une consultation ; produire l'ordre de
	// virement ne l'est pas. Un compte en lecture seule voit donc la liste et ne
	// peut pas en tirer un fichier — la permission est declaree, en plus du
	// filtre global qui refuse deja toute ecriture a ce role.
	api.GET("/payments/payable", authorizer.Require(authz.PermRead), runh.ListPayable)
	api.POST("/payments/export", authorizer.Require(authz.PermWriteAccounting), isoH.ExportPain001)
	api.POST("/bank-statements/import", authorizer.Require(authz.PermWriteAccounting), isoH.ImportCamt053)
	api.GET("/bank-entries", recoh.ListBankEntries)
	api.PUT("/bank-entries/:id/match", authorizer.Require(authz.PermWriteAccounting), recoh.MatchBankEntry)
	api.DELETE("/bank-entries/:id/match", authorizer.Require(authz.PermWriteAccounting), recoh.UnmatchBankEntry)
	api.PUT("/bank-entries/:id/ignore", recoh.IgnoreBankEntry)

	// Legal archive export — CO art. 958f (10-year retention)
	expH := handlers.NewExportHandler(database, cfg.UsePostgres())
	api.GET("/exports/legal-archive", expH.LegalArchive)

	// Journal, grand livre, balance — les trois documents qu'une fiduciaire
	// reclame. Ce sont des lectures : un role en lecture seule doit pouvoir les
	// produire, c'est meme la raison d'etre de ce role.
	aeh := handlers.NewAccountingExportHandler(database, cfg.UsePostgres())
	api.GET("/exports/journal.csv", aeh.ExportJournalCSV)
	api.GET("/exports/ledger.csv", aeh.ExportLedgerCSV)
	api.GET("/exports/trial-balance.csv", aeh.ExportTrialBalanceCSV)

	// Stats dashboard
	statsH := handlers.NewStatsHandler(database, cfg.UsePostgres())
	api.GET("/stats", statsH.GetStats)

	// Reports
	rh := handlers.NewReportsHandler(database, cfg.UsePostgres())
	api.GET("/reports/balance-sheet", rh.BalanceSheet)
	api.GET("/reports/income-statement", rh.IncomeStatement)
	api.GET("/reports/general-ledger", rh.GeneralLedger)
	api.GET("/reports/ar-aging", rh.ARaging)
	// Chiffre d'affaires groupable : par année, par mois ou par client.
	api.GET("/reports/revenue", rh.Revenue)

	// Payments (CRUD — must be registered after /payments/export to avoid shadowing)
	ph := handlers.NewPaymentsHandler(database, cfg.UsePostgres(), accountingSvc)
	api.POST("/payments", ph.CreatePayment)
	api.GET("/payments", ph.ListPayments)
	api.GET("/payments/:id", ph.GetPayment)

	// Audit logs
	alh := handlers.NewAuditHandler(database, cfg.UsePostgres())
		// Le journal d'audit et la verification de la chaine sont le CONTROLE des
	// livres : c'est le metier du comptable, pas de l'administrateur du
	// logiciel. La lecture seule en est ecartee — ce registre nomme qui a fait
	// quoi, et le consulter est deja sensible.
	api.GET("/audit-logs", authorizer.Require(authz.PermManage), alh.ListAuditLogs)
	// Registered before the :id route: gin resolves the static segment first,
	// so "verify-chain" is never read as an identifier.
	api.GET("/audit-logs/verify-chain", authorizer.Require(authz.PermManage), alh.VerifyAuditChain)
	api.GET("/audit-logs/attestation", authorizer.Require(authz.PermManage), alh.IntegrityAttestation)
	api.GET("/audit-logs/:id/verify", authorizer.Require(authz.PermManage), alh.VerifyAuditLog)

	// Security telemetry — admin only: lockout records expose client IPs (nLPD).
	seh := handlers.NewSecurityEventHandler(database, cfg.UsePostgres())
	api.GET("/security-events", authorizer.Require(authz.PermAdmin), seh.ListSecurityEvents)

	// Compliance advisories — served from the feed embedded in the binary,
	// so this works with no network access.
	updates := updatecheck.New(cfg.UpdateCheck, updatecheck.DefaultEndpoint, updatecheck.DefaultInterval)
	cplh := handlers.NewComplianceHandler(updates)
	api.GET("/compliance/advisories", cplh.ListAdvisories)
	api.GET("/compliance/update-check", cplh.CheckForUpdate)

	// Company settings
	sh := handlers.NewSettingsHandler(database, cfg.UsePostgres())
	// Comptes et rôles. Administrateur seulement : donner ou retirer un droit
	// est l'action dont l'abus ne se répare pas.
	uh := handlers.NewUsersHandler(database, cfg.UsePostgres())
	api.GET("/users", authorizer.Require(authz.PermAdmin), uh.ListUsers)
	api.POST("/users", authorizer.Require(authz.PermAdmin), uh.CreateUser)
	api.PUT("/users/:id/role", authorizer.Require(authz.PermAdmin), uh.UpdateUserRole)
	api.PUT("/users/:id/active", authorizer.Require(authz.PermAdmin), uh.SetUserActive)
	// Deux gestes séparés, tracés séparément. Réunis en un seul clic, ils
	// permettraient à un administrateur de se substituer entièrement à n'importe
	// quel compte — et le second facteur ne protégerait plus de rien face à lui.
	api.POST("/users/:id/reset-password", authorizer.Require(authz.PermAdmin), uh.ResetPassword)
	api.DELETE("/users/:id/mfa", authorizer.Require(authz.PermAdmin), uh.RemoveMFA)

	api.GET("/settings/company", sh.GetCompany)
	api.PUT("/settings/company", authorizer.Require(authz.PermManage), sh.PutCompany)
	api.POST("/settings/logo", sh.UploadLogo)
	api.DELETE("/settings/logo", sh.DeleteLogo)

	// ── 8. Frontend (embedded) ───────────────────────────────────────────────
	// The React build is compiled directly into the binary via //go:embed.
	// This eliminates all external path resolution and installer packaging issues.
	distFS, err := fs.Sub(embeddedFrontend.FS, "dist")
	if err != nil {
		log.Fatalf("FATAL: embedded frontend FS is broken: %v", err)
	}

	if assetsFS, err := fs.Sub(distFS, "assets"); err == nil {
		r.StaticFS("/assets", http.FS(assetsFS))
	}

	// serveEmbedded reads a file from the embedded FS and writes it directly.
	// We intentionally avoid c.FileFromFS / http.FileServer here because
	// http.FileServer issues redirects (e.g. "index.html" → "/index.html")
	// that cause ERR_TOO_MANY_REDIRECTS in the browser.
	serveEmbedded := func(c *gin.Context, path, contentType string) {
		data, err := fs.ReadFile(distFS, path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, contentType, data)
	}

	r.GET("/favicon.ico", func(c *gin.Context) { serveEmbedded(c, "favicon.ico", "image/x-icon") })
	r.GET("/logo.svg", func(c *gin.Context) { serveEmbedded(c, "logo.svg", "image/svg+xml") })
	fmt.Println("LedgerAlps: serving embedded frontend")

	// SPA fallback: all non-API routes serve index.html for client-side routing.
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/health") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		serveEmbedded(c, "index.html", "text/html; charset=utf-8")
	})

	// L'attestation d'intégrité s'émet seule, au démarrage puis chaque jour.
	//
	// La chaîne d'empreintes rend une modification détectable À CONDITION d'avoir
	// un point de comparaison : qui peut écrire dans la base peut recalculer la
	// chaîne entière, qui reste alors cohérente. L'ancrage est l'empreinte de
	// tête conservée ailleurs, à une date connue — et une garantie qui suppose
	// qu'on pense à cliquer chaque mois n'existe pas.
	//
	// Le fichier est déposé à côté des sauvegardes : il part donc avec elles
	// vers le NAS ou la clé USB, et c'est ce déplacement qui vaut ancrage.
	attestationCtx, arreterAttestation := context.WithCancel(context.Background())
	defer arreterAttestation()
	alh.StartAttestationScheduler(attestationCtx, config.AppDataDir())

	// ── 9. Start ──────────────────────────────────────────────────────────────
	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	srv := &http.Server{Addr: addr, Handler: r}

	certFile, keyFile, tlsErr := resolveTLS(cfg)
	if tlsErr != nil {
		log.Fatalf("FATAL: %v", tlsErr)
	}

	serveErr := make(chan error, 1)
	go func() {
		scheme := "http"
		if certFile != "" {
			scheme = "https"
		}
		fmt.Printf("LedgerAlps: listening on %s://%s\n", scheme, addr)

		var err error
		if certFile != "" {
			err = srv.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	// Ctrl-C and service stop take the same path as a restart, minus the
	// relaunch: stop serving, then release the database cleanly.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErr:
		log.Fatalf("FATAL: server error: %v", err)

	case <-stop:
		fmt.Println("LedgerAlps: arrêt demandé…")
		shutdown(srv, database)

	case <-restartCh:
		fmt.Println("LedgerAlps: redémarrage pour appliquer la restauration…")
		// Let the HTTP response reach the browser: the user is looking at a
		// page that is about to reload itself.
		time.Sleep(300 * time.Millisecond)
		shutdown(srv, database)

		// Only now is the database file free. The replacement applies the
		// staged restore before opening it, which it could not do while this
		// process still held the handle.
		if err := relaunch(); err != nil {
			log.Fatalf("FATAL: impossible de redémarrer LedgerAlps: %v\n"+
				"  La restauration reste préparée : fermez puis rouvrez l'application.", err)
		}
	}
}

// shutdown stops serving, then closes the database — in that order, so no
// request is still using it when it goes.
func shutdown(srv *http.Server, database *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("WARNING: arrêt du serveur HTTP: %v", err)
	}
	if err := database.Close(); err != nil {
		log.Printf("WARNING: fermeture de la base: %v", err)
	}
}

// relaunch starts a fresh copy of this binary with the same environment and
// arguments, then returns so the caller can exit.
//
// The child is deliberately not waited on: this process is about to disappear
// and the new one has to outlive it.
func relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("chemin de l'exécutable: %w", err)
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// resolveTLS decides how this instance serves, and refuses the combination
// that silently leaks credentials.
//
// Loopback: plain HTTP. The traffic never reaches a cable, so TLS would buy
// nothing and cost a certificate warning at every start.
//
// Any other interface means another machine can reach LedgerAlps, and with it
// the login password, the session token and the backup passphrase. There TLS is
// the default rather than an option: a supplied certificate is used, and
// failing that one is generated. Until v1.4.5 the server bound to every
// interface and served all of it in clear, without anyone having chosen that.
//
// ALLOW_INSECURE_HTTP covers the one legitimate case — a reverse proxy
// terminating TLS on the same host — and says so loudly in the log, because
// that same flag is what someone reaches for to make a warning go away.
func resolveTLS(cfg *config.Config) (certFile, keyFile string, err error) {
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		return cfg.TLSCert, cfg.TLSKey, nil
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return "", "", fmt.Errorf("TLS_CERT et TLS_KEY doivent être fournis ensemble")
	}
	// Loopback stays plain HTTP. The traffic never reaches a cable, so TLS buys
	// nothing an attacker with local code access could not already bypass — and
	// costs a certificate warning that trains people to click through them.
	// Serving TLS here was offered for one pre-release and withdrawn: it bought
	// no real security and spent trust in warnings.
	if tlsutil.IsLoopback(cfg.Host) {
		return "", "", nil
	}

	if cfg.AllowInsecureHTTP {
		log.Printf("WARNING: LedgerAlps sert en HTTP NON CHIFFRÉ sur %s. "+
			"Mot de passe de connexion, jeton de session et phrase de passe de sauvegarde "+
			"traversent le réseau en clair. À n'utiliser que derrière un reverse proxy TLS "+
			"sur la même machine.", cfg.Host)
		return "", "", nil
	}

	dir := filepath.Join(config.AppDataDir(), "tls")
	certFile, keyFile, err = tlsutil.EnsureSelfSigned(dir, tlsutil.LocalHostnames())
	if err != nil {
		return "", "", fmt.Errorf("LedgerAlps est configuré pour être joignable depuis le réseau (HOST=%s) "+
			"mais aucun certificat TLS n'a pu être préparé: %w\n"+
			"  Fournissez TLS_CERT et TLS_KEY, revenez à HOST=127.0.0.1, "+
			"ou acceptez explicitement le clair avec ALLOW_INSECURE_HTTP=true", cfg.Host, err)
	}
	log.Printf("LedgerAlps: certificat auto-signé utilisé (%s). "+
		"Votre navigateur affichera un avertissement à la première visite — c'est attendu. "+
		"Pour l'éviter, fournissez TLS_CERT et TLS_KEY.", certFile)
	return certFile, keyFile, nil
}
