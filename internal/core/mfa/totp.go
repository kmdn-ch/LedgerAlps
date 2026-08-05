// Package mfa implémente le second facteur par code temporaire (TOTP,
// RFC 6238).
//
// # Pourquoi TOTP et pas autre chose
//
// Le SMS demande un opérateur, un numéro, et un appel sortant — trois choses
// que LedgerAlps n'a pas et ne veut pas. Le courriel a les mêmes défauts et une
// faiblesse de plus : le compte de messagerie est souvent celui qu'on cherche
// justement à protéger. Une clé matérielle (WebAuthn) serait plus solide, mais
// exige HTTPS et un matériel que la plupart des PME suisses n'ont pas.
//
// TOTP fonctionne hors ligne, avec n'importe quelle application
// d'authentification — y compris libre : Aegis, KeePassXC, FreeOTP. Le secret
// ne quitte jamais la machine et l'application du téléphone ; aucun tiers n'est
// dans la boucle. C'est le seul second facteur compatible avec la promesse du
// produit.
//
// # Implémenté ici plutôt qu'importé
//
// L'algorithme tient en quarante lignes et la RFC publie des vecteurs de test
// officiels, qui sont vérifiés dans totp_test.go. Une dépendance de plus dans
// un binaire qui se veut auditable coûte plus qu'elle ne rapporte : elle
// ajoute du code que personne ne relit pour économiser du code que tout le
// monde peut vérifier contre une norme.
package mfa

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Period est la fenêtre d'un code, en secondes. Trente : la valeur que
	// toutes les applications d'authentification supposent, et qu'aucune ne
	// laisse configurer.
	Period = 30
	// Digits : six chiffres, comme partout ailleurs. Huit seraient plus longs à
	// deviner mais aucune application grand public ne les affiche.
	Digits = 6
	// SecretBytes : 20 octets, la taille recommandée par la RFC 4226 pour
	// HMAC-SHA1.
	SecretBytes = 20
)

// Skew est le nombre de fenêtres acceptées de part et d'autre de l'heure
// courante.
//
// Une seule : l'horloge d'un téléphone dérive rarement de plus de trente
// secondes, et chaque fenêtre supplémentaire multiplie les codes valides à un
// instant donné. Zéro rendrait le produit inutilisable — un code saisi à la
// vingt-neuvième seconde arriverait après son expiration.
const Skew = 1

// NewSecret tire un secret partagé.
func NewSecret() (string, error) {
	b := make([]byte, SecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("génération du secret: %w", err)
	}
	// Base32 sans remplissage : c'est ce que les applications
	// d'authentification savent lire, et le remplissage « = » casse plusieurs
	// d'entre elles quand il se retrouve dans une URI.
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// Code calcule le code attendu pour un secret et un instant.
func Code(secret string, at time.Time) (string, error) {
	return codeForCounter(secret, uint64(at.Unix()/Period))
}

func codeForCounter(secret string, counter uint64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", fmt.Errorf("secret illisible: %w", err)
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Troncature dynamique, RFC 4226 §5.3.
	offset := sum[len(sum)-1] & 0x0F
	value := (uint32(sum[offset])&0x7F)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < Digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", Digits, value%mod), nil
}

// Verify vérifie un code et rend la fenêtre qui l'a accepté.
//
// La fenêtre est rendue pour que l'appelant la conserve et refuse sa
// réutilisation. Sans cela, un code intercepté — regardé par-dessus l'épaule,
// lu dans un journal — resterait utilisable pendant sa minute de validité, ce
// qui vide le second facteur d'une partie de son sens.
//
// La comparaison est à temps constant : un code se devine chiffre par chiffre
// si le temps de réponse trahit le nombre de chiffres justes.
func Verify(secret, code string, at time.Time, lastUsedWindow int64) (window int64, ok bool) {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != Digits {
		return 0, false
	}
	current := at.Unix() / Period
	for delta := int64(-Skew); delta <= Skew; delta++ {
		w := current + delta
		if w <= lastUsedWindow {
			continue // déjà consommée : un code ne sert qu'une fois
		}
		expected, err := codeForCounter(secret, uint64(w))
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return w, true
		}
	}
	return 0, false
}

// ProvisioningURI construit l'URI otpauth:// que lit une application
// d'authentification.
//
// L'émetteur apparaît deux fois — dans le chemin et en paramètre — parce que
// les applications ne lisent pas toutes le même : omettre l'un des deux fait
// apparaître le compte sans nom d'application dans certaines listes, ce qui
// rend le bon code introuvable quand on en a plusieurs.
func ProvisioningURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(Period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
