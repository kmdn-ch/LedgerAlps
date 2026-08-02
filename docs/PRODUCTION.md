# Déploiement serveur

> **La plupart des utilisateurs n'ont pas besoin de ce document.** Sur Windows,
> l'installeur fait tout : voir le [README](../README.md). Cette page s'adresse
> à qui veut faire tourner LedgerAlps sur un serveur Linux x86-64, par exemple
> pour y accéder depuis plusieurs postes du bureau.

LedgerAlps reste **local-first** : même déployé sur un serveur, il est conçu
pour votre réseau, pas pour être exposé sur Internet.

---

## 1. Installer le binaire

Téléchargez l'archive correspondant à votre système depuis la page
[Releases](https://github.com/kmdn-ch/LedgerAlps/releases/latest), ou utilisez
le paquet `.deb` / `.rpm`.

```bash
tar xzf ledgeralps_*_linux_amd64.tar.gz
sudo install -m 0755 ledgeralps-server ledgeralps-cli /usr/local/bin/
```

## 2. Générer un secret

Le serveur **refuse de démarrer** si `JWT_SECRET` fait moins de 32 caractères
ou figure parmi les valeurs faibles connues. C'est délibéré : un secret faible
permettrait de forger des jetons d'authentification.

```bash
export JWT_SECRET=$(openssl rand -hex 32)
```

## 3. Choisir la base de données

**SQLite (recommandé)** — aucun serveur à administrer, sauvegarde = un fichier.

```bash
export SQLITE_PATH=/var/lib/ledgeralps/ledgeralps.db
```

**PostgreSQL** — si vous en avez déjà un. Définir `POSTGRES_DSN` bascule
automatiquement le moteur.

```bash
export POSTGRES_DSN="postgres://ledgeralps:motdepasse@localhost:5432/ledgeralps"
```

> Les migrations sont embarquées dans le binaire et s'appliquent au démarrage.
> Rien à lancer manuellement.

## 4. Créer le premier administrateur

```bash
ledgeralps-server &                       # démarre sur le port 8000
ledgeralps-cli bootstrap --email=admin@entreprise.ch --password='…'
```

## 5. Service systemd

```ini
# /etc/systemd/system/ledgeralps.service
[Unit]
Description=LedgerAlps
After=network.target

[Service]
Type=simple
User=ledgeralps
ExecStart=/usr/local/bin/ledgeralps-server
Restart=on-failure

# Le secret vit dans un fichier lisible par le seul utilisateur du service,
# et non dans la ligne de commande (visible de tous via /proc).
EnvironmentFile=/etc/ledgeralps/env

# Durcissement : le service n'a besoin d'écrire que dans son répertoire de données.
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ledgeralps

[Install]
WantedBy=multi-user.target
```

```bash
sudo install -d -m 0750 -o ledgeralps -g ledgeralps /var/lib/ledgeralps /etc/ledgeralps
printf 'JWT_SECRET=%s\nSQLITE_PATH=/var/lib/ledgeralps/ledgeralps.db\n' "$(openssl rand -hex 32)" \
  | sudo tee /etc/ledgeralps/env > /dev/null
sudo chmod 0600 /etc/ledgeralps/env
sudo systemctl enable --now ledgeralps
```

## 6. Accès réseau

**Par défaut, LedgerAlps n'écoute que sur `127.0.0.1`** : il n'est joignable que
depuis sa propre machine, et le trafic ne touche aucun câble. C'est le mode
normal du produit.

> Jusqu'à la v1.4.5, le serveur écoutait sur **toutes** les interfaces. Un
> portable sur un réseau public servait donc sa comptabilité en clair à ce
> réseau, sans que personne l'ait choisi. Ce n'est plus le cas.

Pour y accéder depuis d'autres postes :

```bash
export HOST=0.0.0.0            # ou l'adresse de l'interface concernée
```

**Ce choix impose TLS**, et LedgerAlps s'en charge :

| Situation | Comportement |
|---|---|
| `HOST` loopback | HTTP simple — rien à chiffrer, le trafic ne sort pas |
| `HOST` réseau + `TLS_CERT`/`TLS_KEY` | HTTPS avec votre certificat |
| `HOST` réseau, sans certificat | **Certificat auto-signé généré** dans `<données applicatives>/tls`, HTTPS |
| `HOST` réseau + `ALLOW_INSECURE_HTTP=true` | HTTP en clair, avec un avertissement au journal |

Le certificat auto-signé couvre `localhost`, le nom de la machine et ses
adresses IP, il est valable dix ans et **réutilisé d'un démarrage à l'autre** —
l'exception que vous accordez dans le navigateur tient. Le navigateur avertira
à la première visite : c'est le prix d'un certificat sans autorité de
certification, et il reste inférieur à celui d'un mot de passe en clair sur le
réseau.

Pour éviter l'avertissement, fournissez un certificat d'une autorité interne, ou
placez un reverse proxy (nginx, Caddy) devant.

```bash
export ALLOWED_ORIGINS="https://compta.entreprise.local"
```

### Pourquoi HTTP sur localhost

C'est une question légitime, et la réponse n'est pas « par simplicité ».

**Le trafic vers `127.0.0.1` ne touche aucune interface réseau.** Il ne passe ni
par votre carte réseau, ni par un câble, ni par un routeur. Wireshark branché sur
le réseau ne verra jamais rien : il n'y a rien à voir. Capturer la boucle locale
suppose d'exécuter du code **sur la machine**, avec des privilèges élevés — et à
ce stade l'attaquant lit directement le fichier SQLite, la mémoire du processus,
ou enregistre vos frappes. TLS ne l'en empêcherait pas.

Ce n'est pas une opinion isolée : la plateforme web elle-même classe
`http://localhost` parmi les [origines dignes de confiance](https://w3c.github.io/webappsec-secure-contexts/),
au même titre qu'HTTPS. C'est pourquoi les API réservées aux contextes sécurisés
y fonctionnent sans certificat.

**Le coût de forcer TLS localement est réel.** Un certificat auto-signé déclenche
un avertissement à chaque nouveau profil de navigateur. Habituer quelqu'un à
cliquer sur « Continuer malgré tout » entraîne exactement le réflexe dont vit
l'hameçonnage. Et le supprimer voudrait dire installer une autorité de
certification dans le magasin de confiance de Windows : une clé capable de
forger des certificats **pour n'importe quel site**, posée sur le poste — un
risque supérieur à celui qu'on prétend traiter.

**Cette option a existé, et a été retirée.** Une pré-version proposait de servir
en HTTPS sur `localhost` ; elle a été supprimée avant publication. Elle
n'apportait aucune sécurité réelle — le trafic ne quitte toujours pas la
machine — et coûtait un avertissement de certificat à chaque nouveau profil de
navigateur. Dépenser la confiance des utilisateurs dans les avertissements pour
ne rien protéger est un mauvais échange.

Si votre politique de sécurité exige TLS jusqu'au poste de travail, la réponse
n'est pas dans LedgerAlps : c'est le chiffrement du disque (BitLocker, LUKS),
qui protège la base de données elle-même — la vraie cible sur une machine à
laquelle un attaquant a accès.

⚠️ **`ALLOW_INSECURE_HTTP` n'a qu'un usage légitime** : un reverse proxy qui
termine déjà TLS sur la **même** machine. Partout ailleurs, mot de passe de
connexion, jeton de session et phrase de passe de chiffrement des sauvegardes
traversent le réseau en clair, lisibles par quiconque écoute le segment — ce que
la LPD art. 8 et l'OPDo art. 3 demandent précisément d'empêcher.

```bash
export ALLOWED_ORIGINS="https://compta.entreprise.local"
```

⚠️ **N'exposez pas LedgerAlps directement sur Internet.** Il n'y a ni
authentification à deux facteurs, ni gestion multi-utilisateurs à rôles
(prévue, voir la [roadmap](../ROADMAP.md)). Le mode d'emploi prévu est le
réseau local, éventuellement via VPN.

## 7. Sauvegardes

Le fichier SQLite est **l'unique copie** de pièces que le CO art. 958f impose de
conserver dix ans.

```bash
ledgeralps-cli backup --keep=14        # instantané cohérent, serveur en marche
ledgeralps-cli backups                 # lister
ledgeralps-cli restore --file=… --confirm   # serveur ARRÊTÉ
```

Un instantané est pris automatiquement au démarrage si le dernier date de plus
de 24 h. Les sauvegardes vont dans `<données applicatives>/backups`.

**À faire vous-même :** copier ces instantanés hors de la machine (NAS, disque
externe chiffré). Une sauvegarde qui vit sur le disque qui meurt ne sauve rien.

### Sauvegardes chiffrées

```bash
export BACKUP_PASSPHRASE='une phrase de passe distincte du mot de passe de session'
ledgeralps-cli backup                              # instantané chiffré
ledgeralps-cli restore --file=….db.enc --confirm   # serveur ARRÊTÉ
```

Argon2id dérive la clé, XChaCha20-Poly1305 chiffre le contenu — pur Go, aucune
dépendance système. Le fichier prend le suffixe `.enc`, ce qui montre d'un coup
d'œil quelles copies sont protégées.

**La copie en clair n'est effacée qu'après vérification** : l'instantané chiffré
est relu, déchiffré, et son intégrité SQLite contrôlée. Une sauvegarde qu'on ne
peut pas restaurer n'est pas une sauvegarde, et le moment où on s'en aperçoit
est celui où on en avait besoin.

⚠️ **Conservez la phrase de passe ailleurs que sur cette machine.** Sans elle,
l'instantané est définitivement illisible — pour vous comme pour quiconque.
Et choisissez-la **distincte du mot de passe de session** : sinon, perdre le
poste revient à perdre aussi les sauvegardes.

### Que contient une sauvegarde ?

Un instantané est une copie complète de la base SQLite — un seul fichier,
cohérent, écrit par `VACUUM INTO` pendant que le serveur tourne.

Il contient : `invoices` et `invoice_lines` (factures, offres de prix, notes de
crédit), `supplier_invoices` et `supplier_invoice_lines`, `contacts`,
`journal_entries` et `journal_lines`, `accounts`, `payments`, `fiscal_years`,
`company_settings` (logo compris, stocké en base), `users`, `audit_logs`,
`security_events` et `refresh_tokens`.

Il ne contient **pas** :

| Hors sauvegarde | Conséquence |
|---|---|
| Les binaires | Réinstaller LedgerAlps suffit |
| `config.json` (dont `JWT_SECRET`) | Les sessions en cours sont invalidées : il faut se reconnecter. Les données sont intactes |
| Les sauvegardes elles-mêmes | Une sauvegarde ne se sauvegarde pas ; copiez le dossier hors de la machine |

### La copie d'annulation

Avant toute restauration, LedgerAlps sauvegarde la comptabilité **actuelle**.
C'est l'annulation : on ne découvre pas qu'on a restauré la mauvaise sauvegarde
pendant l'opération, mais après avoir regardé les données. Sans cette copie, la
comptabilité en place serait effacée sans retour.

Elle est prise **au moment de la préparation**, pas au redémarrage — le seul
instant où vous êtes présent avec votre phrase de passe. Elle est donc
**chiffrée avec la même phrase que la sauvegarde restaurée** : quelqu'un qui
choisit de chiffrer ses sauvegardes n'a pas choisi de voir apparaître à côté une
copie en clair de toute sa comptabilité.

Elle apparaît dans la liste comme n'importe quel autre instantané et suit la
même rotation. Il n'y a plus de fichiers `pre-restore` en clair qui
s'accumulent : les versions antérieures à la v1.4.4 en produisaient, vous pouvez
les supprimer.

### Pourquoi la base de données est en clair

`ledgeralps.db` est un fichier SQLite ordinaire : lisible par n'importe quel
outil, sur n'importe quelle machine. C'est un choix subi, pas assumé, et il vaut
d'être expliqué.

SQLite ne chiffre pas nativement. Les extensions qui le font — SQLCipher, SEE —
sont des **bibliothèques C**. LedgerAlps compile avec `CGO_ENABLED=0` sur un
pilote SQLite en Go pur, et c'est précisément ce qui lui donne son binaire
unique, sans dépendance système, compilable pour Windows depuis Linux. Adopter
SQLCipher mettrait fin aux deux.

**Les options réelles, avec leur coût :**

| Option | Ce qu'elle protège | Ce qu'elle coûte |
|---|---|---|
| **Chiffrement du disque** (BitLocker, LUKS) | Tout : base, sauvegardes, `config.json`, et le reste du poste | Rien pour l'application. C'est une mesure du système d'exploitation |
| Chiffrement applicatif de colonnes | Les seules colonnes sensibles | Plus de recherche ni de tri SQL dessus : lister vos contacts imposerait de déchiffrer chaque ligne. Et la clé reste sur la machine |
| SQLCipher (CGO) | La base entière | Fin du binaire unique et de la compilation croisée ; une chaîne d'outils par plateforme |
| VFS chiffré en Go pur | La base entière | Aucune implémentation que nous jugions assez mûre pour des données comptables |

**Le point commun de toutes les solutions logicielles** : le serveur doit
pouvoir lire la base **sans intervention humaine** au démarrage. La clé vit donc
sur la même machine que le fichier qu'elle protège. Contre un portable volé
éteint, le chiffrement du disque gagne. Contre une machine allumée et
compromise, rien au niveau applicatif n'aide — l'attaquant lit la mémoire du
processus, où la base est déchiffrée de toute façon.

**Donc : chiffrez le disque.** C'est la mesure qui protège réellement, elle est
gratuite sur Windows Pro et Linux, et elle couvre aussi le secret de signature
dans `config.json` — que le chiffrement de la base laisserait, lui, en clair.

### Comment le chiffrement fonctionne

Deux briques, aucune dépendance système — tout est en Go pur, ce qui préserve
le binaire unique.

**Argon2id** transforme votre phrase de passe en clé. C'est délibérément lent et
gourmand en mémoire (64 Mio, 3 passes) : un attaquant qui détient le fichier
peut essayer des phrases sans limite ni surveillance, alors chaque essai doit
lui coûter cher. Un sel aléatoire de 16 octets est tiré à chaque sauvegarde,
si bien que deux fichiers protégés par la même phrase ne se ressemblent pas.

**XChaCha20-Poly1305** chiffre le contenu, par blocs de 1 Mio. Ce n'est pas
seulement du chiffrement : c'est du chiffrement *authentifié*. Chaque bloc porte
une empreinte qui inclut son numéro d'ordre et le fait qu'il soit le dernier.
Conséquence pratique : un fichier altéré, tronqué par une clé USB défaillante,
ou dont on aurait retiré des blocs est **refusé**, au lieu de produire une
comptabilité silencieusement amputée.

Ces deux algorithmes sont des standards publics et audités — Argon2 a remporté
la *Password Hashing Competition* (2015), ChaCha20-Poly1305 est normalisé par la
[RFC 8439](https://www.rfc-editor.org/rfc/rfc8439) et utilisé par TLS 1.3, SSH
et WireGuard. Ils ne sont ni maison, ni exotiques.

**La phrase de passe exige au minimum 16 caractères**, avec minuscule, majuscule
et chiffre. C'est plus long que pour une connexion, et volontairement : une
connexion est protégée par la limitation des tentatives et le verrouillage de
compte, un fichier emporté ne l'est par rien.

### Ce que dit la loi

> **LPD art. 8 al. 1** (RS 235.1) — « Les responsables du traitement et les
> sous-traitants doivent assurer, par des mesures organisationnelles et
> techniques appropriées, une sécurité adéquate des données personnelles par
> rapport au risque encouru. »

L'ordonnance d'application ([OPDo, RS 235.11](https://www.fedlex.admin.ch/eli/cc/2022/568/fr))
précise ces mesures : son art. 1 demande d'évaluer le besoin de protection selon
le type de données et le risque, son art. 3 exige des mesures assurant la
confidentialité, la disponibilité et l'intégrité.

**Aucun de ces textes ne nomme d'algorithme.** La loi impose un résultat —
une sécurité proportionnée au risque — et non un moyen. Argon2id et
XChaCha20-Poly1305 sont donc notre réponse à cette exigence, pas une obligation
légale que nous exécuterions. Le raisonnement est celui-ci : une sauvegarde
contient des données de clients identifiables et l'intégralité d'une
comptabilité ; elle voyage sur des supports amovibles ; sa perte serait une
violation de la sécurité des données au sens de l'art. 8 al. 2. Le besoin de
protection est donc élevé, et des primitives modernes avec un facteur de travail
élevé sont proportionnées.

> Cette page décrit ce que fait le logiciel. Elle ne constitue ni un avis
> juridique, ni une certification : votre conformité dépend aussi de l'usage que
> vous faites de ces sauvegardes.

> Le chiffrement de la base elle-même (au repos, en fonctionnement) n'est pas
> disponible : il exigerait SQLCipher, une bibliothèque C, donc l'abandon de la
> compilation croisée et du binaire unique. Voir la [roadmap](../ROADMAP.md).
> En attendant, sur un poste mobile, chiffrez le disque (BitLocker, LUKS).

## Vérification de mise à jour

C'est le **seul** appel réseau sortant de LedgerAlps. Il demande à GitHub s'il
existe une version plus récente et, le cas échéant, affiche une bannière : les
mises à jour portent les correctifs de conformité (QR-facture, TVA), et une
version périmée finit par produire des factures que les banques refusent.

Aucun identifiant ni donnée utilisateur n'est transmis, le résultat est mis en
cache 24 h et tout échec est silencieux. Pour une installation totalement isolée :

```bash
export UPDATE_CHECK=false
```

## Variables d'environnement

| Variable | Défaut | Rôle |
|---|---|---|
| `JWT_SECRET` | — | **Obligatoire**, ≥ 32 caractères |
| `SQLITE_PATH` | `ledgeralps.db` | Chemin de la base SQLite |
| `POSTGRES_DSN` | vide | Si défini, PostgreSQL remplace SQLite |
| `PORT` | `8000` | Port d'écoute |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | Origines CORS, séparées par des virgules |
| `DEBUG` | `false` | Journalisation verbeuse |
| `LOG_LEVEL` | `INFO` | Niveau de journal |
| `UPDATE_CHECK` | `true` | Vérifier l'existence d'une version plus récente. `false` = aucun appel réseau |
| `BACKUP_PASSPHRASE` | vide | Si définie, chiffre les sauvegardes. À conserver hors de la machine |
| `HOST` | `127.0.0.1` | Interface d'écoute. Toute valeur non-loopback impose TLS |
| `TLS_CERT` / `TLS_KEY` | vide | Certificat et clé. À fournir ensemble |
| `ALLOW_INSECURE_HTTP` | `false` | Sert en clair sur le réseau. Uniquement derrière un proxy TLS local |

> **Attention à la précédence.** Si un fichier `config.json` existe dans le
> répertoire de données applicatives, il **prime sur ces variables**. C'est
> surprenant et cela a déjà conduit à viser la mauvaise base : pour cibler une
> base précise en ligne de commande, passez `--sqlite-path` à `ledgeralps-cli`.
