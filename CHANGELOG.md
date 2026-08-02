# Changelog

Toutes les modifications notables de LedgerAlps sont documentées ici.  
Format : [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/) — Versioning : [SemVer](https://semver.org/lang/fr/).

---

## [Unreleased]

### Corrigé
- **Activer HTTPS était impossible en pratique.** Deux défauts empilés : `config.json` n'est écrit qu'au premier lancement et jamais retouché — une installation mise à jour ne contient donc que les clés de sa version d'origine, sans `host` ni `force_tls` — et le lanceur transmettait `FORCE_TLS=false` explicitement, ce qui **écrasait le fichier** même édité à la main. Le lanceur ne transmet plus ce réglage : le serveur lit le même fichier, une seule source de vérité
- **Des textes s'affichaient sans couleur dans toute l'interface.** Les palettes `danger`, `warning` et `success` ne définissent que les nuances 100, 500 et 700 ; les classes en `-600`, `-50`, `-200` ou `-800` n'émettaient donc aucun CSS. Onze fichiers étaient concernés — messages d'erreur de formulaire, montants négatifs du plan comptable, avertissements TVA, boutons d'annulation
- **Les options `HOST`, `TLS_CERT`, `TLS_KEY`, `FORCE_TLS` et `ALLOW_INSECURE_HTTP` étaient inatteignables sur une installation Windows.** `config.json` primait sur les variables d'environnement, et l'assistant écrit toujours ce fichier : le serveur lisait donc des clés absentes et retombait sur les valeurs par défaut. Le lanceur transmettait même `FORCE_TLS`, ignoré. **Les variables d'environnement priment désormais sur le fichier** lorsqu'elles sont réellement définies — une variable vide ou absente n'écrase rien. C'est aussi la précédence qui avait fait viser la base live lors d'une restauration

### Ajouté
- **Paramètres → Maintenance → Réseau & chiffrement** — interface d'écoute, HTTPS local, certificat et clé, réglables sans éditer de JSON dans `%APPDATA%`. Les réglages sont écrits dans `config.json` **en préservant les clés inconnues** (sérialiser une structure supprimerait `jwt_secret` et déconnecterait tout le monde), via une écriture puis un renommage — une coupure laisserait sinon un fichier tronqué et l'application ne redémarrerait plus. Certificat et clé sont exigés ensemble et leurs chemins vérifiés avant enregistrement : une demi-configuration TLS empêcherait le démarrage. Un bouton « Redémarrer maintenant » applique le changement
- **Paramètres → Maintenance** — première tranche du point 6 de la roadmap. **Contrôle de cohérence** : équilibre débit/crédit, écritures et documents vides, empreintes d'intégrité manquantes, totaux ne correspondant pas aux lignes, contacts orphelins, factures créditées au-delà de leur montant, offres sans réponse depuis 90 jours. Il **ne répare rien** — une comptabilité incohérente se corrige par une écriture, pas par un bouton ; réparer en silence effacerait la trace (CO art. 957a al. 2 ch. 5). Chaque constat dit quoi faire. **État du système** : moteur, version, volumétrie, sauvegardes et combien sont chiffrées, exposition réseau, et les capacités déclarées du produit
- **Contrôle mécanique de cohérence des avis de conformité.** Un avis périmé coûte plus qu'un avis absent : l'utilisateur agit dessus et cesse de croire le suivant. Chaque avis déclare désormais les capacités qu'il suppose **absentes** du produit ; un test échoue dès qu'une de ces capacités existe, en nommant l'avis. Livrer une fonctionnalité en oubliant la bannière n'est plus possible — la construction s'arrête avant. Le contrôle a trouvé un second avis non protégé dès sa première exécution. Voir [`compliance/README.md`](compliance/README.md)


### Corrigé
- **L'avis de conformité nLPD art. 8 était périmé** : il annonçait encore « base et sauvegardes en clair » alors que les sauvegardes sont chiffrables depuis la v1.4.4. Réécrit pour dire ce qui est vrai — les sauvegardes peuvent l'être, la base ne le sera pas (SQLCipher est incompatible avec le binaire unique), et l'action qui vous revient est d'activer le chiffrement du disque (BitLocker, LUKS). Un avis faux est pire qu'un avis absent : il est cru

### Ajouté
- **`FORCE_TLS=true`** sert en HTTPS même sur `localhost`, pour les politiques de sécurité qui l'exigent. Ce n'est pas le défaut, et `docs/PRODUCTION.md` explique pourquoi : le trafic vers `127.0.0.1` ne touche aucune interface réseau, les navigateurs classent `http://localhost` parmi les origines dignes de confiance, et un avertissement de certificat répété entraîne le réflexe dont vit l'hameçonnage

### Sécurité
- **Le serveur n'écoute plus sur toutes les interfaces par défaut.** Il écoutait sur `0.0.0.0` : un portable sur un réseau public ou une salle d'attente servait sa comptabilité à ce réseau, en clair, sans que personne l'ait demandé. Le défaut est désormais `127.0.0.1` — joignable depuis cette machine seulement
- **HTTPS natif dès que LedgerAlps est joignable depuis le réseau.** Définir `HOST` sur autre chose que loopback active TLS : votre certificat via `TLS_CERT`/`TLS_KEY`, ou un **certificat auto-signé généré** dans `<données applicatives>/tls`. Il couvre `localhost`, le nom de la machine et ses adresses IP, vaut dix ans, et est **réutilisé d'un démarrage à l'autre** pour que l'exception accordée au navigateur tienne — la régénérer à chaque fois apprendrait à cliquer sans lire
- `ALLOW_INSECURE_HTTP=true` conserve le clair pour le seul cas légitime — un reverse proxy terminant TLS sur la même machine — et l'écrit au journal, parce que c'est aussi le drapeau vers lequel on se tourne pour faire taire un avertissement
- Sans cela, mot de passe de connexion, jeton de session et **phrase de passe de chiffrement des sauvegardes** traversaient le réseau en clair (LPD art. 8, OPDo art. 3 al. 1 let. c)

> **Changement de comportement.** Si vous accédiez à LedgerAlps depuis un autre poste sans reverse proxy, il faut désormais définir `HOST` explicitement — et l'accès passera en HTTPS. C'est délibéré : cette configuration servait des identifiants en clair.


---

## [1.4.4] — 2026-08-02

### Modifié
- **La phrase de passe de chiffrement est demandée après le clic sur « Créer une sauvegarde »**, dans un dialogue portant l'avertissement, et non plus dans un champ affiché en permanence. Sauvegarder sans chiffrer reste possible, mais devient un **choix explicite** au lieu d'être la conséquence d'un champ laissé vide
- **La restauration nomme la sauvegarde concernée** dans l'invite de phrase de passe : plusieurs copies peuvent avoir été chiffrées avec des phrases différentes

### Ajouté
- **Bouton « Redémarrer LedgerAlps maintenant »** sur le bandeau de restauration en attente. Le serveur s'arrête proprement — écoute HTTP, puis base — relance une copie de lui-même, et la restauration s'applique avant l'ouverture de la base. L'interface attend que `/health` réponde avant de recharger, au lieu d'afficher une erreur de connexion pendant le redémarrage. Le lanceur Windows ne supervise pas le serveur : personne d'autre ne le relancerait. Refusé s'il n'y a aucune restauration en attente
- **Sauvegardes depuis l'interface** — onglet **Sauvegardes** dans Paramètres : créer un instantané, saisir la phrase de passe de chiffrement, consulter les copies existantes et leur état (chiffrée / en clair). Réservé aux administrateurs
- **Restauration depuis l'interface, avec avertissement** — un dialogue explique que la comptabilité actuelle sera remplacée et que **LedgerAlps devra être redémarré**, puis la restauration est *préparée* : l'instantané est déchiffré et vérifié immédiatement, et appliqué au démarrage suivant avant l'ouverture de la base. Un serveur ne peut pas échanger sous lui le fichier qu'il a ouvert ; l'interface le dit au lieu de laisser croire que le clic a suffi. Une phrase de passe erronée est refusée sur-le-champ, pas au redémarrage. La restauration préparée reste annulable, et la comptabilité remplacée est sauvegardée juste avant
- **Sauvegardes chiffrées** — `BACKUP_PASSPHRASE`, ou `--passphrase` sur `ledgeralps-cli backup` et `restore`. Argon2id dérive la clé, XChaCha20-Poly1305 chiffre le contenu, le tout en Go pur. Les sauvegardes sont la copie qui *quitte* la machine (NAS, clé USB) et donc la plus exposée : un instantané égaré expose l'intégralité de la comptabilité et des données clients (nLPD art. 8, OPDo art. 1-6). **La copie en clair n'est effacée qu'après avoir déchiffré l'instantané et contrôlé son intégrité SQLite** — une sauvegarde irrécupérable n'est pas une sauvegarde. Le chiffrement est authentifié : une altération, une troncature ou un réordonnancement des blocs sont refusés, jamais silencieusement acceptés
- **Filtrer les documents par client ou fournisseur** — sélecteur sur les pages Factures et Offres de prix, et liste des factures, offres et notes de crédit sur la fiche du contact. Le filtre est appliqué en SQL : filtrer la page affichée n'aurait jamais trouvé les pièces des pages suivantes
- **Note de crédit rattachée à la facture qu'elle corrige** — `POST /invoices/:id/credit-note`, et un bouton « Note de crédit » sur une facture envoyée ou payée. La note référence la facture (`corrects_invoice_id`) et son PDF porte la mention « Annule la facture : FA-… » : LTVA art. 27 al. 4 définit la correction comme « un document qui mentionne et annule la facture d'origine », et le CO art. 957a al. 2 ch. 5 exige la traçabilité. Jusqu'ici une note de crédit ne référençait rien — un contrôleur constatant une TVA réduite n'avait aucun moyen de remonter à la facture concernée. La facture d'origine n'est pas modifiée
- **Le montant d'une note de crédit est borné** par la facture : la somme des notes rattachées ne peut plus dépasser son total (`409`). Les notes annulées libèrent leur part, puisqu'elles ne créditent rien. Un crédit partiel est possible en fournissant des lignes ; les crédits partiels s'additionnent contre le même plafond, au lieu d'être jaugés chacun contre la facture entière

### Corrigé
- **La copie de sécurité prise avant une restauration était en clair**, même quand toutes vos sauvegardes étaient chiffrées : le processus défaisait votre choix en silence. Elle est désormais prise **à la préparation** — le seul moment où vous êtes présent avec votre phrase de passe — et **chiffrée avec celle-ci**. Elle apparaît dans la liste comme un instantané ordinaire et suit la même rotation, au lieu de s'accumuler sous forme de fichiers `pre-restore` que rien ne nettoyait. Son utilité reste entière : on découvre qu'on a restauré la mauvaise sauvegarde après avoir regardé les données, pas pendant
- **Les sauvegardes chiffrées étaient invisibles.** Le listing exigeait le suffixe `.db`, or un instantané chiffré finit en `.db.enc` : il existait sur le disque et n'apparaissait nulle part. Ce n'était pas qu'un défaut d'affichage — le listing sert aussi à **résoudre le nom d'une restauration** (aucun instantané chiffré n'était donc restaurable depuis l'interface), à **nettoyer** les anciennes copies (elles s'accumulaient indéfiniment) et à décider au démarrage qu'une sauvegarde est due
- **Deux sauvegardes chiffrées dans la même seconde échouaient.** Le compteur anti-collision testait l'existence du `.db` alors que le fichier final est `.db.enc` : la seconde reprenait le même nom et butait sur un chiffré déjà présent, au lieu de passer au numéro suivant
- **Titre tronqué sur la page d'accueil de l'installeur** : « Bienvenue dans le programme d'installation de LedgerAlps 1.4.4 » ne tenait pas sur deux lignes, et le débordement était coupé net plutôt que replié — le numéro de version disparaissait
- **La liste des factures affichait un identifiant tronqué** (`a4571078…`) à la place du nom du client. On ne peut pas chercher les factures d'un client dont le nom n'apparaît jamais
- **Le bouton « Note de crédit » restait actif sur une facture déjà créditée en totalité**, alors que le serveur refuse (409). Les factures exposent désormais `credited_amount` et le bouton se grise, avec un bandeau l'expliquant

### Limitations connues
- **La base de données elle-même n'est pas chiffrée** — seules les sauvegardes le sont. Chiffrer la base exigerait SQLCipher, une bibliothèque C : le projet compile avec `CGO_ENABLED=0` et un pilote SQLite en Go pur, ce qui donne la compilation croisée et le binaire unique sans dépendance. Adopter SQLCipher y mettrait fin. Sur un poste mobile, chiffrer le disque (BitLocker, LUKS) reste la mesure à prendre
- **Une note de crédit ne passe pas d'écriture au journal** — parce qu'aucune facture n'en passe. Les ventes se saisissent manuellement au journal ; seuls les paiements sont automatisés. Contrepasser automatiquement un produit jamais enregistré créerait un produit négatif sans contrepartie, et doublerait la correction si l'utilisateur l'a déjà passée. L'automatisation doit commencer par les factures, pas par leurs corrections

---

## [1.4.3] — 2026-08-01

### Ajouté
- **Conversion d'une offre en facture** — `POST /invoices/:id/convert`, et un bouton « Convertir en facture » sur le détail d'une offre. **L'offre est conservée, pas transformée** : le client en détient une copie, et remplacer l'enregistrement le laisserait citer une référence disparue de votre système — le lien que le CO art. 958f al. 3 demande de garantir. La facture porte son propre numéro `FA-`, reprend les lignes à l'identique et référence l'offre par `converted_from_id` ; l'offre est marquée « acceptée ». Une seconde conversion est refusée (`409`) : c'est la garde contre une double facturation
- **Issue commerciale d'une offre** — `POST /invoices/:id/outcome` enregistre `refused` ou `expired`. `accepted` n'y est pas acceptée : une offre s'accepte en produisant la facture, jamais en basculant un champ, faute de quoi une offre pourrait se lire « acceptée » sans facture derrière

### Corrigé
- **Une offre de prix était comptée dans la déclaration TVA.** La table `invoices` héberge aussi les offres et les notes de crédit, et la déclaration ne filtrait pas `document_type` : une offre passée au statut « envoyée » — le geste naturel quand on l'adresse à un prospect — entrait au chiffre 200 avec sa TVA. L'entreprise déclarait et payait de la TVA sur un chiffre d'affaires jamais réalisé. Sous LTVA art. 40 al. 1 let. a, la dette d'impôt naît « au moment de la facturation » ; une offre n'est pas une facture
- **Une offre de prix produisait un PDF intitulé « FACTURE » avec bulletin QR.** Le générateur ignorait `document_type` : le prospect recevait un document payable, portant le taux et le montant de TVA ainsi que le numéro IDE, indiscernable d'une facture. Il pouvait le payer et en déduire l'impôt préalable — or LTVA art. 27 al. 2 rend redevable celui qui fait figurer l'impôt sans en avoir le droit. Le titre suit désormais le type de document et le bulletin QR n'est imprimé que sur une facture
- **Une note de crédit augmentait la TVA due** au lieu de la réduire, les montants étant stockés sans signe. Elle la réduit désormais correctement (LTVA art. 41) : le signe est appliqué à l'agrégation, les montants restant stockés sans signe pour que le document reste lisible à l'écran comme sur papier
- **Le bouton « Marquer en retard » échouait systématiquement**, et ce n'était que la partie visible : le statut `overdue` n'a jamais existé côté serveur — ni dans l'enum, ni dans les transitions, ni dans la contrainte `CHECK` de la base. Le compteur « en retard » du tableau de bord valait donc toujours zéro et le filtre « En retard » de la liste ne renvoyait rien, sans qu'aucun message ne le signale. « En retard » est désormais **déduit de la date d'échéance** — une facture envoyée dont l'échéance est passée — comme le fait déjà la balance âgée. Le bouton disparaît : une facture ne devient pas en retard parce qu'on l'a décidé
- **Les offres étaient comptées comme créances client** dans la balance âgée et sur le tableau de bord — un devis envoyé à un prospect n'est dû par personne
- **Page de licence de l'installeur** : « Copyright (c) 2024–2026 » s'affichait « 2024â€"2026 ». Le fichier `LICENSE` contenait un tiret demi-cadratin en UTF-8, or NSIS ne lit un fichier de licence en UTF-8 que s'il porte un BOM — sans quoi il retombe sur la codepage ANSI. Le caractère est remplacé par un tiret ASCII plutôt que d'ajouter un BOM, qui perturberait les détecteurs de licence (GitHub licensee, scanners SPDX) et partirait aussi dans les paquets `.deb` et `.rpm`

### Modifié
- **Une offre de prix ne peut plus être marquée « payée »** — personne ne doit rien sur une offre. Sa machine à états est désormais distincte de celle des factures : `brouillon → envoyée → annulée | archivée`. C'est ce chemin qui plaçait les offres dans les créances et dans la déclaration TVA
- **PDF d'une offre** : « Échéance » devient « Valable jusqu'au ». Rien n'est dû sur une offre, et le mot « échéance » invite à la traiter comme payable
- Le contrôle d'encodage de l'installeur vérifie désormais **`LICENSE` en plus de `installer.nsi`**, et s'exécute **avant** la compilation. Il ne vivait que dans le workflow de répétition : le vrai workflow de publication n'en avait aucun

### Documentation
- **Ce que contient une sauvegarde**, et **comment le chiffrement fonctionne** : README pour l'essentiel, `docs/PRODUCTION.md` pour le détail — Argon2id, XChaCha20-Poly1305, et le fondement légal (LPD art. 8, OPDo art. 1 et 3). Précision assumée : **aucun de ces textes ne nomme d'algorithme** ; la loi impose un résultat proportionné au risque, ces primitives sont notre réponse à cette exigence, pas une obligation que nous exécuterions
- `docs/API.md` : section « Types de documents » et documentation de la conversion, avec les codes de retour et le fondement légal

---

## [1.4.2] — 2026-08-01

### Modifié
- **Plateformes publiées réduites à Windows x86-64 et Linux x86-64.** macOS et toutes les variantes ARM sont abandonnées : ces binaires étaient produits parce que la compilation croisée Go ne coûte rien, pas parce qu'ils étaient testés — il n'existe aucune machine macOS ni ARM dans la CI. Sur un PC Windows ARM, l'installeur x86-64 fonctionne par émulation ; pour macOS ou ARM, le projet reste compilable depuis les sources
- `scripts/install.sh` refuse macOS et ARM en indiquant la raison, au lieu de tenter un téléchargement qui répondrait 404

### Corrigé
- **L'archive `windows_arm64` ne contenait que `ledgeralps-cli.exe`** — ni serveur ni lanceur. Quiconque téléchargeait le fichier portant le nom de son architecture obtenait un outil en ligne de commande sans rien à piloter. Présent depuis la v1.3.15 au moins, passé inaperçu faute de test. Résolu par la suppression de la cible
- **Le dossier d'installation survivait à la désinstallation.** Les installations antérieures à la v1.1.1 livraient l'interface en fichiers séparés (`index.html`, `assets\`) ; les mises à jour ne les ont jamais retirés, et le `RMDir` non récursif du désinstalleur — volontairement prudent, pour ne jamais effacer un fichier déposé par l'utilisateur — échouait donc en silence. Ces reliquats sont désormais nettoyés à l'installation comme à la désinstallation

### Documentation
- Nouvelle section « Plateformes prises en charge » dans la roadmap, avec le raisonnement de l'abandon
- Rattrapage des entrées 1.4.0 et 1.4.1, absentes de ce fichier alors qu'il est livré dans chaque archive

---

## [1.4.1] — 2026-08-01

### Corrigé
- **Recherche IDE de l'assistant de premier démarrage** : l'endpoint interrogé répondait 403 à tout le monde, et non à cause d'une saisie erronée. La recherche passe désormais par la route publique du registre. Le code existait en double — serveur et lanceur — si bien que corriger un seul côté laissait l'assistant cassé ; implémentation unifiée dans `internal/core/zefix`
- Le code postal était perdu en silence (champ JSON mal nommé)
- Les données société saisies dans l'assistant sont bien enregistrées et visibles dans Paramètres ; en cas d'échec, l'assistant le signale au lieu d'annoncer un succès
- Une panne du registre n'affiche plus de code HTTP : le message invite à saisir les informations manuellement

### Sécurité
- Jeton de rafraîchissement déplacé dans un cookie **HttpOnly + SameSite=Strict**, inaccessible au JavaScript de la page
- La déconnexion révoque le jeton côté serveur
- Suppression de tout appel à Google Fonts : ouvrir l'application ne signale plus son usage à un tiers
- bcrypt sorti du budget de la transaction base de données (Bootstrap, Login)

### Modifié
- Bannières de conformité repliables (~40 px au lieu de ~180)
- Messages de validation et libellés d'accessibilité

---

## [1.4.0] — 2026-07-28

### Ajouté
- **Veille de conformité automatisée** — surveillance hebdomadaire de Fedlex (SPARQL) pour nLPD, OPDo, LTVA et CO, de SIX pour les Implementation Guidelines QR-facture, et d'EUR-Lex pour le RGPD. Une évolution ouvre une issue ; l'avis est **rédigé par un humain avec citation de la source**, jamais généré automatiquement
- **Avis de conformité dans l'application**, servis depuis un flux embarqué dans le binaire — fonctionne hors ligne
- **Vérification de mise à jour** — seul appel réseau sortant du produit, sans identifiant ni télémétrie, résultat mis en cache 24 h, échec silencieux, désactivable par `UPDATE_CHECK=false`

### Documentation
- Suppression du code Python/FastAPI, de `docker-compose.yml` et du script Inno Setup orphelin
- Réécriture d'`ARCHITECTURE.md` et `PRODUCTION.md` ; README recentré sur l'utilisateur

---

## [1.3.16-rc1] — 2026-07-27

> Pré-version publiée depuis la branche `test` pour validation.

### Ajouté
- **Veille de conformité automatisée** — surveillance hebdomadaire des sources faisant autorité : Fedlex (SPARQL) pour nLPD RS 235.1, OPDo 235.11, LTVA 641.20 et CO 220 ; SIX pour les Implementation Guidelines QR-facture ; EUR-Lex pour le RGPD. Une évolution ouvre une issue ; l'avis est ensuite **rédigé par un humain avec citation de la source** — jamais généré automatiquement
- **Avis de conformité dans l'application** — bannière affichant les évolutions qui concernent l'utilisateur, servie depuis un flux embarqué dans le binaire (fonctionne hors ligne). `GET /api/v1/compliance/advisories`
- **Répétition de release sans publication** — workflow « Release (dry run) » produisant tous les artefacts, y compris l'installeur NSIS, sans créer de release ni de tag

### Modifié
- La CI et le lint s'exécutent désormais sur la branche `test` avec les mêmes contrôles que `main`
- `release.yml` refuse un tag **final** dont le commit n'est pas atteignable depuis `main` ; les pré-versions en sont exemptées

### Documentation
- Suppression du code Python/FastAPI (`backend/`), de `docker-compose.yml` et du script Inno Setup orphelin : remplacés par la réécriture Go depuis la v1.0.0, ni construits ni livrés
- Réécriture de `ARCHITECTURE.md` et `PRODUCTION.md`, qui décrivaient encore la pile Python abandonnée
- README recentré sur l'utilisateur ; contenu technique déplacé vers `docs/`
- Nouveaux documents : `docs/DEVELOPMENT.md`, `docs/API.md`, `docs/BRANCHING.md`

---

## [1.3.15] — 2026-07-27

### Ajouté
- **Factures fournisseurs** — l'impôt préalable alimente enfin le chiffre 400 de la déclaration TVA. Il était figé à zéro : la TVA due était systématiquement surévaluée. Lignes multi-taux, garde anti-doublon `UNIQUE(fournisseur, référence)`
- Journalisation des verrouillages de connexion (`security_events`, endpoint réservé aux administrateurs)

### Corrigé
- **Installeur Windows** — le désinstalleur affichait « donnÃ©es ». Le script était en UTF-8 mais sans BOM, et NSIS retombait alors sur la codepage ANSI du système. Les messages sont désormais localisés EN/FR
- **Tableau de bord** — la carte « année fiscale » interrogeait les colonnes `label`/`status`, inexistantes : la requête échouait à chaque appel et la carte restait vide

---

## [1.3.14] — 2026-07-27

### Corrigé
- **Conformité QR-facture (IG v2.4)** — l'appariement référence/compte n'était pas vérifié, première cause de rejet par les banques. QRR exige désormais un QR-IBAN, SCOR et NON sont refusés sur un QR-IBAN, la référence QRR est restreinte au CHF (nouveauté v2.4), les références SCOR sont validées selon ISO 11649
- Un QR-IBAN ne peut plus retomber silencieusement sur une référence NON lors de la génération du PDF

### Ajouté
- **Sauvegarde et restauration** — instantanés `VACUUM INTO` cohérents sans interruption de service, vérification d'intégrité, instantané automatique au démarrage, commandes CLI `backup` / `backups` / `restore`
- **Limitation des tentatives de connexion** — verrouillage par IP après 5 échecs en 15 minutes

---

## [1.2.0] – [1.3.13] — avril 2026

Pipeline de release (GoReleaser + NSIS), CLI d'administration, endpoints
rapports / paiements / journal d'audit, logo d'entreprise, édition des factures
et devis, auto-remplissage IDE/ZEFIX, et une longue série de corrections de la
QR-facture (encodage Latin-1, layout du bulletin, suppression de Swico S1,
validation IBAN, passage à l'adresse structurée de type S).

Le détail commit par commit est disponible sur la page
[Releases](https://github.com/kmdn-ch/LedgerAlps/releases), générée
automatiquement à chaque publication.

---

## [1.1.1] — 2026-04-09

### Ajouté
- **Lanceur Windows** (`cmd/launcher` → `ledgeralps.exe`, `-H=windowsgui`) — assistant de configuration au premier démarrage : génère le JWT_SECRET via `crypto/rand`, collecte email/nom/mot de passe admin dans le navigateur, écrit `%APPDATA%\LedgerAlps\config.json`, démarre le serveur, bootstrap l'admin, ouvre l'application
- **Config JSON** — `internal/config` lit `%APPDATA%\LedgerAlps\config.json` (Windows) ou `~/.ledgeralps/config.json` en priorité sur les variables d'environnement
- **Frontend statique embarqué** — `ledgeralps-server.exe` sert `dist/` depuis le répertoire d'installation avec fallback SPA (`NoRoute → index.html`)
- **Goreleaser** — build `ledgeralps-launcher` (Windows amd64, `-H=windowsgui`) ajouté au pipeline de release
- **NSIS installer** — réécriture complète : installe le lanceur + frontend `dist\`, supprime l'enregistrement de service Windows, raccourcis pointent sur `ledgeralps.exe`

### Modifié
- `Makefile` — cibles `build-launcher`, `build-windows`, `build-frontend`, `build-installer`, `release`
- `README.md` — section installation Windows, assistant premier démarrage, liste complète des 35 endpoints

### Corrigé
- CI : `noctx` — `http.Get` / `http.Post` remplacés par `http.NewRequestWithContext` + `http.DefaultClient.Do` dans le lanceur
- CI : `build-check` — cross-compilation `cmd/launcher` ajoutée pour détecter les régressions à chaque push

---

## [1.0.0] — 2026-04-08

### Réécriture complète — Backend Go (branche go-rewrite, Sprints 1–7)

#### Ajouté
- **Backend Go** (`gin-gonic/gin`) remplace FastAPI — binaire unique, zéro-config
- **SQLite WAL** embarqué (`modernc.org/sqlite`) + **PostgreSQL** (`pgx/v5`) en production
- **Migrations embed.FS** auto au démarrage — aucun outil externe requis
- **Plan comptable PME suisse** — 88 comptes (CO art. 957) seedés en migration
- **JWT refresh tokens** — `POST /auth/refresh`, `POST /auth/logout` (révocation jti), `POST /auth/register`, `POST /auth/bootstrap` (premier admin one-shot)
- **Hash chain SHA-256** (CO art. 957a) sur toutes les écritures postées — immuabilité garantie
- **PDF factures A4** avec QR payment slip Swiss intégré (`fpdf` + `go-qrcode`)
- **QR-facture SPC 0200** — référence QRR MOD-10 récursif, FormatQRRReference, Swico S1
- **ISO 20022 pain.001.001.09** — export virements (`POST /payments/export`)
- **ISO 20022 camt.053.001.08** — import relevés bancaires (`POST /bank-statements/import`)
- **Clôture exercice fiscal** — FiscalYearService.CloseYear() (CO art. 958)
- **Déclaration TVA** — méthode effective + TDFN (AFC 318/100), taux 2024
- **Export ZIP légal** — `GET /exports/legal-archive` (CO art. 958f, 10 ans) + manifest SHA-256, IBAN masqué nLPD
- **Dashboard stats** — `GET /stats` (créances, journal, comptes actifs, contacts, exercice ouvert)
- **26 endpoints** API v1 documentés
- **44 tests** : 34 unitaires (compliance, security, db) + 10 intégration end-to-end (httptest + SQLite temp)
- **Frontend aligné** — json tags snake_case, intercepteur 401 + refresh queue, `vite build` propre

#### Modifié
- `internal/models/models.go` — json tags snake_case sur tous les champs (breaking change API)
- `frontend/src/api/client.ts` — réécriture complète : silent refresh, endpoints Go
- `frontend/src/types/index.ts` — types alignés backend Go (currency, total_amount, invoice_number)

#### Supprimé
- Backend Python/FastAPI (remplacé par Go)
- Dépendances Alembic, SQLAlchemy, Pydantic

#### Conformité
- CO art. 957–963 : partie double, immuabilité, conservation 10 ans
- nLPD : IBAN masqué dans export légal, données minimales
- TVA CH 2024 : 8.1% / 2.6% / 3.8%, arrondi 0.05 CHF
- QR-facture SPC 0200 (Six-Group)
- ISO 20022 pain.001 / camt.053

---

## [0.1.0] — 2026-04-07

### Ajouté
- **Backend FastAPI** avec SQLAlchemy async et PostgreSQL 16
- **Modèles** : `Account`, `JournalEntry` / `JournalLine`, `Invoice` / `InvoiceLine`, `Contact`, `AuditLog`, `FiscalYear`, `User`
- **Migration Alembic initiale** (`0001_initial`) — toutes les tables et enums PostgreSQL
- **API REST complète** : auth JWT, comptes, journal, factures, contacts, TVA, QR-facture, ISO 20022, exports
- **`GET /api/v1/journal`** — pagination (`page`, `page_size`) + filtres (`date_from`, `date_to`, `status`, `reference`)
- **`GET /api/v1/contacts/{id}`** et **`PATCH /api/v1/contacts/{id}`** — mise à jour partielle
- **Moteur comptable** : partie double, contrepassation, hash SHA-256 chaîné (CO art. 957a)
- **Service de facturation** : cycle draft → sent → paid → archived, écritures auto au journal
- **Calcul TVA** suisse : taux 8.1% / 2.6% / 3.8%, arrondi 0.05 CHF, méthode effective et TDFN
- **QR-facture** : génération payload SPC 0200, référence QRR/RF (Six-Group / STUZZA)
- **ISO 20022** : export pain.001.001.09 (virements), import camt.053.001.08 (relevés)
- **Middleware** : rate limiting, security headers, audit log
- **Frontend React/TypeScript/Tailwind** : Dashboard, Factures, Journal, Contacts, Comptes, Rapports, Paramètres
- **`InvoiceDetailPage`** : détail facture, transitions de statut, aperçu PDF inline
- **Composant `PDFPreview`** : affichage inline avec `<iframe>` + objectURL
- **Tests unitaires** : TVA, arrondi 5 rappen
- **Tests d'intégration** : auth, contacts, TVA, factures (cycle complet), journal (pagination + filtres), PATCH contacts
- **Docker Compose** : PostgreSQL + backend + frontend + Nginx (profil production)
- **`.env.example`** avec toutes les variables documentées
- **README** complet : installation, configuration, commandes `make`, conformité légale

### Conformité légale
- CO art. 957–963 : comptabilité en partie double, immuabilité des écritures postées
- nLPD : local-first, données minimales, Privacy by Design
- TVA CH 2024 : taux 8.1% / 2.6% / 3.8%
- QR-facture Six-Group SPC 0200
- ISO 20022 pain.001 / camt.053–054

---

[Unreleased]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.4...HEAD
[1.4.4]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.3...v1.4.4
[1.4.3]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.2...v1.4.3
[1.4.2]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.1...v1.4.2
[1.4.1]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.3.15...v1.4.0
[1.0.0]: https://github.com/kmdn-ch/LedgerAlps/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/kmdn-ch/LedgerAlps/releases/tag/v0.1.0
