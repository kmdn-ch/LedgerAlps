package handlers

// Émettre l'attestation d'intégrité toute seule, et la poser sur le disque.
//
// # Ce que la chaîne d'empreintes ne peut pas faire seule
//
// Elle rend toute modification détectable — à condition de disposer d'un point
// de comparaison. Quelqu'un capable d'écrire dans la base peut recalculer la
// chaîne ENTIÈRE : elle reste alors cohérente, simplement différente. Rien,
// dans le fichier, ne dit qu'elle a été refaite.
//
// Ce qui referme cette faille est un ANCRAGE : une empreinte de tête conservée
// ailleurs, à une date connue. Réécrire l'histoire produit alors une empreinte
// qui ne correspond plus, et l'écart est démontrable à un tiers.
//
// # Pourquoi automatiquement
//
// L'attestation existait, téléchargeable à la demande. Une garantie qui suppose
// que quelqu'un pense à cliquer chaque mois n'existe pas : le jour où elle
// servirait, la dernière date d'un an. Elle est donc émise seule, à chaque
// démarrage puis une fois par jour.
//
// # Ce que cela ne fait PAS
//
// Rien n'est envoyé nulle part. LedgerAlps ne parle à aucun service extérieur,
// et un horodatage tiers (RFC 3161) supposerait un appel réseau — contraire à
// la promesse du produit. Le fichier est déposé à côté des sauvegardes, donc
// il PART AVEC ELLES vers le NAS ou la clé USB. C'est ce déplacement, décidé
// par l'utilisateur, qui constitue l'ancrage. L'écran le dit.

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AttestationDir est le sous-dossier des attestations, à côté des sauvegardes.
const AttestationDir = "attestations"

// attestationKeep borne le nombre de fichiers conservés sur le poste.
//
// Une attestation par jour pendant dix ans ferait 3650 fichiers pour une
// question à laquelle trois suffisent. Les anciennes ne se perdent pas pour
// autant : chaque sauvegarde emporte celles du jour, et ce sont les copies
// sorties de la machine qui comptent.
const attestationKeep = 90

// EmitAttestationFile écrit l'attestation du jour et rend son chemin.
//
// Le nom porte la date : deux émissions le même jour écrasent le même fichier,
// ce qui est voulu. La dernière de la journée est la plus complète, et garder
// vingt versions d'un même constat n'apprend rien.
func (h *AuditHandler) EmitAttestationFile(ctx context.Context, dataDir string) (string, error) {
	dir := filepath.Join(dataDir, AttestationDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("dossier des attestations: %w", err)
	}

	// « LedgerAlps (émission automatique) » plutôt qu'un nom d'utilisateur :
	// personne n'a demandé ce document, et l'attribuer à quelqu'un qui dormait
	// serait faux. L'attestation dit ce qu'elle est.
	final, err := h.BuildAttestation(ctx, "LedgerAlps — émission automatique")
	if err != nil {
		return "", fmt.Errorf("production de l'attestation: %w", err)
	}

	nom := fmt.Sprintf("attestation-integrite-%s.json", time.Now().UTC().Format("2006-01-02"))
	chemin := filepath.Join(dir, nom)

	// Écriture par fichier temporaire puis renommage : une coupure au milieu
	// laisserait sinon une attestation tronquée, qui a l'air d'un document et
	// n'en est pas un.
	tmp := chemin + ".partiel"
	if err := os.WriteFile(tmp, final, 0o600); err != nil {
		return "", fmt.Errorf("écriture: %w", err)
	}
	if err := os.Rename(tmp, chemin); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("renommage: %w", err)
	}

	pruneAttestations(dir)
	return chemin, nil
}

// pruneAttestations garde les plus récentes et retire le reste.
//
// Silencieuse en cas d'échec : ne pas réussir à faire le ménage n'est pas une
// raison de refuser d'écrire l'attestation du jour.
func pruneAttestations(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var noms []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, "attestation-integrite-") &&
			strings.HasSuffix(n, ".json") {
			noms = append(noms, n)
		}
	}
	if len(noms) <= attestationKeep {
		return
	}
	// Le nom porte la date en ISO : l'ordre alphabétique EST l'ordre
	// chronologique, sans lire les dates du système de fichiers — qu'une copie
	// ou une restauration remettrait toutes à la même valeur.
	sort.Strings(noms)
	for _, n := range noms[:len(noms)-attestationKeep] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}

// StartAttestationScheduler émet l'attestation au démarrage, puis chaque jour.
//
// L'émission au démarrage compte autant que la périodique : un poste éteint la
// nuit et rallumé le matin ne verrait jamais passer une échéance nocturne.
func (h *AuditHandler) StartAttestationScheduler(ctx context.Context, dataDir string) {
	emettre := func() {
		// Délai propre à l'émission : la vérification de chaîne parcourt tout le
		// journal, et un poste chargé peut y passer un moment.
		c, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		chemin, err := h.EmitAttestationFile(c, dataDir)
		if err != nil {
			// Journalisé, jamais fatal : un défaut d'attestation ne doit pas
			// empêcher de tenir sa comptabilité.
			log.Printf("WARNING: attestation d'intégrité non émise: %v", err)
			return
		}
		fmt.Printf("LedgerAlps: attestation d'intégrité écrite — %s\n", chemin)
	}

	go func() {
		emettre()
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				emettre()
			}
		}
	}()
}
