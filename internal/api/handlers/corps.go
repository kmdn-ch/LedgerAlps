package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
)

// maxCorpsFacultatif borne ce qu'on lit d'un corps facultatif : ces routes ne portent que
// quelques dates et lignes, et le plafond d'envoi global est de dix mégaoctets.
const maxCorpsFacultatif = 1 << 20

// errCorpsIllisible : phrase FIXE, rendue par le catalogue par correspondance exacte. Elle ne
// cite pas la position de l'erreur, qui n'apprend rien à l'utilisateur.
var errCorpsIllisible = errors.New("le corps de la requête n'est pas un JSON valide")

// bindOptionalJSON lit un corps JSON FACULTATIF : vide, il laisse `dst` à ses valeurs par
// défaut ; présent, il doit être valide.
//
// # Pourquoi pas `_ = c.ShouldBindJSON(&body)`
//
// L'erreur y était jetée pour tolérer le corps vide — mais elle tolérait du même coup le corps
// MAL FORMÉ, lu comme vide. Sur la note de crédit, « vide » veut dire « créditer la facture en
// entier » : une virgule oubliée dans les lignes à créditer émettait un avoir de 100 % (audit
// croisé Gemini, HIGH-03).
func bindOptionalJSON(c *gin.Context, dst any) error {
	if c.Request == nil || c.Request.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxCorpsFacultatif+1))
	if err != nil {
		return errCorpsIllisible
	}
	if len(raw) > maxCorpsFacultatif {
		return errCorpsIllisible
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(dst); err != nil {
		return errCorpsIllisible
	}
	return nil
}
