# Branches et promotion

## Les branches

| Branche | Rôle | CI | Release |
|---|---|---|---|
| `main` | État livrable. Tout commit doit être releasable. | CI + Lint | Tag `v*` → GoReleaser |
| `test` | Branche d'intégration : on y valide avant de promouvoir. | CI + Lint (identiques à `main`) | jamais |
| `feat/*`, `fix/*` | Travail en cours. | CI + Lint via pull request | jamais |

`test` reçoit **exactement les mêmes contrôles** que `main`. Une branche de
validation avec une CI plus permissive ne valide rien.

## Le flux

```
feat/ma-fonctionnalite
        │  pull request (CI + Lint)
        ▼
      test  ──────── validation : CI verte + vérification manuelle
        │
        │  merge --ff-only  (aucun commit de merge : l'historique reste linéaire
        │                    et le commit validé sur test est bit-à-bit celui
        │                    qui arrive sur main)
        ▼
      main
        │  git tag -a vX.Y.Z
        ▼
    release (GoReleaser : binaires + installeur NSIS)
```

### Promouvoir `test` vers `main`

```bash
git fetch origin
git checkout test && git pull --ff-only
# la CI de test doit être verte AVANT de promouvoir
gh run list --branch test --limit 3

git checkout main && git pull --ff-only
git merge --ff-only test
git push origin main
```

Le `--ff-only` est délibéré : si la fusion ne peut pas être en avance rapide,
c'est que `main` a divergé et il faut rebaser `test` dessus plutôt que de
fabriquer un commit de merge qui n'a jamais été testé tel quel.

### Remettre `test` à niveau après une release

```bash
git checkout test
git merge --ff-only main
git push origin test
```

## Politique de version

Rappel de [`ROADMAP.md`](../ROADMAP.md) :

- `vX.Y.0` — livraison d'un milestone fonctionnel complet ;
- `vX.Y.Z` (Z > 0) — correctifs groupés dans le cycle du milestone ;
- **jamais un tag par commit isolé** : les correctifs sont regroupés puis
  taggés une fois stables.

## Cas particulier : la veille de conformité

Un changement détecté par [`compliance/`](../compliance/README.md) suit le même
chemin. Une évolution légale n'autorise pas à court-circuiter `test` : un avis
erroné ou une régression de génération QR livrés en urgence font plus de dégâts
qu'un jour d'attente.

Seule exception envisageable : une non-conformité qui empêche déjà les
utilisateurs d'être payés (QR-facture rejetée par les banques). Même alors, la
CI doit être verte — c'est le délai de validation manuelle qui est raccourci,
pas les tests.
