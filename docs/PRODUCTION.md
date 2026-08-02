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

LedgerAlps n'implémente pas TLS lui-même. Pour un accès hors de la machine,
placez un reverse proxy devant (nginx, Caddy) et laissez-lui gérer le
certificat.

⚠️ **Sans proxy TLS, tout passe en clair sur le réseau** : mot de passe de
connexion, jeton de session et **phrase de passe de chiffrement des
sauvegardes**. Quiconque écoute le segment les lit. Sur un poste unique la
question ne se pose pas — le trafic vers `localhost` ne quitte pas la machine —
mais dès qu'un second poste se connecte, le proxy n'est pas optionnel.
Le TLS natif est à la [roadmap](../ROADMAP.md).

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

> **Attention à la précédence.** Si un fichier `config.json` existe dans le
> répertoire de données applicatives, il **prime sur ces variables**. C'est
> surprenant et cela a déjà conduit à viser la mauvaise base : pour cibler une
> base précise en ligne de commande, passez `--sqlite-path` à `ledgeralps-cli`.
