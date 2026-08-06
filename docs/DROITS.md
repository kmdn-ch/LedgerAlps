# Droits par type de compte

Inventaire de ce qui est **lisible**, **modifiable** et **cliquable** pour
chacun des trois rôles.

| | Lecture seule | Comptable | Administrateur |
|---|:---:|:---:|:---:|
| **Second facteur exigé** | non | **oui** | **oui** |

**La règle en une phrase** : le comptable fait **tout** sur la comptabilité ;
l'administrateur ajoute la **sécurité du logiciel** et **qui y accède**.

Le rôle est relu **dans la base à chaque requête**, jamais dans le jeton : un
changement s'applique immédiatement, sans attendre l'expiration d'une session.
Masquer un bouton ne protège de rien — chaque refus ci-dessous est appliqué par
le serveur, et l'adresse tapée à la main répond **403**.

Légende : ✅ autorisé · 👁 lecture seule · ⛔ refusé (403)

---

## Tableau de bord

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir les indicateurs, le chiffre d'affaires, les factures récentes | 👁 | ✅ | ✅ |

Les *clients actifs* ne comptent que les contacts **actifs**. Un contact ne se
désactive plus à la main : il s'**anonymise**, ce qui l'écarte des listes et
efface ses données personnelles d'un même geste.

## Facturation — factures, offres de prix, notes de crédit

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir la liste et le détail — **toutes**, quel que soit l'auteur | 👁 | ✅ | ✅ |
| Télécharger un PDF, un lot de PDF | ✅ | ✅ | ✅ |
| Créer, modifier une facture ou une offre | ⛔ | ✅ | ✅ |
| Envoyer, annuler, convertir une offre | ⛔ | ✅ | ✅ |
| Émettre une note de crédit | ⛔ | ✅ | ✅ |
| Enregistrer un paiement | ⛔ | ✅ | ✅ |

## Achats — factures fournisseurs et paiements

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir les factures reçues | 👁 | ✅ | ✅ |
| Saisir une facture fournisseur | ⛔ | ✅ | ✅ |
| Lire le QR d'une facture déposée | ⛔ | ✅ | ✅ |
| Modifier une facture au brouillon | ⛔ | ✅ | ✅ |
| Comptabiliser un achat (écrit au journal) | ⛔ | ✅ | ✅ |
| Voir ce qu'il y a à payer | 👁 | ✅ | ✅ |
| **Produire un ordre de paiement pain.001** | ⛔ | ✅ | ✅ |
| Importer un relevé camt.053, rapprocher | ⛔ | ✅ | ✅ |

## Contacts

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir la liste et les fiches | 👁 | ✅ | ✅ |
| Voir les contacts **désactivés** | ⛔ | ✅ | ✅ |
| Créer et modifier | ⛔ | ✅ | ✅ |
| **Anonymiser** (nLPD art. 6 al. 4) | ⛔ | ✅ | ✅ |

## Journal et plan comptable

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir le journal — **complet**, jamais filtré par auteur | 👁 | ✅ | ✅ |
| Voir le détail d'une écriture et son empreinte | 👁 | ✅ | ✅ |
| Saisir une écriture | ⛔ | ✅ | ✅ |
| **Comptabiliser** (scelle l'écriture) | ⛔ | ✅ | ✅ |
| Voir le plan comptable et la balance | 👁 | ✅ | ✅ |
| Créer un compte au plan | ⛔ | ✅ | ✅ |

## Rapports et exports

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Journal général, grand livre, balance (CSV) | ✅ | ✅ | ✅ |
| Archive légale ZIP (CO art. 958f) | ✅ | ✅ | ✅ |
| Bilan, compte de résultat, déclaration TVA | 👁 | ✅ | ✅ |

C'est la raison d'être du rôle **lecture seule** : remettre les livres à sa
fiduciaire sans lui donner les clés. Elle produit tous les documents, et ne peut
rien modifier.

## Exercices comptables

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir les exercices | 👁 | ✅ | ✅ |
| Créer un exercice | ⛔ | ✅ | ✅ |
| **Clôturer un exercice** (CO art. 958f) | ⛔ | ✅ | ✅ |

## Contrôle d'intégrité et journal d'audit

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Vérifier la chaîne d'empreintes (CO art. 957a) | ⛔ | ✅ | ✅ |
| Lire le journal d'audit | ⛔ | ✅ | ✅ |
| Télécharger l'attestation d'intégrité (Olico art. 9) | ⛔ | ✅ | ✅ |
| Voir la santé du système | ⛔ | ✅ | ✅ |

La lecture seule en est écartée : ce registre nomme **qui a fait quoi**, et le
consulter est déjà sensible.

## Sauvegardes

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Lister les sauvegardes | ⛔ | ✅ | ✅ |
| Créer une sauvegarde | ⛔ | ✅ | ✅ |
| **Restaurer** une sauvegarde | ⛔ | ⛔ | ✅ |
| Politique de chiffrement des sauvegardes | ⛔ | ⛔ | ✅ |

Prendre une copie relève de l'hygiène comptable. **Restaurer** remplace les
livres par une autre version d'eux-mêmes, et la politique de chiffrement est une
fonction de sécurité : les deux restent à l'administrateur.

## Réglages de l'entreprise

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir la fiche entreprise | 👁 | ✅ | ✅ |
| Modifier l'identité, l'IBAN, le n° TVA, le logo | ⛔ | ✅ | ✅ |
| Comptabilisation automatique des factures | ⛔ | ✅ | ✅ |

## Sécurité du logiciel — **administrateur uniquement**

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Chiffrement de la base, phrase de récupération | ⛔ | ⛔ | ✅ |
| Réseau, TLS, port d'écoute | ⛔ | ⛔ | ✅ |
| Clé de signature des sessions | ⛔ | ⛔ | ✅ |
| Déconnexion automatique, réglages de session | ⛔ | ⛔ | ✅ |
| Journal de sécurité (connexions, verrouillages) | ⛔ | ⛔ | ✅ |
| Redémarrer le serveur | ⛔ | ⛔ | ✅ |

## Comptes utilisateurs — **administrateur uniquement**

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Voir la liste des comptes | ⛔ | ⛔ | ✅ |
| Créer un compte, changer un rôle | ⛔ | ⛔ | ✅ |
| Activer ou désactiver un compte | ⛔ | ⛔ | ✅ |
| Réinitialiser l'accès de quelqu'un | ⛔ | ⛔ | ✅ |
| Retirer le second facteur de quelqu'un | ⛔ | ⛔ | ✅ |

## Son propre compte — tous les rôles

Onglet **Paramètres → Mon compte**, visible quel que soit le rôle. Le second
facteur appartient au compte de celui qui le lit, pas à l'administration du
logiciel.

| Action | Lecture seule | Comptable | Admin |
|---|:---:|:---:|:---:|
| Changer son mot de passe | ✅ | ✅ | ✅ |
| Inscrire ou retirer son second facteur | ✅ | ✅ | ✅ |
| Voir ses ordinateurs de confiance, les oublier | ✅ | ✅ | ✅ |

---

## Comment ces droits sont appliqués

**Deux barrières indépendantes**, qui attrapent des erreurs différentes.

1. **La permission déclarée sur la route.** Chaque route d'écriture annonce ce
   qu'elle exige : `read`, `write_documents`, `write_accounting`, `manage`,
   `admin`.
2. **Le filtre global.** Toute méthode d'écriture est refusée à un rôle en
   lecture seule, *quelle que soit la route* — y compris celles qui n'existent
   pas encore. C'est ce qui couvre la route qu'on oubliera d'annoter.

Un troisième filtre existe pour l'état des comptes : mot de passe temporaire à
remplacer, second facteur à inscrire. Tant que c'est dû, le compte ne peut
**rien** faire d'autre — pas même lire.

Aucune de ces barrières ne lit le jeton : toutes interrogent la base.
