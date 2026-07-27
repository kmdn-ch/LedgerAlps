# Veille de conformité

Ce dossier fait tenir LedgerAlps à jour vis-à-vis du droit suisse et des
standards de paiement, et permet de prévenir l'utilisateur **dans le logiciel**
quand une évolution le concerne.

---

## Règle fondamentale

> **La surveillance détecte un *changement*. Un humain écrit l'*avis*.**

Le script de veille ne dit jamais ce qu'une modification signifie. Il constate
qu'une source a bougé et ouvre une issue. C'est un mainteneur qui lit la source,
détermine l'impact et rédige l'avis, en citant la référence.

Cette séparation n'est pas de la prudence excessive : générer automatiquement
« la loi exige désormais X » placerait un texte juridique inventé sous les yeux
de comptables qui prennent des décisions fiscales. Un avis faux est pire
qu'aucun avis. **Tout avis sans `source_url` vérifiable est rejeté au parsing**
(`ParseFeed`), et le test `TestBundledFeedIsValid` casse la compilation si un
avis livré n'a pas de source.

---

## Sources surveillées

Enregistrées dans [`sources.json`](sources.json), classées par fiabilité.

| Fiabilité | Source | Signal | Pourquoi |
|---|---|---|---|
| **primaire** | Fedlex — nLPD (RS 235.1), OPDo (235.11), LTVA (641.20), CO (220) | SPARQL `dateApplicability` | Plateforme officielle de publication de la Confédération : le texte consolidé qu'elle sert **est** la version qui fait foi. Endpoint public, sans authentification, donnée structurée. |
| **primaire** | SIX — Implementation Guidelines QR-facture | SHA-256 du PDF | Spécification normative des Swiss Payment Standards. Fichier statique : le hachage est stable (vérifié). |
| **primaire** | EUR-Lex — RGPD 2016/679 | Identifiants de versions consolidées `02016R0679-AAAAMMJJ` | Voir ci-dessous. |
| secondaire | SIX — page d'accueil QR-facture | Numéro de version le plus élevé dans les liens `ig-qr-bill-v*.pdf` | Détecte l'apparition d'une nouvelle version avant même de la télécharger. |

### Pourquoi pas un simple hachage de page

Le hachage du HTML rendu a été essayé pour le RGPD **et rejeté** : deux
récupérations de la *même* URL à quelques minutes d'intervalle produisent des
empreintes différentes (contenu dynamique). La veille aurait annoncé une
modification du RGPD à chaque exécution.

Une source qui crie au loup est pire que pas de source : on apprend à ignorer
l'alerte, et on rate celle qui compte. D'où le choix d'un signal *sémantique*
(l'identifiant de version consolidée) plutôt que d'un hachage d'affichage.

Le PDF de SIX, lui, est un fichier statique : la stabilité du hachage a été
vérifiée avant de l'adopter.

### Échec ≠ changement

`compliance_watch.py` distingue trois issues :

| Code | Signification | Effet en CI |
|---|---|---|
| `0` | aucun changement | rien |
| `1` | une source a changé | ouvre/commente une issue `compliance` |
| `2` | une source n'a **pas pu** être vérifiée | avertissement, build non rouge |

Une panne DNS ne doit jamais ressembler à une révision de la loi sur la
protection des données.

---

## Utilisation

```bash
python scripts/compliance_watch.py            # vérifier
python scripts/compliance_watch.py --json     # sortie machine
python scripts/compliance_watch.py --update   # enregistrer les empreintes actuelles
```

Le script n'utilise que la bibliothèque standard : une veille de conformité qui
tombe en panne parce qu'un paquet tiers a publié une mauvaise version ne vaut
rien. Exécution automatique : tous les lundis (`.github/workflows/compliance-watch.yml`).

---

## Diffusion des avis vers un logiciel déjà installé

Un binaire livré aujourd'hui ne peut pas savoir ce que dira la loi dans deux
ans. Deux mécanismes, par ordre de confiance :

**1. Flux embarqué (défaut, souverain)**
`internal/core/compliance/advisories.json` est compilé dans le binaire via
`go:embed`. Aucun réseau requis, LedgerAlps fonctionne entièrement hors ligne.
Reflète l'état des connaissances à la date de la version.

**2. Rafraîchissement signé (optionnel)**
Le binaire peut récupérer un flux mis à jour et le fusionner, pour qu'un avis
rédigé *après* la version installée atteigne quand même l'utilisateur.

Le flux distant est signé en Ed25519 et vérifié contre une clé publique
compilée dans le binaire (`VerifySignedFeed`). Sans cette vérification,
quiconque intercepte la connexion pourrait injecter de fausses instructions
juridiques dans un logiciel de comptabilité : **un flux non signé serait pire
que pas de flux du tout.**

La requête est un simple `GET` d'un document statique — aucun identifiant,
aucune télémétrie, aucune donnée utilisateur — et tout échec laisse le flux
embarqué en place.

> **Limite d'amorçage.** Les versions antérieures à celle qui introduit ce
> mécanisme ne peuvent recevoir aucun avis : elles n'ont pas le code pour les
> lire. La première version capable d'être prévenue est celle qui embarque ce
> système. Pour les versions plus anciennes, le canal reste la page Releases.

---

## Ajouter un avis

1. Lire la source. Déterminer si LedgerAlps est réellement affecté.
2. Si le code doit changer : le corriger **et** ajouter un test de régression.
3. Ajouter une entrée dans `internal/core/compliance/advisories.json` :

```jsonc
{
  "id": "identifiant-stable-et-unique",
  "domain": "qr_bill",              // qr_bill | privacy | vat | accounting | security
  "severity": "action_required",    // info | action_required | critical
  "title": { "fr": "…", "en": "…" },
  "body":  { "fr": "…", "en": "…" },
  "source_name": "SIX — IG QR-bill v2.4, §4.2.2",
  "source_url": "https://…",        // OBLIGATOIRE, vérifiable
  "published_at": "2026-02-24",
  "effective_from": "2026-02-24",   // null si pas d'échéance
  "resolved_in_version": "1.3.14"   // null tant que le produit ne le gère pas
}
```

4. `python scripts/compliance_watch.py --update` pour enregistrer l'empreinte.
5. Ouvrir une PR vers `test`, valider, puis promouvoir vers `main`.

### Ce qu'un avis ne doit pas faire

- **Inventer une échéance.** Si la source ne donne pas de date, `effective_from`
  reste `null`. Écrire « dès août 2027 » sans texte à l'appui est une faute.
- **Confondre obligation de moyens et obligation datée.** L'art. 8 LPD impose
  des mesures « appropriées » : c'est une obligation de moyens, pas une
  échéance. L'avis `nlpd-art8-data-security` le dit explicitement.
- **Donner un conseil juridique.** On décrit l'exigence et on cite la source ;
  l'utilisateur reste responsable de sa conformité et peut consulter sa
  fiduciaire.

---

## Affichage

`GET /api/v1/compliance/advisories?lang=fr` renvoie les avis pertinents pour la
version en cours. Sont masqués :

- ceux déjà couverts par la version installée (`resolved_in_version`) ;
- ceux dont l'entrée en vigueur dépasse l'horizon (6 mois) — annoncer une
  obligation à deux ans à chaque démarrage apprend à ignorer la bannière.

La bannière (`ComplianceBanner.tsx`) est montée dans le layout. Un avis masqué
par l'utilisateur réapparaît si sa date de publication change : un
« ne plus afficher » ne doit pas enterrer une obligation légale révisée.
