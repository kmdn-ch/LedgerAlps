# LedgerAlps

Une solution open source de gestion pour les devis, la facturation et la comptabilité, spécifiquement conçue pour répondre aux exigences légales et fiduciaires suisses, tout en restant extensible aux normes européennes.

## Présentation

Ce projet propose une base logicielle robuste pour les indépendants et les PME. Contrairement aux solutions SaaS traditionnelles, il s'agit d'un outil **local-first** : aucune dépendance au cloud, aucun abonnement et une maîtrise totale des données.

## Fonctionnalités Clés

* **Gestion des Devis et Facturation :** Création, suivi et archivage des documents commerciaux.
* **Comptabilité Double :** Journal, Grand Livre et Balance de vérification conformes aux pratiques fiduciaires.
* **QR-facture Suisse :** Génération native des QR-codes de paiement selon les standards de la place financière suisse (STUZZA/Six-Group).
* **Gestion de la TVA :** Prise en charge des taux effectifs et de la méthode des taux de la dette fiscale nette (TDFN).
* **Interopérabilité Bancaire :** Support du standard **ISO 20022** pour les flux de paiements (`pain.001`) et les relevés bancaires (`camt.053`/`camt.054`).

## Conformité Légale et Normes

### Code des Obligations (CO)
Le logiciel est structuré pour respecter les articles **957 à 963 du Code des obligations suisse** :
* **Intégrité et traçabilité :** Journalisation de toutes les écritures pour garantir l'historique des modifications.
* **Clarté et précision :** Structure du plan comptable modulaire selon le modèle PME.
* **Conservation :** Fonctions d'exportation conformes pour l'archivage légal sur 10 ans.

### nLPD (Nouvelle Loi sur la Protection des Données)
Conçu avec le principe de *Privacy by Design* :
* **Local-first :** Les données restent sur votre infrastructure ou machine locale.
* **Sécurité :** Chiffrement des bases de données et gestion fine des accès.

### ISO 20022 & Standards Bancaires
* Standardisation des messages financiers pour une communication fluide avec les banques suisses et européennes.
* Support des fichiers XML requis pour les ordres de virement groupés.

## Architecture Technique

* **Pas de Cloud :** L'application peut être déployée en local ou sur un serveur privé.
* **Indépendance :** Aucune connexion externe obligatoire pour le fonctionnement de base.
* **Extensibilité :** Architecture modulaire permettant l'ajout de modules spécifiques (ex: gestion des stocks, interface fiscale européenne/TVA intracommunautaire).

## Installation

```bash
# Exemple de déploiement via Docker
git clone https://github.com/votre-repo/swiss-accounting.git
cd swiss-accounting
docker-compose up -d
```

## Configuration de la Comptabilité

1.  **Initialisation du Plan Comptable :** Importez le plan comptable suisse standardisé (PME).
2.  **Paramétrage TVA :** Définition des comptes de TVA collectée et déductible.
3.  **Coordonnées QR-facture :** Saisie de l'IBAN/QR-IBAN et de l'ID d'adhérent.

## Contribution

Les contributions sont bienvenues pour améliorer la conformité et ajouter des modules régionaux.
Veuillez vous assurer que toute modification respecte les principes de la comptabilité en partie double et les exigences de la nLPD.

## Licence

Ce projet est distribué sous licence open source MIT. Consultez le fichier `LICENSE` pour plus de détails.

---

**Avertissement Légal** : Bien que ce logiciel soit conçu pour respecter les normes suisses, l'utilisateur demeure responsable de la validation de sa comptabilité auprès d'un expert fiduciaire et du respect des délais de déclaration fiscale.
