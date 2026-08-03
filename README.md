# LedgerAlps

**Comptabilité et facturation pour les PME et indépendants suisses.**

Vos données restent sur votre machine. Pas de cloud, pas d'abonnement, pas de
compte à créer chez qui que ce soit.

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CI](https://github.com/kmdn-ch/LedgerAlps/actions/workflows/test.yml/badge.svg)](https://github.com/kmdn-ch/LedgerAlps/actions/workflows/test.yml)

---

## Installation

Téléchargez l'installeur depuis la page
**[Releases](https://github.com/kmdn-ch/LedgerAlps/releases/latest)** :

```
LedgerAlps_Setup_<version>_windows_amd64.exe
```

Double-cliquez, suivez l'assistant. Au premier lancement, LedgerAlps ouvre
votre navigateur et vous demande de créer votre compte administrateur. C'est
tout — il n'y a rien à configurer.

> Vos données sont enregistrées dans `%APPDATA%\LedgerAlps\` et sont conservées
> lors des mises à jour comme des désinstallations.

*Linux : voir le [guide de déploiement](docs/PRODUCTION.md).*

*LedgerAlps est publié pour **Windows x86-64** et **Linux x86-64**. Sur un PC
Windows ARM, l'installeur ci-dessus fonctionne par émulation. Pour macOS ou
ARM, il faut compiler depuis les sources — voir la
[roadmap](ROADMAP.md#plateformes-prises-en-charge) pour le pourquoi.*

---

## Ce que fait LedgerAlps

**Facturation**
Créez vos factures et vos offres de prix, suivez leur statut, générez le PDF
avec le bulletin **QR-facture** suisse conforme au standard des banques.
Une offre acceptée se convertit en facture en un clic : l'offre est conservée
telle qu'elle a été envoyée, et les deux documents restent liés.

**Comptabilité**
Journal en partie double, plan comptable PME suisse préchargé, grand livre,
balance de vérification, bilan et compte de résultat.

**TVA**
Déclaration selon la méthode effective ou les taux de la dette fiscale nette
(TDFN), avec l'arrondi suisse à 5 centimes. Les factures fournisseurs
alimentent l'impôt préalable déductible.

**Banque**
Export de virements et import de relevés bancaires aux formats ISO 20022
(`pain.001`, `camt.053`).

**Conformité**
Écritures scellées par empreinte numérique (CO art. 957a), archive légale
10 ans (CO art. 958f), et une **veille automatique** qui vous prévient dans
l'application quand une loi ou un standard qui vous concerne évolue.
**Paramètres → Maintenance** vous laisse vérifier vous-même que rien n'a été
modifié ni retiré, d'un bouton : LedgerAlps recalcule chaque empreinte et
contrôle le chaînage — c'est ce qui rend une **suppression** visible, qu'une
vérification entrée par entrée ne verrait pas. Vous y clôturez aussi vos
exercices : une fois bouclé, un exercice n'accepte plus aucune écriture, pas
même antidatée. Et vous en téléchargez une **attestation d'intégrité** à
remettre à votre fiduciaire, qui dit aussi bien ce qu'elle prouve que ce
qu'elle ne prouve pas.

**Vos données restent les vôtres**
L'archive légale (**Paramètres → Maintenance**) contient dix ans de pièces en
JSON *et* en CSV : ouvrable dans un tableur, importable dans un autre logiciel
comptable. Le verrouillage fournisseur ne tient pas au refus d'exporter, il
tient au format de l'export.

**Sauvegardes**
Un instantané de votre comptabilité est pris automatiquement, et vous pouvez
en déclencher un à tout moment depuis **Paramètres → Sauvegardes**, en le
protégeant par une phrase de passe.

---

## Pourquoi local plutôt que cloud

| | LedgerAlps | Solutions cloud |
|---|---|---|
| Où sont vos données | sur votre machine | sur les serveurs d'un tiers |
| Coût | gratuit, open source | abonnement mensuel |
| Si le fournisseur ferme | rien ne change | vous devez migrer |
| Accès à vos données | fichier que vous possédez | export, quand c'est proposé |
| Fonctionne hors ligne | oui | non |

---

## Questions fréquentes

**Mes données partent-elles quelque part ?**
Non. LedgerAlps fonctionne entièrement sur votre machine. Les avis de
conformité sont livrés avec l'application, sans appel réseau.

**Que se passe-t-il si je perds mon ordinateur ?**
Vous perdez votre comptabilité si vous n'avez pas de copie ailleurs. Les
sauvegardes automatiques sont écrites sur le **même** disque : copiez-les
régulièrement sur un support externe. Le CO impose de conserver vos pièces dix
ans.

**Que contient une sauvegarde ?**
Tout ce que LedgerAlps enregistre, en un seul fichier : vos factures, offres de
prix et notes de crédit avec leurs lignes, vos contacts, le journal et le plan
comptable, les paiements, les exercices, les factures fournisseurs, les
paramètres de votre société — logo compris — et le journal d'audit.

Ce qui n'y est **pas** : le logiciel lui-même, et le fichier de configuration
qui contient la clé de session. Restaurer sur une autre machine ramène donc
toute votre comptabilité ; il faudra simplement vous reconnecter.

**Mes données sont-elles chiffrées sur le disque ?**
Vos **sauvegardes** peuvent l'être, si vous indiquez une phrase de passe. La
**base de données elle-même n'est pas chiffrée** : SQLite ne le fait pas
nativement, et les extensions qui le permettent obligeraient à abandonner le
principe d'un logiciel qui s'installe sans rien d'autre.

La bonne protection est ailleurs, et elle est gratuite : **activez le
chiffrement du disque** — BitLocker sur Windows, LUKS sur Linux. Il protège la
base, les sauvegardes, et tout le reste de votre ordinateur. Le
[guide de déploiement](docs/PRODUCTION.md#pourquoi-la-base-de-données-est-en-clair)
détaille pourquoi les autres options coûtent plus qu'elles ne rapportent.

**Puis-je annuler une restauration ?**
Oui. Avant de restaurer, LedgerAlps sauvegarde d'abord votre comptabilité
actuelle — protégée par la même phrase de passe que la sauvegarde que vous
restaurez. Si vous vous êtes trompé de fichier, elle est là, dans la liste.

**Les sauvegardes sont-elles chiffrées ?**
Oui, si vous le demandez. À la création, LedgerAlps vous propose une phrase de
passe : le fichier devient alors illisible sans elle. C'est ce qui compte quand
la copie part sur une clé USB ou un NAS.

Deux points à retenir : choisissez-la **différente de votre mot de passe de
connexion**, et notez-la **ailleurs que sur cet ordinateur**. Sans elle,
personne ne peut ouvrir la sauvegarde — vous non plus.

Le détail technique — et pourquoi ces choix — est dans le
[guide de déploiement](docs/PRODUCTION.md#comment-le-chiffrement-fonctionne).

**Puis-je l'utiliser à plusieurs ?**
Un seul compte pour l'instant. La gestion multi-utilisateurs avec rôles — dont
un accès en lecture seule pour votre fiduciaire — est planifiée.

**Est-ce que je peux faire confiance aux calculs ?**
Le code est ouvert et vérifiable, et la conformité QR-facture, TVA et
comptable est couverte par des tests automatisés. Cela dit, **LedgerAlps est
un outil, pas un conseil fiscal** : votre fiduciaire reste votre interlocuteur.

---

## Documentation

| Document | Pour qui |
|---|---|
| [Déploiement serveur](docs/PRODUCTION.md) | installer sur Linux ou un serveur du bureau |
| [Architecture](docs/ARCHITECTURE.md) | comprendre comment c'est construit |
| [Développement](docs/DEVELOPMENT.md) | compiler, tester, contribuer |
| [Référence API](docs/API.md) | intégrer un autre outil |
| [Veille de conformité](compliance/README.md) | suivi des évolutions légales |
| [Branches et releases](docs/BRANCHING.md) | processus de publication |
| [Roadmap](ROADMAP.md) | ce qui arrive ensuite |

---

## Contribuer

Les contributions sont bienvenues — voir
[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) pour compiler le projet et
[docs/BRANCHING.md](docs/BRANCHING.md) pour le processus.

Signalements de bugs et demandes de fonctionnalités :
[Issues](https://github.com/kmdn-ch/LedgerAlps/issues).

## Licence

[MIT](LICENSE) — libre d'utilisation, y compris commerciale.
