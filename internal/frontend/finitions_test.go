package frontend

// Audit croisé Gemini, finitions (P3) : l'interface (MED-02, MED-15).
//
// Comme les autres tests de ce paquet, ils lisent les sources : TypeScript trouve valide un
// téléchargement qui coupe sous Firefox, et valide un texte français affiché sur une interface
// allemande.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sourcesTS(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	racine := filepath.Join("..", "..", "frontend", "src")
	err := filepath.Walk(racine, func(chemin string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if strings.HasSuffix(chemin, ".ts") || strings.HasSuffix(chemin, ".tsx") {
			b, e := os.ReadFile(chemin)
			if e != nil {
				return e
			}
			rel, _ := filepath.Rel(racine, chemin)
			out[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	if err != nil || len(out) < 50 {
		t.Fatalf("sources illisibles (%d fichiers) : %v", len(out), err)
	}
	return out
}

// MED-15. Révoquer l'adresse d'un fichier AUSSITÔT après le clic coupe le téléchargement sous
// Firefox et Safari, où il démarre de façon asynchrone. Cinq écrans recopiaient ce motif ; ils
// passent tous par `downloadBlob`, qui attend. Seul l'aperçu PDF révoque, et c'est l'adresse
// PRÉCÉDENTE, quand il en affiche une nouvelle.
func TestAucunEcranNeRevoqueLAdresseDUnTelechargementAussitot(t *testing.T) {
	for chemin, src := range sourcesTS(t) {
		if chemin == "components/ui/PDFPreview.tsx" {
			continue
		}
		for i, ligne := range strings.Split(src, "\n") {
			if strings.Contains(ligne, "revokeObjectURL(") && !strings.Contains(ligne, "setTimeout(") {
				t.Errorf("%s:%d révoque l'adresse sans attendre — passer par downloadBlob (api/client.ts) : %s",
					chemin, i+1, strings.TrimSpace(ligne))
			}
		}
		if chemin != "api/client.ts" && chemin != "components/ui/PDFPreview.tsx" &&
			strings.Contains(src, "URL.createObjectURL(") {
			t.Errorf("%s fabrique une adresse de fichier lui-même : downloadBlob (api/client.ts) le fait, et attend avant de la révoquer", chemin)
		}
	}
	if !strings.Contains(sourcesTS(t)["api/client.ts"], "setTimeout(() => URL.revokeObjectURL(url)") {
		t.Error("downloadBlob ne retarde plus la révocation de l'adresse")
	}
}

// MED-02. L'écran du chiffre d'affaires affichait le texte `basis` du serveur, en français quelle
// que soit la langue. Il lit maintenant des clés de traduction — dont l'avertissement quand des
// devises différentes sont additionnées.
func TestLEcranDuChiffreDAffairesTraduitSaConventionEtAvertitDesDevises(t *testing.T) {
	panneau := sourcesTS(t)["components/dashboard/RevenuePanel.tsx"]
	if strings.Contains(panneau, "revenue.data.basis") {
		t.Error("RevenuePanel affiche encore le texte français du serveur (basis)")
	}
	for _, cle := range []string{"ca.base", "ca.noteEncaisse", "ca.devisesMelangees"} {
		if !strings.Contains(panneau, "t('"+cle+"'") {
			t.Errorf("RevenuePanel n'utilise plus la clé %s", cle)
		}
		for _, langue := range []string{"fr", "de", "it", "en"} {
			if !strings.Contains(sourcesTS(t)["i18n/"+langue+".ts"], "'"+cle+"':") {
				t.Errorf("la clé %s manque en %s", cle, langue)
			}
		}
	}
	if !strings.Contains(panneau, "currencies") {
		t.Error("RevenuePanel ne lit plus la liste des devises")
	}
}
