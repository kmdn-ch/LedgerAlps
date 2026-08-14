package middleware

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kmdn-ch/ledgeralps/internal/i18n"
)

// Langue traduit les messages destinés à l'utilisateur, à la sortie.
//
// # Pourquoi ici, et pas dans les gestionnaires
//
// Deux cents endroits produisent un refus, et une bonne moitié vient d'une
// couche plus basse — un service qui rend une `error` que le gestionnaire se
// contente de renvoyer. Traduire à la source aurait demandé de porter la
// langue de la requête jusque dans la comptabilité, où elle n'a rien à faire :
// un service qui sait équilibrer une écriture n'a pas à savoir en quelle langue
// on la lui demande.
//
// Surtout, le motif corrigé côté frontend se serait répété : une route ajoutée
// demain aurait été oubliée. Ici la couverture ne dépend de la vigilance de
// personne — tout ce qui sort en JSON passe par cette fonction.
//
// # Ce qui est traduit, et rien d'autre
//
// Les seuls champs `error` et `message` d'un objet JSON de premier niveau.
// C'est la convention de toutes les réponses du produit, et la restreindre
// évite de toucher à une donnée qui contiendrait par hasard une phrase connue
// — le nom d'un contact, la description d'une ligne de facture.
//
// # Ce qui n'est pas mis en mémoire tampon
//
// Tout ce qui n'est pas `application/json` traverse sans être touché : les
// PDF, les archives ZIP, les CSV. Une facture de dix mégaoctets n'a aucune
// raison de passer par la mémoire pour qu'on y cherche un mot.
func Langue() gin.HandlerFunc {
	return func(c *gin.Context) {
		lang := i18n.Langue(c)
		if lang == i18n.FR {
			// Rien à faire : le français est la langue des sources.
			c.Next()
			return
		}

		tampon := &écrivainTraduisant{ResponseWriter: c.Writer, lang: lang}
		c.Writer = tampon
		c.Next()
		tampon.vider()
	}
}

// écrivainTraduisant retient le corps le temps de le relire.
//
// # Décider au premier OCTET, pas à l'en-tête
//
// C'est le piège de ce fichier, et il a coûté un aller-retour : gin appelle
// `WriteHeader` AVANT de poser le `Content-Type`. Son chemin de rendu fait
// `c.Status(code)` — donc `WriteHeader` — puis `WriteContentType(w)`, puis
// écrit le corps. Regarder le type de contenu depuis `WriteHeader` le trouve
// donc toujours vide : la réponse traversait intacte, et le premier essai sur
// un vrai serveur rendait du français en demandant de l'allemand.
//
// La décision est donc prise au premier appel à `Write`, où l'en-tête est
// posé. Le statut, lui, passe immédiatement : `gin.responseWriter` ne fait
// que l'enregistrer et ne l'émet qu'au premier octet — le retenir nous-mêmes
// ne servirait à rien et risquerait de le perdre.
type écrivainTraduisant struct {
	gin.ResponseWriter
	lang    i18n.Lang
	corps   bytes.Buffer
	retient bool
	décidé  bool
}

// décide regarde le type de contenu, une seule fois, au premier octet.
func (w *écrivainTraduisant) décide() {
	if w.décidé {
		return
	}
	w.décidé = true
	w.retient = strings.HasPrefix(w.Header().Get("Content-Type"), "application/json")
	if w.retient {
		// Traduire change la longueur du corps : une valeur écrite par un
		// gestionnaire serait fausse.
		w.Header().Del("Content-Length")
	}
}

func (w *écrivainTraduisant) Write(b []byte) (int, error) {
	w.décide()
	if w.retient {
		return w.corps.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *écrivainTraduisant) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// vider écrit le corps, traduit si c'est du JSON.
func (w *écrivainTraduisant) vider() {
	if !w.retient {
		return
	}
	corps := w.corps.Bytes()
	if traduit, ok := traduireJSON(w.lang, corps); ok {
		corps = traduit
	}
	_, _ = w.ResponseWriter.Write(corps)
}

// traduireJSON remplace les champs `error` et `message` d'un objet.
//
// Rend `false` si le corps n'est pas un objet JSON ou ne porte aucun de ces
// champs — auquel cas l'original part tel quel, à l'octet près. Réencoder pour
// rien changerait l'ordre des clés et la mise en forme sans aucun bénéfice.
func traduireJSON(l i18n.Lang, corps []byte) ([]byte, bool) {
	if len(corps) == 0 || corps[0] != '{' {
		return nil, false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(corps, &obj); err != nil {
		return nil, false
	}

	changé := false
	for _, champ := range [...]string{"error", "message"} {
		brut, ok := obj[champ]
		if !ok {
			continue
		}
		var texte string
		if err := json.Unmarshal(brut, &texte); err != nil {
			continue // le champ n'est pas une chaîne : on n'y touche pas
		}
		traduit := i18n.Traduire(l, texte)
		if traduit == texte {
			continue
		}
		encodé, err := json.Marshal(traduit)
		if err != nil {
			continue
		}
		obj[champ] = encodé
		changé = true
	}
	if !changé {
		return nil, false
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return out, true
}

// Assure que le type continue de satisfaire l'interface de gin.
var _ gin.ResponseWriter = (*écrivainTraduisant)(nil)
