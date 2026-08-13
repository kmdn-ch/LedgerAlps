package handlers

// Vérifier une attestation d'intégrité.
//
// # Ce qui manquait
//
// L'attestation était produite, signée d'une empreinte, et remise à un tiers
// — qui n'avait aucun moyen de la vérifier. Un document qu'on ne peut pas
// contrôler ne vaut pas mieux qu'une affirmation orale : il documentait l'état
// d'un mécanisme, et il fallait le croire sur parole.
//
// # Ce que la vérification établit, et ce qu'elle n'établit pas
//
// Trois contrôles, de valeur inégale, et il faut le dire :
//
//  1. LE SCEAU. `self_hash` couvre le reste du document. Il détecte une
//     modification faite après coup par quelqu'un qui n'a pas l'outil — une
//     ligne changée dans un éditeur de texte. Il ne prouve RIEN contre qui
//     dispose du logiciel : recalculer l'empreinte est à la portée du premier
//     script venu. C'est un scellé de transport, pas une signature.
//
//  2. LA CORRESPONDANCE. L'empreinte de tête de l'attestation est comparée à
//     celle que portent les livres AUJOURD'HUI, au même numéro de séquence.
//     C'est le contrôle qui compte : une attestation remise en janvier et
//     conservée par la fiduciaire prouve, en juin, qu'aucune écriture couverte
//     n'a été réécrite entre-temps. Le pouvoir de preuve vient de ce que la
//     fiduciaire détient une copie que le client ne peut plus modifier.
//
//  3. L'ÉTAT ACTUEL. La chaîne est reparcourue en entier. Une attestation
//     ancienne peut correspondre alors que les livres ont rompu depuis.
//
// # Pourquoi c'est le SERVEUR qui vérifie
//
// Parce que seul lui a les livres. La fiduciaire qui reçoit le fichier ne peut
// contrôler que le sceau — et pour cela, elle n'a besoin d'aucun logiciel :
// la marche à suivre est écrite dans l'attestation elle-même, avec des outils
// que tout poste possède. La comparaison des empreintes, elle, se fait chez le
// client, devant lui, sur ses livres.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kmdn-ch/ledgeralps/internal/db"
	"github.com/kmdn-ch/ledgeralps/internal/i18n"
)

// Verdict d'une vérification, rendu à l'écran.
type verdictAttestation struct {
	// SceauValide : le document n'a pas été modifié depuis son émission.
	SceauValide bool `json:"seal_valid"`
	// EmpreinteCorrespond : les livres portent toujours la même empreinte au
	// numéro de séquence couvert par l'attestation.
	EmpreinteCorrespond bool `json:"head_matches"`
	// ChaineIntacte : l'état des livres aujourd'hui, indépendamment de
	// l'attestation.
	ChaineIntacte bool `json:"chain_intact"`

	// Favorable dit si le verdict est bon. C'est le SERVEUR qui le décide,
	// parce que c'est lui qui rédige le verdict : laisser l'écran le
	// recalculer avec un ET des trois booléens produisait un cadre rouge sous
	// la phrase « Attestation vérifiée » dès que la chaîne était vide — les
	// deux se contredisaient à l'écran, et c'est la couleur qu'on croit.
	Favorable bool `json:"ok"`
	// RienACouvrir : l'attestation a été émise sur des livres vides. Il n'y a
	// pas d'empreinte à comparer, et annoncer « divergente » serait faux.
	RienACouvrir bool `json:"nothing_covered"`

	EmisLe            string `json:"issued_at"`
	EmisPar           string `json:"issued_by"`
	SequenceCouverte  int64  `json:"covered_sequence"`
	EmpreinteAttestee string `json:"attested_head_hash"`
	EmpreinteActuelle string `json:"current_head_hash"`
	EcrituresDepuis   int    `json:"entries_since"`

	// Verdict est la phrase à lire. Le reste est le détail qui la soutient.
	Verdict string `json:"verdict"`
	Detail  string `json:"detail"`
}

// VerifyAttestation POST /api/v1/audit-logs/attestation/verify
//
// Reçoit une attestation produite par LedgerAlps et rend un verdict.
func (h *AuditHandler) VerifyAttestation(c *gin.Context) {
	lang := i18n.Langue(c)
	t := func(fr string, args ...any) string { return i18n.T(lang, fr, args...) }

	brut, err := lireAttestation(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": t("le fichier d'attestation n'a pas pu être lu")})
		return
	}

	var att Attestation
	if err := json.Unmarshal(brut, &att); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": t("ce fichier n'est pas une attestation LedgerAlps"),
		})
		return
	}
	if att.Document == "" || att.Chain.Algorithm == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": t("ce fichier n'est pas une attestation LedgerAlps"),
		})
		return
	}

	v := verdictAttestation{
		EmisLe:            att.IssuedAt,
		EmisPar:           att.IssuedBy,
		SequenceCouverte:  att.Chain.LastSequence,
		EmpreinteAttestee: att.Chain.HeadHash,
	}
	v.SceauValide = sceauValide(att)

	// L'état des livres, maintenant.
	rapport, err := h.ComputeChainReport(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": t("le contrôle d'intégrité n'a pas pu s'exécuter: ") + err.Error(),
		})
		return
	}
	v.ChaineIntacte = rapport.Verified

	// L'empreinte que portent les livres au numéro de séquence attesté.
	actuelle, trouvee, err := h.HashAtSequence(c.Request.Context(), att.Chain.LastSequence)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": t("le contrôle d'intégrité n'a pas pu s'exécuter: ") + err.Error(),
		})
		return
	}
	v.EmpreinteActuelle = actuelle
	v.EmpreinteCorrespond = trouvee && actuelle == att.Chain.HeadHash
	if rapport.LastSeq > att.Chain.LastSequence {
		v.EcrituresDepuis = int(rapport.LastSeq - att.Chain.LastSequence)
	}

	v.RienACouvrir = v.SceauValide && v.SequenceCouverte == 0 && v.EmpreinteAttestee == ""
	v.Verdict, v.Detail, v.Favorable = rédigerVerdict(t, v, trouvee)
	c.JSON(http.StatusOK, v)
}

// lireAttestation accepte le fichier dans un formulaire ou dans le corps.
//
// Les deux, parce que l'écran envoie un formulaire et qu'un contrôle depuis un
// script est plus simple avec un corps brut — et que rien ne justifie de
// choisir pour l'appelant.
func lireAttestation(c *gin.Context) ([]byte, error) {
	const maxTaille = 4 << 20 // une attestation fait quelques kilooctets

	if f, err := c.FormFile("file"); err == nil {
		fh, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer fh.Close()
		return io.ReadAll(io.LimitReader(fh, maxTaille))
	}
	return io.ReadAll(io.LimitReader(c.Request.Body, maxTaille))
}

// sceauValide recalcule l'empreinte du document sans son propre champ.
//
// La sérialisation doit être identique à celle de l'émission : mêmes champs,
// même ordre, même indentation. C'est `encoding/json` sur la même structure
// qui l'assure — et c'est pourquoi la marche à suivre remise à la fiduciaire
// décrit exactement cette opération.
func sceauValide(att Attestation) bool {
	attendu := att.SelfHash
	if attendu == "" {
		return false
	}
	att.SelfHash = ""
	corps, err := json.MarshalIndent(att, "", "  ")
	if err != nil {
		return false
	}
	somme := sha256.Sum256(corps)
	return hex.EncodeToString(somme[:]) == attendu
}

// rédigerVerdict transforme trois booléens en une phrase qu'on peut lire, et
// dit si cette phrase est une bonne nouvelle.
//
// L'ordre importe : ce qui invalide le document passe avant ce qui invalide
// les livres, et une correspondance sans rupture se dit simplement.
//
// Le troisième retour est là parce que « les trois booléens sont vrais » n'est
// PAS la même chose que « le verdict est bon » : sur une chaîne vide, il n'y a
// pas d'empreinte à faire correspondre, et l'écran affichait un cadre rouge
// sous une phrase rassurante.
func rédigerVerdict(t func(string, ...any) string, v verdictAttestation, trouvée bool) (string, string, bool) {
	switch {
	case !v.SceauValide:
		return t("Ce document a été modifié après son émission"),
			t("Son sceau ne correspond plus à son contenu. Demandez une attestation neuve : celle-ci ne prouve rien."),
			false

	// Une attestation émise sur des livres encore vides ne couvre rien. Dire
	// « la séquence est absente » serait exact et inutile : il n'y a pas de
	// séquence à porter, et l'utilisateur n'a rien à corriger.
	case v.SequenceCouverte == 0 && v.EmpreinteAttestee == "":
		return t("Attestation vérifiée — aucune écriture couverte"),
			t("Le document est intact. Il a été émis alors qu'aucune écriture n'était encore comptabilisée : il n'y a donc rien à comparer. Une attestation ne devient probante qu'une fois les livres alimentés."),
			true

	case !trouvée:
		return t("Les livres ne portent pas la séquence attestée"),
			t("L'attestation couvre des écritures que cette comptabilité ne contient pas. Ce sont deux comptabilités différentes, ou les écritures ont été supprimées."),
			false

	case !v.EmpreinteCorrespond:
		return t("Les livres ont changé depuis cette attestation"),
			t("L'empreinte enregistrée au même numéro de séquence ne correspond plus. Une écriture couverte par cette attestation a été réécrite. Une sauvegarde antérieure est nécessaire pour établir ce qui a bougé."),
			false

	case !v.ChaineIntacte:
		return t("Attestation vérifiée, mais les livres présentent une rupture"),
			t("Les écritures couvertes par cette attestation n'ont pas bougé. La chaîne a en revanche rompu plus loin : voyez Paramètres → Maintenance → Piste d'audit."),
			false

	default:
		return t("Attestation vérifiée"),
			t("Le document est intact et les livres portent toujours la même empreinte au numéro de séquence attesté. Aucune écriture couverte n'a été modifiée depuis l'émission."),
			true
	}
}

// HashAtSequence rend l'empreinte enregistrée à un numéro de séquence.
//
// C'est le cœur de la comparaison : une attestation dit « au maillon 412,
// l'empreinte valait X ». Si les livres portent autre chose à ce maillon,
// une écriture couverte par l'attestation a été réécrite.
//
// Le second retour distingue « la séquence n'existe pas » de « elle existe et
// vaut autre chose ». Les deux sont graves, mais pas de la même façon : le
// premier veut dire que des écritures ont disparu, ou qu'il s'agit d'une autre
// comptabilité.
func (h *AuditHandler) HashAtSequence(ctx context.Context, seq int64) (string, bool, error) {
	q := db.Rebind(
		`SELECT entry_hash FROM audit_logs WHERE sequence_number = ?`, h.usePostgres)
	var hash string
	err := h.db.QueryRowContext(ctx, q, seq).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hash, true, nil
}
