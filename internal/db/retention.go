package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

// Rétention des données personnelles — nLPD (RS 235.1) art. 6 al. 4.
//
// « Les données personnelles doivent être détruites ou anonymisées dès qu'elles
// ne sont plus nécessaires au regard des finalités du traitement. »
//
// La table `security_events` porte depuis sa création un commentaire annonçant
// qu'elle est « retention-limited ». Rien ne l'appliquait : les adresses IP des
// tentatives de connexion bloquées s'accumulaient sans terme. Une adresse IP est
// une donnée personnelle, et un commentaire décrivant une garantie inexistante
// est pire que l'absence de garantie — il empêche de voir le problème.
//
// Deux durées, parce que les deux informations ne servent pas à la même chose :
//
//   - l'**adresse** répond à « d'où venait cette tentative », question qui perd
//     tout intérêt passé quelques mois et dont la réponse est une donnée
//     personnelle ;
//   - le **fait** qu'un verrouillage ait eu lieu répond à « suis-je ciblé »,
//     question qui garde du sens plus longtemps et dont la réponse, une fois
//     l'adresse retirée, n'est plus personnelle.
//
// D'où un traitement en deux temps : anonymiser d'abord, supprimer ensuite.
// Détruire tout de suite ferait disparaître un signal de sécurité sans
// nécessité, et la nLPD demande la minimisation, pas l'amnésie.

const (
	// IPRetention — au-delà, l'adresse est retirée mais l'événement subsiste.
	IPRetention = 90 * 24 * time.Hour
	// EventRetention — au-delà, l'événement lui-même disparaît.
	EventRetention = 365 * 24 * time.Hour
)

// RetentionReport décrit ce qu'une passe a fait, pour que l'écran de conformité
// puisse l'afficher plutôt que de demander qu'on le croie sur parole.
type RetentionReport struct {
	IPsAnonymised int64 `json:"ips_anonymised"`
	EventsDeleted int64 `json:"events_deleted"`
}

// ApplyRetention exécute une passe de rétention. Idempotente : une seconde
// exécution immédiate ne touche rien.
func ApplyRetention(database *sql.DB, usePostgres bool, now time.Time) (RetentionReport, error) {
	var rep RetentionReport

	// 1. Retirer l'adresse des événements plus anciens que IPRetention. Le
	//    marqueur explicite vaut mieux qu'un NULL : il distingue « purgée » de
	//    « jamais enregistrée », ce qui évite de conclure à un bug.
	anonQ := Rebind(`
		UPDATE security_events
		SET ip_address = '(anonymisée)'
		WHERE created_at < ? AND ip_address IS NOT NULL AND ip_address <> '(anonymisée)'`,
		usePostgres)
	res, err := database.Exec(anonQ, now.Add(-IPRetention))
	if err != nil {
		return rep, fmt.Errorf("anonymise security events: %w", err)
	}
	rep.IPsAnonymised, _ = res.RowsAffected()

	// 2. Supprimer les événements plus anciens que EventRetention. Ils ne
	//    contiennent alors plus d'adresse ; c'est la ligne elle-même qui n'a
	//    plus d'utilité.
	delQ := Rebind(`DELETE FROM security_events WHERE created_at < ?`, usePostgres)
	res, err = database.Exec(delQ, now.Add(-EventRetention))
	if err != nil {
		return rep, fmt.Errorf("delete security events: %w", err)
	}
	rep.EventsDeleted, _ = res.RowsAffected()

	if rep.IPsAnonymised > 0 || rep.EventsDeleted > 0 {
		log.Printf("[rétention] nLPD art. 6 al. 4 : %d adresse(s) IP anonymisée(s), %d événement(s) supprimé(s)",
			rep.IPsAnonymised, rep.EventsDeleted)
	}
	return rep, nil
}
