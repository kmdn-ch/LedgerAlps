# Déploiement serveur

> **La plupart des utilisateurs n'ont pas besoin de ce document.** Sur Windows,
> l'installeur fait tout : téléchargez-le depuis la page
> [Releases](https://github.com/kmdn-ch/LedgerAlps/releases/latest) et
> double-cliquez — rien d'autre à configurer. Cette page s'adresse à qui veut
> faire tourner LedgerAlps sur une machine Linux x86-64, sans l'installeur
> Windows.

LedgerAlps est **local-first** : il n'écoute que sur la machine où il tourne
(`127.0.0.1`), et il n'existe aucun moyen de l'ouvrir au réseau — voir §6.

---

## 1. Installer le binaire

Téléchargez l'archive depuis la page
[Releases](https://github.com/kmdn-ch/LedgerAlps/releases/latest).

> Les paquets `.deb` et `.rpm` ne sont plus publiés depuis la v1.4.5. Le code
> Go est inchangé et Linux reste une plateforme de compilation **et de test** —
> la CI tourne sur Ubuntu, c'est là que `go test -race` s'exécute. Ce qui
> s'arrête est l'empaquetage : scripts d'installation, unité systemd,
> arborescence FHS. Suivre les conventions de plusieurs distributions demande de
> les vérifier sur de vraies machines, ce que personne ne faisait. L'unité
> systemd ci-dessous reste documentée et fonctionnelle : elle est simplement à
> installer soi-même.

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

### Ce que fixer `JWT_SECRET` désactive

Poser `JWT_SECRET` dans l'environnement du service — c'est ce que fait
`EnvironmentFile=` sous systemd —
**désactive la rotation quotidienne de la clé de signature**.

Ce n'est pas un défaut, c'est une conséquence de la préséance : la variable
d'environnement l'emporte sur le fichier de configuration (voulu, pour que
`SQLITE_PATH` et consorts restent atteignables), et le service la recharge à
chaque démarrage. Une clé régénérée serait donc réécrasée au redémarrage
suivant.

LedgerAlps ne fait plus semblant : dans ce cas, l'écran *Paramètres →
Sécurité des sessions* affiche « rotation désactivée par votre mode
d'installation » au lieu d'une date qui avancerait sans effet.

Deux choix, tous deux défendables :

- **Garder `JWT_SECRET` dans l'environnement.** La clé est stable, connue de
  votre gestionnaire de configuration, et vous la faites tourner vous-même
  quand vous le décidez. Convient à un parc administré.
- **Le retirer après le premier démarrage.** LedgerAlps écrit alors sa propre
  clé dans `config.json` et la fait tourner chaque jour. Convient à une
  installation autonome.

Ce qu'il ne faut pas faire, c'est laisser la variable en place *en croyant*
que la rotation opère.

## 3. Choisir la base de données

**SQLite (recommandé)** — aucun serveur à administrer, sauvegarde = un fichier.

```bash
export SQLITE_PATH=/var/lib/ledgeralps/ledgeralps.db
```

**PostgreSQL — non supporté.** Ce document annonçait le contraire : définir
`POSTGRES_DSN` était présenté comme une bascule de moteur. Elle n'a jamais
fonctionné. Les migrations embarquées sont écrites en SQLite pur — valeurs par
défaut `randomblob()`, `CREATE TRIGGER IF NOT EXISTS`, corps de déclencheur
`RAISE(ABORT, …)` — et la toute première échoue sur PostgreSQL.

Le serveur refuse désormais de démarrer si `POSTGRES_DSN` est défini, en le
disant, plutôt que d'échouer sur une erreur de syntaxe après avoir commencé à
écrire dans votre base.

SQLite n'est pas un pis-aller ici : LedgerAlps tient la comptabilité d'une PME
dans un fichier unique, chiffrable, sauvegardable par copie, sans serveur à
administrer ni à mettre à jour. C'est le moteur pour lequel le produit est
conçu.

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

## 5 bis. Installation sous Windows

Il existe **deux** façons d'installer LedgerAlps sous Windows. **Aucune n'installe de service Windows** :
le lanceur démarre le serveur quand il ne répond pas, ouvre le navigateur puis se termine, et le serveur
continue de tourner seul, jusqu'à l'arrêt de la machine ou de sa session.

| Chemin | Ce qu'il installe | Où vivent les données | Rotation de la clé |
|---|---|---|---|
| **Installeur `.exe` (NSIS)** — le choix par défaut | Lanceur + serveur + CLI, application de bureau | `%APPDATA%\LedgerAlps` (par utilisateur) | **Active**, quotidienne |
| **Archive `.zip`** — décompression manuelle | Les binaires, rien d'autre | où vous lancez le serveur | Active si vous ne posez pas `JWT_SECRET` |

> **Correction de la 1.6.6 (audit croisé Gemini, CRIT-16).** La colonne « Active, quotidienne » de
> l'installeur `.exe` était une promesse que le code ne tenait pas : le lanceur passait
> `JWT_SECRET` dans l'environnement du serveur, ce qui faisait de la clé « une clé venue de
> l'environnement » — et la rotation ne tournait jamais. Il ne le fait plus : le serveur lit la
> clé dans `config.json`, et la rotation s'applique. **À la première mise à jour vers la 1.6.6,
> toute session ouverte se reconnecte une fois** (la clé tourne, son âge étant inconnu).

**Le script `scripts/install.ps1` a été retiré (1.6.6).** Il installait le serveur et le CLI sans
lanceur, ajoutait le dossier au `PATH` et enregistrait, jusqu'à la 1.6.5, un service Windows
qui ne pouvait pas démarrer : `ledgeralps-server.exe` n'appelle jamais `StartServiceCtrlDispatcher`, et le
Gestionnaire de services abandonnait avec l'erreur 1053 (audit croisé Gemini, CRIT-15). Le chemin
« service » avait déjà été retiré sur décision du propriétaire ; le script ne gardait plus que la copie de
deux binaires, ce que l'archive `.zip` fait aussi. L'installeur `.exe` couvre le poste de travail,
l'archive `.zip` le reste.

**Si vous aviez lancé l'ancien script**, un service nommé `LedgerAlps` est peut-être resté enregistré. Il
est inoffensif, mais il échoue à chaque démarrage de la machine. Dans un PowerShell **administrateur** :

```powershell
sc.exe qc LedgerAlps          # vérifier que BINARY_PATH_NAME pointe sur ledgeralps-server.exe
sc.exe delete LedgerAlps      # le retirer (« service inexistant » = rien à faire)
```

Le dossier `C:\ProgramData\LedgerAlps` que créait ce script n'est pas supprimé : il peut contenir une
base et une clé de session. Vérifiez-le avant de l'effacer.

## 6. Accès : cette machine uniquement

**LedgerAlps n'écoute que sur `127.0.0.1`, en HTTP, et rien ne permet de changer cela** (1.6.6).
Il n'est joignable que depuis sa propre machine, et le trafic ne touche aucun câble.
L'adresse n'est pas un réglage : c'est une constante du programme.

> **Historique.** Jusqu'à la v1.4.5, le serveur écoutait sur **toutes** les interfaces : un portable sur
> un réseau public servait sa comptabilité en clair à ce réseau, sans que personne l'ait choisi. De la
> 1.4.5 à la 1.6.5, l'écoute sur le réseau était un choix (`HOST`, écran Paramètres → Sécurité et réseau)
> qui imposait TLS, avec un certificat fourni ou auto-signé. **Ce choix est supprimé en 1.6.6, avec tout
> ce qui le portait** : `HOST`, `TLS_CERT`, `TLS_KEY`, `ALLOW_INSECURE_HTTP`, `ALLOWED_ORIGINS`,
> `TRUSTED_PROXIES`, l'écran, les routes `GET`/`PUT /settings/server`, le certificat auto-signé et le
> sondage TLS du lanceur. Il n'y a donc plus de HTTPS, et plus d'avertissement de certificat.

**Si vous aviez ouvert l'accès au réseau**, l'installation redevient locale à la mise à jour : les autres
postes ne peuvent plus s'y connecter. Le serveur l'écrit au journal au démarrage (`server.log` dans le
dossier de données, `%APPDATA%\LedgerAlps` sous Windows) : « l'accès par le réseau n'existe plus […]
Réglages ignorés : … », avec le nom de ce qu'il a trouvé dans l'environnement ou dans `config.json`. Les
anciennes clés de `config.json` ne sont pas effacées : elles sont ignorées. Pour travailler depuis un
autre poste, installez LedgerAlps sur ce poste et reportez-y les données par une sauvegarde (§7).

Ce que le serveur fait de ce qui arrive :

| Quoi | Comportement |
|---|---|
| En-tête `Host` | Seuls `localhost`, `127.0.0.1` et `::1` sont admis ; tout autre nom reçoit `421`. C'est la défense contre le « DNS rebinding » — et c'est ce qui refuse un mandataire local qui retransmettrait un nom public |
| `X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Proto` | **Ignorés.** L'adresse retenue est celle de la connexion elle-même (verrouillage des connexions, chaîne d'audit) |
| CORS | Les origines de la machine seulement : `http://localhost:<port>` et `http://127.0.0.1:<port>` ; en mode débogage (`DEBUG=true`), aussi le serveur de développement du frontend (port 5173) |
| `Strict-Transport-Security` | **Jamais émis** : un navigateur qui l'enregistre pour `localhost` refuse ensuite `http://` sur cet hôte, et l'utilisateur perd l'accès à sa comptabilité |
| Cookie de session | Sans l'attribut `Secure` (HttpOnly, SameSite=Strict, chemin restreint) : certains navigateurs écartent un cookie `Secure` reçu en `http://localhost` |

Un bureau à distance ou un tunnel SSH vers la machine sont des outils extérieurs à LedgerAlps : il ne les
prévoit pas, ne les teste pas et ne les protège pas.

### Pourquoi HTTP, et pas HTTPS

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

Si votre politique de sécurité exige TLS jusqu'au poste de travail, la réponse
n'est pas dans LedgerAlps : c'est le chiffrement du disque (BitLocker, LUKS),
qui protège la base de données elle-même — la vraie cible sur une machine à
laquelle un attaquant a accès.

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

### Chiffrement au repos : ce qui protège quoi

Trois protections distinctes, qui ne couvrent pas la même chose. Les confondre
conduit à en activer une et à se croire couvert pour les trois.

| | Protège | État par défaut |
|---|---|---|
| **Chiffrement du disque** (BitLocker, LUKS) | Tout : base, sauvegardes, `config.json`, et le reste du poste | À activer par l'utilisateur — c'est une mesure du système d'exploitation |
| **Sauvegardes chiffrées** (Argon2id + XChaCha20-Poly1305) | La copie qui voyage : NAS, clé USB | Actif dès qu'une phrase de passe est enregistrée |
| **Base chiffrée** (VFS adiantum) | Le fichier `ledgeralps.db`, y compris copié ailleurs | Désactivé — option |

**Commencez par le disque.** Il est gratuit, déjà dans votre système, et il
couvre aussi vos documents et vos courriels. Les deux autres ne le remplacent
pas.

#### Chiffrement de la base

`ledgeralps.db` peut être chiffré (Paramètres → Maintenance → Sécurité). Il l'est
alors par blocs de 4 Kio, journal WAL compris.

Cela a longtemps été porté comme impossible, et pour une bonne raison :
SQLCipher est une bibliothèque C, or LedgerAlps compile avec `CGO_ENABLED=0`,
ce qui lui donne son binaire unique et sa compilation croisée. Ce qui a débloqué
la situation n'est pas SQLCipher mais un **changement de pilote SQLite** : le
paquet `vfs` de `modernc.org/sqlite` est en lecture seule, donc rien ne pouvait
s'insérer sous lui, alors que `github.com/ncruces/go-sqlite3` expose un VFS
chiffrant en Go pur. Aucune dépendance C n'a été ajoutée.

**Ce que cela apporte de plus que BitLocker** : une seule chose, mais réelle.
Un disque chiffré ne suit pas le fichier ; la base copiée sur un NAS, un partage
réseau ou un dossier synchronisé reste illisible.

**Ce que cela n'apporte pas** : aucune protection contre un programme lancé sous
le même compte. Il peut demander la clé exactement comme LedgerAlps le fait.

#### Le moment où tout cela se décide

À l'installation. L'assistant du premier lancement demande les deux phrases
après les informations de l'entreprise, et pose les clés **avant de démarrer le
serveur**. La base n'existe pas encore : elle naît chiffrée, sans conversion,
sans redémarrage, et la comptabilité n'est jamais écrite en clair — pas même le
temps d'une migration. La première sauvegarde automatique est déjà chiffrée.

Sur une installation existante, ou si vous avez décliné, le réglage reste
accessible dans Paramètres → Maintenance → Sécurité ; la conversion passe alors
par un redémarrage, parce qu'elle remplace le fichier que le serveur a ouvert.

#### Les deux phrases de passe ne se confondent pas

| | Ce qu'elle ouvre | Quand elle est demandée |
|---|---|---|
| **Phrase des sauvegardes** | les fichiers `.db.enc` | à la restauration d'une sauvegarde |
| **Phrase de récupération de la base** | la clé de la base, rien d'autre | changement de machine ou de compte Windows |

Prenez-les **différentes**. Une seule phrase compromise ouvrirait sinon les deux,
et comme on ne les tape presque jamais, les retenir toutes les deux ne coûte
rien. Celle des sauvegardes est la plus importante des deux : le jour où la
machine a disparu, les sauvegardes sont le seul chemin de retour.

#### La clé, et pourquoi il y en a deux copies

La clé est scellée au compte Windows par DPAPI — le démarrage ne demande donc
rien. Hors Windows, il n'existe pas d'équivalent utilisable par un serveur sans
session de bureau : la clé est alors dans un fichier `0600`, et l'interface le
dit au lieu de laisser croire à une protection absente.

Un profil recréé ou un Windows réinstallé rend ce scellement illisible. Comme
une comptabilité doit être conservée dix ans (CO art. 958f), une mesure de
confidentialité ne peut pas créer une perte de données : une **phrase de
récupération**, obligatoire à l'activation, enveloppe la même clé. Si la clé
scellée devient illisible, LedgerAlps démarre en **mode récupération** — une
page unique qui demande cette phrase, la desselle, la rescelle et relance
l'application.

**Vos sauvegardes ne dépendent jamais de cette clé.** Elles sont produites en
clair puis rechiffrées avec votre phrase de passe de sauvegarde, donc
restaurables sur n'importe quelle machine. Une sauvegarde chiffrée avec la clé
de la machine deviendrait illisible le jour où cette machine n'est plus là :
on échangerait un risque de confidentialité contre un risque de perte totale.

#### Le point commun de toutes les solutions logicielles

Le serveur doit pouvoir lire la base **sans intervention humaine** au démarrage.
La clé vit donc sur la même machine que le fichier qu'elle protège. Contre un
portable volé éteint, elle gagne. Contre une machine allumée et compromise, rien
au niveau applicatif n'aide — l'attaquant lit la mémoire du processus, où la base
est déchiffrée de toute façon.

**LedgerAlps le vérifie.** Au démarrage, l'application lit l'état de BitLocker ;
si le disque est protégé, l'avertissement de conformité disparaît de lui-même.
La vérification a ses limites, et elles sont assumées : détecter BitLocker
correctement exige des droits d'administrateur, que LedgerAlps n'a pas. La clé
de registre lue sans élévation ne couvre que le volume système. Un résultat
positif fait donc taire l'avis ; tout autre le laisse visible, formulé comme
quelque chose à vérifier plutôt que comme un constat.

**Donc : chiffrez le disque.** C'est la mesure qui protège réellement, elle est
gratuite sur Windows Pro et Linux, et elle couvre aussi le secret de signature
dans `config.json` — que le chiffrement de la base laisserait, lui, en clair.

### Horodatage des sauvegardes

Le nom d'une sauvegarde porte l'**heure locale** de la machine, suivie du
décalage UTC :

```
ledgeralps-2026-08-04T20-10-51+0200.db
```

Il était écrit en UTC alors que l'interface affiche l'heure du fichier, que le
système rend en heure locale : une sauvegarde prise à 16 h 23 en Suisse
s'appelait `…T14-23-05` et s'affichait « 16:23 ». Deux heures d'écart entre
l'explorateur de fichiers et le logiciel, sur des fichiers qu'il faut savoir
identifier pendant dix ans (CO art. 958f).

Le décalage est conservé parce que l'heure locale seule est ambiguë une nuit par
an : au passage à l'heure d'hiver, 02 h 30 existe deux fois.

L'ordre des sauvegardes — celui qui désigne la plus ancienne à purger — se lit
désormais sur la **date du fichier**, pas sur son nom. En heure locale, l'ordre
alphabétique se casse cette même nuit-là, et la purge aurait effacé la mauvaise
copie. Les sauvegardes nommées à l'ancien format restent listées et datées
correctement.

---

## Comment le chiffrement fonctionne

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
| `POSTGRES_DSN` | vide | **Non supporté** — s'il est défini, le serveur refuse de démarrer (voir §3) |
| `PORT` | `8000` | Port d'écoute |
| `DEBUG` | `false` | Journalisation verbeuse |
| `LOG_LEVEL` | `INFO` | Niveau de journal |
| `UPDATE_CHECK` | `true` | Vérifier l'existence d'une version plus récente. `false` = aucun appel réseau |
| `BACKUP_PASSPHRASE` | vide | Si définie, chiffre les sauvegardes. À conserver hors de la machine |

`HOST`, `TLS_CERT`, `TLS_KEY`, `ALLOW_INSECURE_HTTP`, `ALLOWED_ORIGINS` et `TRUSTED_PROXIES` n'existent plus
(1.6.6) : le serveur n'écoute que sur `127.0.0.1` (§6). S'ils sont encore posés, ils sont ignorés et le
serveur le dit au journal.

> **Attention à la précédence.** Si un fichier `config.json` existe dans le
> répertoire de données applicatives, il **prime sur ces variables**. C'est
> surprenant et cela a déjà conduit à viser la mauvaise base : pour cibler une
> base précise en ligne de commande, passez `--sqlite-path` à `ledgeralps-cli`.
