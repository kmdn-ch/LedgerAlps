package i18n

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// La langue vient de la requête, et « de-CH » vaut « de ».
//
// Le repli sur le français est le comportement par défaut : une langue
// inconnue ne doit jamais produire une page vide ou une clé nue.
func TestLangueDeLaRequête(t *testing.T) {
	cas := []struct {
		entête, requête string
		attendu         Lang
	}{
		{"", "", FR},
		{"de", "", DE},
		{"de-CH", "", DE},
		{"de-CH,de;q=0.9,fr;q=0.8", "", DE},
		{"en-GB", "", EN},
		{"it_CH", "", IT},
		{"rm", "", FR},   // le romanche n'est pas traduit : repli
		{"xx", "", FR},   // valeur absurde : repli
		{"de", "it", IT}, // le paramètre l'emporte sur l'en-tête
	}
	for _, c := range cas {
		gin.SetMode(gin.TestMode)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		cible := "/x"
		if c.requête != "" {
			cible += "?lang=" + c.requête
		}
		ctx.Request = httptest.NewRequest("GET", cible, nil)
		if c.entête != "" {
			ctx.Request.Header.Set("Accept-Language", c.entête)
		}
		if got := Langue(ctx); got != c.attendu {
			t.Errorf("Accept-Language %q, lang=%q : %q au lieu de %q",
				c.entête, c.requête, got, c.attendu)
		}
	}
}

// Une phrase du catalogue ressort traduite ; une phrase inconnue ressort
// telle quelle, c'est-à-dire en français.
func TestTraduireLesPhrasesConnues(t *testing.T) {
	const fr = "aucune facture sélectionnée"
	if got := Traduire(DE, fr); got == fr {
		t.Errorf("%q n'a pas été traduit en allemand", fr)
	}
	if got := Traduire(FR, fr); got != fr {
		t.Errorf("le français a été modifié : %q", got)
	}
	const inconnue = "une phrase que personne n'a jamais écrite ici"
	if got := Traduire(DE, inconnue); got != inconnue {
		t.Errorf("une phrase inconnue doit ressortir intacte, obtenu %q", got)
	}
}

// LE test qui compte pour les messages à valeurs.
//
// « le compte 1020 n'existe pas… » doit retrouver son moule et rendre la
// traduction AVEC le 1020 à la bonne place. C'est le cas que la recherche
// exacte manque, et ce sont les messages les plus utiles — ceux qui disent
// lequel.
func TestTraduireRéinjecteLesValeurs(t *testing.T) {
	cas := []struct{ message, doitContenir string }{
		{"le compte 1020 est désactivé", "1020"},
		{"le montant doit valoir au moins 0.01, reçu 0.00", "0.00"},
		{"un IBAN CH compte 21 caractères, celui-ci en a 19", "19"},
		{"la phrase de passe doit compter au moins 16 caractères (4 saisis)", "16"},
	}
	for _, c := range cas {
		got := Traduire(DE, c.message)
		if got == c.message {
			t.Errorf("%q n'a pas été traduit", c.message)
			continue
		}
		if !strings.Contains(got, c.doitContenir) {
			t.Errorf("la valeur %q a disparu de la traduction : %q", c.doitContenir, got)
		}
	}
}

// Un message qui annonce puis colle une cause technique : l'annonce se
// traduit, la cause reste intacte — c'est elle qu'on cherchera dans les
// sources.
func TestTraduireGardeLaCauseTechnique(t *testing.T) {
	const cause = "open /var/db: permission denied"
	got := Traduire(DE, "les comptes n'ont pas pu être lus : "+cause)
	if !strings.HasSuffix(got, cause) {
		t.Errorf("la cause technique a été perdue : %q", got)
	}
	if strings.HasPrefix(got, "les comptes") {
		t.Errorf("l'annonce n'a pas été traduite : %q", got)
	}
}

// Les verbes de format doivent être les MÊMES, dans le même ordre, dans les
// quatre langues.
//
// Les valeurs sont réinjectées par position : un verbe déplacé afficherait le
// montant à la place du numéro de compte, et un verbe en moins avalerait la
// valeur sans que rien ne le signale. C'est le défaut le plus coûteux de ce
// paquet, parce qu'il produit une phrase parfaitement lisible et fausse.
func TestLesVerbesDeFormatSontConservés(t *testing.T) {
	re := regexp.MustCompile(`%[+\-# 0]*[0-9]*(?:\.[0-9]+)?[a-zA-Z]`)
	for _, fr := range Phrases() {
		attendus := re.FindAllString(fr, -1)
		if len(attendus) == 0 {
			continue
		}
		for _, l := range []Lang{DE, IT, EN} {
			obtenus := re.FindAllString(Traductions(fr)[l], -1)
			if len(obtenus) != len(attendus) {
				t.Errorf("%s : %q porte %v, le français porte %v",
					l, court(fr), obtenus, attendus)
				continue
			}
			for i := range attendus {
				if obtenus[i] != attendus[i] {
					t.Errorf("%s : %q — verbe %d vaut %s au lieu de %s",
						l, court(fr), i+1, obtenus[i], attendus[i])
				}
			}
		}
	}
}

// Aucune traduction ne doit être vide, ni rester la phrase source.
//
// # Pourquoi l'anglais échappe au contrôle
//
// Une poignée de messages étaient DÉJÀ en anglais dans les sources et
// atteignaient l'utilisateur tels quels — « journal entry not found » sur un
// écran français. Ils sont au catalogue pour que l'allemand et l'italien
// existent ; leur « traduction » anglaise est légitimement identique à la
// source.
//
// L'allemand et l'italien, eux, ne peuvent jamais coïncider avec une phrase
// source de plus de vingt-cinq caractères, qu'elle soit française ou anglaise.
// C'est donc sur eux que porte le contrôle du copier-coller oublié.
func TestAucuneTraductionNestVideNiSource(t *testing.T) {
	for _, fr := range Phrases() {
		trads := Traductions(fr)
		for _, l := range []Lang{DE, IT, EN} {
			if strings.TrimSpace(trads[l]) == "" {
				t.Errorf("%s : %q est vide", l, court(fr))
			}
		}
		for _, l := range []Lang{DE, IT} {
			if trads[l] == fr && len(fr) > 25 {
				t.Errorf("%s : %q vaut encore la phrase source", l, court(fr))
			}
		}
	}
}

func court(s string) string {
	if len(s) > 60 {
		return s[:60] + "…"
	}
	return s
}
