package accounting

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kmdn-ch/ledgeralps/internal/core/security"
	"github.com/kmdn-ch/ledgeralps/internal/db"
)

// Ajout d'un maillon à la chaîne d'empreintes du CO art. 957a.
//
// Extrait de PostEntry parce qu'un second chemin en avait besoin : la clôture
// d'exercice insérait son écriture directement, avec `status = 'posted'`, sans
// empreinte et sans entrée d'audit. L'écriture de clôture — celle qui vire le
// résultat, la plus lourde de l'exercice — était donc la seule à échapper à la
// chaîne. Le contrôle de cohérence du produit la signalait déjà comme « écriture
// postée sans empreinte d'intégrité », sans que personne fasse le lien.
//
// Deux implémentations auraient fini par diverger. Il n'y en a plus qu'une.

// AppendAuditEntry écrit le maillon d'audit pour `recordID` et retourne
// l'empreinte chaînée à stocker sur l'écriture, ainsi que l'horodatage retenu.
//
// La lecture du maillon précédent se fait DANS la transaction passée. La faire
// à l'extérieur laissait deux comptabilisations concurrentes lire le même
// prédécesseur et produire deux maillons de même numéro de séquence — une
// chaîne fourchue, que la vérification signalerait comme rompue alors que
// personne n'aurait rien falsifié.
func AppendAuditEntry(
	ctx context.Context,
	tx execQuerier,
	usePostgres bool,
	userID, action, recordID, ipAddress string,
	afterState map[string]any,
) (chainedHash string, at time.Time, err error) {
	return AppendAuditEntryFor(ctx, tx, usePostgres,
		"journal_entries", userID, action, recordID, ipAddress, afterState)
}

// AppendAuditEntryFor écrit un maillon pour n'importe quelle table.
//
// Le nom de la table était figé à « journal_entries », si bien que la chaîne
// d'empreintes ne couvrait QUE le journal. Les factures, les contacts et les
// paiements n'y laissaient aucune trace : ils portaient un created_by_id — qui
// a créé — mais rien sur qui a modifié, transformé ou annulé quoi.
//
// La vérification relit `table_name` sur la ligne et recalcule à partir d'elle :
// ouvrir ce paramètre est donc rétrocompatible, les maillons existants
// continuant de se vérifier contre « journal_entries ».
func AppendAuditEntryFor(
	ctx context.Context,
	tx execQuerier,
	usePostgres bool,
	tableName, userID, action, recordID, ipAddress string,
	afterState map[string]any,
) (chainedHash string, at time.Time, err error) {
	rawAfterJSON, err := json.Marshal(afterState)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("encode after_state: %w", err)
	}
	// Les données personnelles sont masquées AVANT le calcul (nLPD art. 6) :
	// hacher l'état non masqué puis stocker l'état masqué rendrait l'empreinte
	// non recalculable, ce qui est exactement le défaut corrigé en v1.4.6.
	maskedAfter := maskPersonalData(string(rawAfterJSON))
	maskedBefore := maskPersonalData("")

	now := time.Now().UTC().Truncate(time.Second)
	entryHash := security.ComputeEntryHash(
		userID, action, tableName, recordID,
		maskedBefore, maskedAfter, ipAddress, now,
	)

	prevQ := db.Rebind(
		"SELECT entry_hash, sequence_number FROM audit_logs ORDER BY sequence_number DESC LIMIT 1",
		usePostgres)
	var prevHash string
	var lastSeq int64
	if err := tx.QueryRowContext(ctx, prevQ).Scan(&prevHash, &lastSeq); err != nil && err != sql.ErrNoRows {
		return "", time.Time{}, fmt.Errorf("load prev hash: %w", err)
	}

	var prevHashPtr *string
	if prevHash != "" {
		prevHashPtr = &prevHash
	}
	var beforePtr *string
	if maskedBefore != "" {
		beforePtr = &maskedBefore
	}

	// hash_version = 2 : empreinte calculée sur les valeurs réellement stockées.
	insertAudit := db.Rebind(`
		INSERT INTO audit_logs (id, user_id, action, table_name, record_id,
		                        before_state, after_state, ip_address,
		                        entry_hash, prev_hash, sequence_number, created_at, hash_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 2)`, usePostgres)
	if _, err := tx.ExecContext(ctx, insertAudit,
		db.NewID(), userID, action, tableName, recordID,
		beforePtr, maskedAfter, ipAddress,
		entryHash, prevHashPtr, lastSeq+1, now); err != nil {
		return "", time.Time{}, fmt.Errorf("insert audit log: %w", err)
	}

	return security.ChainHash(prevHash, entryHash), now, nil
}
