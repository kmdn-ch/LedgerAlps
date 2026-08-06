# Changelog

Toutes les modifications notables de LedgerAlps sont documentées ici.  
Format : [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/) — Versioning : [SemVer](https://semver.org/lang/fr/).

---

## [Unreleased]

### Corrigé

- **La liste des fournisseurs de l'écran Achats était toujours vide.** Elle lisait `.items` sur une réponse qui est un **tableau** — `GET /contacts` ne renvoie pas d'enveloppe. Le type TypeScript décrivait fidèlement une réponse qui n'existe pas, donc rien ne pouvait le signaler à la compilation : le défaut ne se voyait qu'à l'écran, sur une liste déroulante qui ne proposait jamais rien.

- **Trois exports des Rapports n'étaient que des maquettes.** « Journal général », « Grand livre » et « Balance de vérification » portaient une pastille « Bientôt disponible », un bouton désactivé et un gestionnaire vide ; l'« Archive annuelle ZIP » annonçait une « fonctionnalité prévue dans une prochaine version » alors que sa route existait et fonctionnait depuis des mois. Une maquette laissée dans une version livrée est pire qu'une absence : elle promet, on planifie autour, et le manque se découvre au moment où l'on en a besoin.

- **Les accents disparaissaient du PDF des factures.** « Bénéficiaire » s'imprimait « B?n?ficiaire », et le même défaut touchait « N° facture », « N° note de crédit » et « Échéance » — quatre libellés, pas un. La conversion vers l'encodage du générateur était appliquée **deux fois** : le premier passage transforme « é » en octet 0xE9, ce qui n'est plus de l'UTF-8 valide ; le second lit cet octet comme un caractère de remplacement, hors de la plage Latin-1, et écrit « ? ». Un test relit désormais le flux PDF produit, et un second interdit le double appel dans le code source.

### Ajouté

- **Journal général, grand livre et balance en CSV.** Séparateur point-virgule et BOM UTF-8 — sans quoi Excel en configuration suisse produit une colonne unique et affiche « GenÃ¨ve ». Chaque fichier porte une ligne de total : un export tronqué se repère à un total qui ne s'équilibre plus. Seules les écritures **comptabilisées** y figurent. Une date mal formée est refusée plutôt qu'ignorée — un export silencieusement non filtré est plus trompeur qu'une erreur, parce qu'il a l'air complet.

  Ces exports sont des **lectures** : un compte en lecture seule peut les produire, c'est même la raison d'être de ce rôle — remettre les livres à sa fiduciaire sans lui donner les clés. Vérifié sur un serveur réel.

- **Création d'un fournisseur depuis l'écran Achats.** Renvoyer vers Contacts au milieu d'une saisie fait perdre ce qui est déjà tapé, et la facture qu'on a sous les yeux vient souvent d'un fournisseur pas encore enregistré. Le fournisseur créé est immédiatement sélectionné.

- **Le taux de TVA se choisit dans une liste** — 8.1 %, 2.6 %, 3.8 %, 0 % — au lieu d'être saisi librement. Ces taux sont fixés par la loi ; un 8.0 au lieu de 8.1 fausse la déclaration et ne se découvre qu'au décompte trimestriel.

- **Le journal dit quand la comptabilisation automatique est éteinte.** Un journal vide après avoir envoyé des factures ressemble à une panne, alors que c'est un réglage — celui, par défaut, des installations créées avant qu'il n'existe. Le message dit où l'activer.

## [1.4.8] — 2026-08-06

### Ajouté

- **Achats — un écran pour saisir, comptabiliser et payer les factures fournisseurs.** L'API existait depuis longtemps ; l'interface, non. Saisir une facture reçue demandait de forger une requête HTTP, ce qui revient à dire que la fonction n'existait pas.

  La comptabilisation **écrit désormais au journal** : charge et TVA déductible au débit, créanciers au crédit, écriture scellée. Le statut « comptabilisée » l'annonçait depuis l'origine — le schéma dit « posted to the journal, counts for VAT » — mais rien ne l'écrivait : la charge n'entrait dans les livres que si quelqu'un la saisissait à la main, pendant que la TVA déductible alimentait déjà la déclaration. Les livres et la déclaration racontaient deux histoires différentes.

- **Ordre de paiement pain.001, utilisable sans forger de requête.** L'écran disait « exportez via l'API `POST /api/v1/payments/export` » et laissait décrire chaque virement à la main dans un corps JSON. On coche maintenant les factures à régler et LedgerAlps produit le fichier XML à déposer dans l'e-banking.

  **Les montants ne viennent pas du navigateur** : la sélection ne transmet que des identifiants, et le créancier, l'IBAN, le montant et la référence sont relus par le serveur dans les livres. C'est la différence entre « payer ces factures » et « virer ces sommes » — dans le second cas, une page web dicterait ce qui part à la banque.

  **Générer n'est pas payer** : aucun statut ne bouge. La facture reste « comptabilisée » jusqu'à ce que le débit apparaisse au relevé, ce qu'établit le rapprochement camt.053. L'écran le dit, parce qu'un bouton suivi d'un silence laisse croire l'affaire réglée.

  Une facture qu'on ne peut pas payer est **montrée** avec ce qui manque et où le corriger — « Aucun IBAN sur la fiche du fournisseur (Contacts → …) » — plutôt que masquée : une dette qui disparaît de l'écran sans explication est pire qu'une dette visible.

- **Référence de paiement sur les factures fournisseurs.** Celle du bulletin de versement, à ne pas confondre avec le numéro de facture du fournisseur. Sans elle le virement part quand même, mais il arrive anonyme et la relance suit.

### Corrigé

- **Le fichier de paiement annonçait le niveau de service « SEPA » sur tous les virements**, y compris en francs vers un IBAN suisse. SEPA ne concerne que les virements en euros dans l'espace SEPA ; l'annoncer décrit un service que l'opération n'utilise pas et expose au rejet par la banque. Il n'est plus posé que pour un paiement en euros.

- **La référence QR était écrite dans `<Cd>`**, l'élément réservé à la liste de codes externes ISO 20022 — où « QRR » ne figure pas. Elle passe en `<Prtry>` ; `SCOR`, qui appartient bien à cette liste, reste en `<Cd>`.

- **La règle QRR ⇔ QR-IBAN est vérifiée avant l'export** (SIX IG QR-facture v2.4 §4.2.2, champs 28 et 29). Une référence QR exige un QR-IBAN, un IBAN ordinaire n'accepte pas de référence QR : l'inadéquation entre les deux est la première cause de rejet bancaire. Le refus nomme la facture, le fournisseur et la correction.

- **Le contenu des cartes touchait le cadre sur la fiche facture.** `.card` ne porte aucune marge intérieure — elle vient de `.card-body`, absent sur ces cartes — si bien qu'un montant aligné à droite semblait sortir de la boîte. Une classe `.card-pad` porte cette marge pour les cartes sans structure interne ; l'ajouter à `.card` l'aurait doublée partout ailleurs.

### Sécurité

- **Les routes d'achat et de paiement déclarent leur permission**, en plus du filtre global qui refuse déjà toute écriture à un rôle en lecture seule. Vérifié sur un serveur réel avec un compte `viewer` : il **consulte** ce qu'il y a à payer (200) mais ne peut ni produire d'ordre de paiement, ni saisir une facture fournisseur, ni comptabiliser, ni importer un relevé bancaire — **403** sur les quatre. Deux barrières qui couvrent des erreurs différentes : une permission oubliée sur une route future, et une route d'écriture non annotée.


### Corrigé

- **Un code de secours était impossible à saisir.** L'écran de vérification invitait à en entrer un dans le champ du code à six chiffres — champ plafonné à **sept caractères**, avec clavier numérique et espacement de chiffres. Un code de secours en fait onze : la moitié se perdait à la frappe. La consigne décrivait un mécanisme qui n'existait pas, au moment précis où l'on a le moins envie de chercher.

  Le passage au code de secours est désormais un **bouton** — « Téléphone perdu ? Utiliser un code de secours » — et le champ change réellement de nature : longueur, clavier, casse, espacement, exemple affiché. Le message d'échec correspond à ce qui a été tenté, au lieu de parler de l'horloge d'un téléphone à quelqu'un qui recopie un papier.

  La saisie est acceptée telle qu'on la tape : majuscules ou minuscules, avec ou sans tiret, avec ou sans espaces. Ces caractères ne portent aucune information, et les exiger transformerait la dernière porte de secours en énigme. L'écran qui remet les codes dit maintenant aussi **où** ils se saisissent.

- **Le bilan et le compte de résultat comptaient les brouillons.** Une écriture jamais comptabilisée — donc scellée par rien, et modifiable — apparaissait dans la balance de vérification, au bilan et au compte de résultat. La condition `status = 'posted'` était portée par une jointure **externe**, qui décide si l'écriture est *rattachée* et non si la ligne est *retenue* : les lignes de brouillon survivaient à la jointure et leurs montants entraient dans les totaux.

  Vérifié sur un serveur réel avant et après : un brouillon de CHF 100 produisait un actif de CHF 100 au bilan et CHF 100 de produits au compte de résultat, avec un bilan déséquilibré par construction. C'est le défaut le plus grave des trois, parce qu'il ne se voyait pas — les états s'affichaient normalement, simplement faux. Trois tests le tiennent désormais fermé.

- **Le journal ne fonctionnait pas — trois défauts indépendants, chacun suffisant.**

  *La liste ne demandait rien au serveur.* Sa source était une promesse résolue sur un tableau vide, vestige de maquette : le journal restait vide même après une écriture réussie.

  *Le formulaire parlait une autre langue que l'API.* Il envoyait `debit_account` / `credit_account` / `amount` là où le serveur attend un compte et un montant au débit ou au crédit, et une seule ligne là où une écriture en comporte au moins deux. **Chaque enregistrement répondait 422**, quelle que soit la saisie.

  *Le message d'erreur accusait toujours la partie double.* Un numéro de compte inexistant, une date mal formée, un montant manquant : tout devenait « vérifiez la partie double ». Un avertissement qui se trompe de cause est pire qu'aucun.

- **La page Plan comptable lisait des champs que le serveur n'envoie pas.** La colonne des numéros lisait `number` là où l'API rend `code` : elle était **vide** sur les quatre-vingt-un comptes. La balance de vérification lisait `account_number`, `debit`, `credit` là où l'API rend `code`, `total_debit`, `total_credit` : chaque colonne affichait « — », et la ligne « Équilibrée ✓ » ne pouvait jamais apparaître puisqu'elle cherchait un total que le serveur ne rend pas. Les types TypeScript décrivaient fidèlement une API qui n'existait plus, donc rien ne pouvait le signaler à la compilation.

- **Le journal se filtrait sur son auteur.** La liste ne montrait à un non-administrateur que les écritures qu'il avait lui-même créées, sous couvert de minimisation des données. C'est un contresens comptable : le journal doit être complet et se rapprocher de la balance (CO art. 957a al. 2 ch. 2 et 3). Deux personnes travaillant sur les mêmes livres voyaient deux journaux différents, tous deux en désaccord avec le bilan. Ce qui protège ici est le rôle, pas un filtre — et la minimisation nLPD porte sur les données personnelles, pas sur des pièces que la loi oblige à conserver dix ans.

### Ajouté

- **Le journal, réellement utilisable.** Les comptes se désignent par leur **numéro** — « 1000 », « 3200 » — et non plus par un identifiant interne que personne ne connaît ; le nom du compte s'affiche sous le champ pendant la frappe, et un numéro inconnu est signalé avant l'envoi. Les totaux débit et crédit se calculent en direct avec l'écart s'il y en a un. Un débit et un crédit sur la même ligne forment une écriture simple ; laisser un côté vide permet une ventilation (une vente répartie entre produit et TVA, par exemple).

  La liste rend le **montant** et l'**auteur** de chaque écriture — la traçabilité du CO art. 957a al. 2 ch. 5 devient lisible plutôt qu'affirmée. Une ligne se déplie sur son détail : chaque compte en clair, débit, crédit, libellé, et l'**empreinte** qui la scelle. Un brouillon n'en porte aucune, et l'écran le dit : « rien ne la scelle et elle ne compte ni à la balance, ni au bilan, ni au compte de résultat ».

  La comptabilisation passe par une confirmation qui énonce ce qu'elle fait — scellement irréversible, entrée dans les états, correction par contrepassation seulement.

- **Les refus disent ce qui ne va pas.** « ligne 1 : le compte 10 n'existe pas dans le plan comptable » plutôt que 422 muet. « l'écriture n'est pas équilibrée : débit 100.00, crédit 10.00, **écart 90.00** » plutôt que « vérifiez la partie double » — un écart de 90.00 sur 100.00 désigne le zéro oublié. Sont aussi refusés : un compte à la fois débité et crédité sur la même ligne (l'écriture s'équilibrerait en s'annulant, laissant deux mouvements fantômes) et un montant négatif (la contrepartie s'écrit de l'autre côté, pas avec un signe).

- **La balance de vérification, en état de servir.** Elle masque par défaut les comptes sans mouvement — soixante-dix-huit lignes à zéro noient le contrôle — avec un repli pour la vue complète que demande une fiduciaire. Le total est calculé et l'équilibre annoncé. Un écart, qui ne devrait jamais survenir puisque le serveur refuse toute écriture déséquilibrée, est traité comme un défaut d'intégrité et renvoie vers la vérification et la sauvegarde, plutôt qu'affiché comme un nombre au milieu d'un tableau.

- `GET /journal/:id` — le détail d'une écriture avec ses lignes, les comptes en clair et l'empreinte d'intégrité.

### Sécurité

- **Le compte administrateur est protégé par un second facteur (code à usage unique, TOTP — RFC 6238).** Un compte administrateur peut créer des comptes, restaurer une sauvegarde, déverrouiller une période et déchiffrer la base. Jusqu'ici, un mot de passe seul l'en séparait — réutilisé sur un autre site, deviné, lu par-dessus l'épaule, il suffisait.

  **Ce que cela protège, et ce que cela ne protège pas.** Le second facteur couvre le cas où le *mot de passe* fuit. Il ne protège **pas** de quelqu'un qui lit déjà le fichier de base : le secret y est, et celui-là n'a besoin d'aucun code. Ce qui répond à cette menace est le chiffrement de la base et celui du disque. Le dire évite de croire couvert un risque qui ne l'est pas.

  **TOTP plutôt qu'autre chose.** Le SMS demande un opérateur, un numéro et un appel sortant — trois choses que LedgerAlps n'a pas et ne veut pas. Le courriel a les mêmes défauts et une faiblesse de plus : la boîte est souvent le compte qu'on cherche justement à protéger. WebAuthn serait plus solide mais exige HTTPS et un matériel que la plupart des PME n'ont pas. TOTP fonctionne hors ligne, avec n'importe quelle application — y compris libre : Aegis, KeePassXC, FreeOTP — et aucun tiers n'est dans la boucle.

  L'algorithme est implémenté ici plutôt qu'importé : il tient en quarante lignes, et les **vecteurs de test officiels de la RFC 6238** sont vérifiés par le suite de tests. Un code ne sert **qu'une fois** — la fenêtre acceptée est enregistrée et refusée ensuite —, la tolérance est d'une fenêtre de trente secondes de chaque côté, et la comparaison est à temps constant.

  **Le blocage est technique.** Vérifié sur un serveur réel : un administrateur non inscrit reçoit **403** sur `/users`, `/backups`, `/contacts`, `/invoices` et `/journal` — en lecture comprise. Les seules routes ouvertes sont celles de l'inscription, montées hors du groupe filtré : les y inclure aurait enfermé le compte hors de sa propre installation.

  **Le jeton d'attente ne vaut rien ailleurs.** Après un mot de passe accepté, la connexion ne délivre ni jeton d'accès ni cookie : seulement un jeton d'attente de cinq minutes, refusé par le filtre d'authentification sur **toute** autre route — donc aussi sur celles qui n'existent pas encore. La vérification est derrière la limitation de tentatives existante : cinq échecs et la porte se ferme quinze minutes.

  **Dix codes de secours, montrés une seule fois, hachés en base.** Sans eux, un téléphone perdu enfermerait définitivement le dernier administrateur — plus personne ne pourrait créer de compte, restaurer une sauvegarde ni rendre le droit de le faire : le second facteur créerait la panne qu'il est censé prévenir. Ils sont hachés comme des mots de passe, ne servent qu'une fois, et leur usage est tracé.

  *À la première connexion après cette mise à jour, l'administrateur sera conduit à inscrire son téléphone avant de pouvoir travailler.* C'est voulu : une protection qu'on peut remettre à plus tard n'est jamais activée. Les autres rôles peuvent l'activer s'ils le souhaitent, sans y être contraints — un comptable écrit dans un journal chaîné et tracé, et n'a pas les clés de l'installation.

- **L'administrateur peut réinitialiser l'accès d'un compte.** Un mot de passe oublié n'avait aucune issue : le produit refuse de supprimer un compte, parce que les écritures portent l'identifiant de leur auteur et que l'effacer casserait la traçabilité du CO art. 957a al. 2 ch. 5.

  « Réinitialiser » **remplace** le mot de passe — il n'est jamais révélé, pas même à l'administrateur — par un mot de passe temporaire de quinze caractères tiré au hasard, affiché **une seule fois** et jamais écrit dans le journal de sécurité. Le compte devra en choisir un autre à sa connexion suivante et ne pourra rien faire avant. Les sessions ouvertes tombent : une réinitialisation sert souvent à reprendre la main sur un compte qu'on craint compromis.

  **Elle ne retire pas le second facteur.** Un administrateur qui pourrait, d'un clic, remettre à zéro le mot de passe *et* le second facteur d'un autre compte pourrait s'y substituer entièrement — le second facteur ne protégerait alors plus de rien face à lui. Le retrait du second facteur est un geste séparé, confirmé séparément et tracé séparément. On ne réinitialise pas son propre accès, ni celui d'un compte désactivé.

- **Un compte créé par un administrateur doit changer son mot de passe à la première connexion, et ne peut rien faire avant.** Le mot de passe choisi *pour* quelqu'un d'autre lui est transmis par message, par téléphone ou sur un papier : il est connu de deux personnes et a voyagé par un canal qui n'est pas fait pour ça. Tant qu'il vaut, l'administrateur peut se connecter au nom de l'autre — et les actions seraient tracées sous un compte qui n'est pas celui de leur auteur réel, ce qui rend le journal d'audit **trompeur** et non simplement incomplet.

  Le blocage est technique : vérifié sur un serveur réel, un compte marqué reçoit 403 sur `GET /invoices`, `GET /contacts`, `GET /reports/*` comme sur toute écriture. La seule route ouverte est le changement lui-même, montée hors du groupe filtré — l'y inclure aurait enfermé le compte définitivement. Le nouveau mot de passe exige 12 caractères, minuscule, majuscule et chiffre, doit différer de l'ancien, et le mot de passe actuel est vérifié même sur un compte marqué : sans quoi un jeton volé suffirait à s'approprier le compte. Les autres sessions tombent au changement.

  Les comptes existants ne sont pas marqués : leur mot de passe a été choisi par leur titulaire, et forcer un changement au prochain démarrage ressemblerait à une panne.

### Ajouté

- **Une saisie en cours ne se perd plus sur un clic à côté.** Le voile d'une fenêtre modale fermait au moindre contact, et un lien du menu emmenait ailleurs sans un mot — alors que LedgerAlps n'enregistre aucun brouillon automatique et qu'une facture de quinze lignes disparaît entièrement. Trois sorties, trois protections : le voile et la croix d'une modale, la navigation interne, et la fermeture de l'onglet. La confirmation n'apparaît que si quelque chose a été saisi — la demander sur un formulaire vide apprendrait à cliquer « Quitter » sans lire.

### Modifié

- **Factures et offres de prix sous un seul libellé** dans le menu — « Facturation ». Ce sont deux vues du même registre : une offre acceptée devient une facture, et les deux se citent. La bascule vit dans l'écran, là où l'on regarde déjà les documents.
- Les mentions « Comptabilité CH » et « Comptabilité suisse » sont retirées ; l'écran de connexion dit « Accédez à votre espace ».

---


### Sécurité

- **Un compte en lecture seule n'accède plus du tout à Sauvegardes ni à Maintenance — en lecture non plus.** Ces écrans exposent le dossier de sauvegarde, l'état du chiffrement, la santé du système et les événements de sécurité : les consulter est déjà sensible. Vérifié sur un serveur réel, `/backups`, `/backups/policy`, `/database/encryption`, `/maintenance/*`, `/settings/server`, `/settings/security`, `/security-events`, `/users` et `/audit-logs` répondent **403** ; l'import camt.053, la création de contact, de facture, d'offre et d'écriture au journal aussi.

  Les onglets correspondants ne sont plus rendus du tout, et une adresse tapée à la main se recale sur un onglet autorisé au lieu d'afficher un panneau vide dont chaque appel échoue. Masquer sans interdire ne protège de rien ; interdire sans masquer use la confiance dans l'interface.

### Ajouté

- **Le compte en cours est annoncé en permanence**, en bas du menu, avec ce que le rôle implique plutôt que son seul nom. « Compte ADMINISTRATEUR — ne pas utiliser pour le travail courant, ce compte peut effacer les sauvegardes, changer les rôles et déchiffrer la base ». « Compte en lecture seule — vous n'êtes pas autorisé à faire des modifications ». Un administrateur qui se croit en lecture seule modifie sans s'en rendre compte, et un compte administrateur laissé ouvert sur un poste partagé est la porte que personne ne pense à fermer.

- **Les actions sur les documents entrent dans la chaîne d'empreintes du CO art. 957a al. 2 ch. 5.** La phrase affichée dans l'interface — « les documents portent le nom de leur auteur » — ne se vérifiait qu'à moitié : la chaîne ne couvrait que le journal. Une facture portait `created_by_id`, c'est-à-dire qui l'avait créée, et **rien** sur qui l'avait envoyée, corrigée ou annulée — soit tout ce qui arrive à une pièce après sa naissance.

  Les traces entrent dans la **même** chaîne que les écritures, avec le même numéro de séquence et la même vérification : réattribuer une action à un autre compte casse l'empreinte, et un test le prouve. Un second registre non chaîné aurait la force d'une table qu'on peut modifier — c'est-à-dire aucune.

  Deux limites, énoncées plutôt que masquées : la trace est écrite **après** l'action et non dans sa transaction, si bien qu'une coupure entre les deux laisse l'action sans trace ; et une action sans auteur connu n'écrit **rien**, plutôt qu'une trace anonyme qui ferait croire à une couverture inexistante.

---


### Ajouté

- **Point 9 — trois rôles : Administrateur, Comptable, Lecture seule.** Le cas central est celui de la fiduciaire : lui ouvrir les livres sans lui donner les clés. Jusqu'ici il n'y avait qu'un interrupteur — administrateur ou non — si bien que partager l'accès revenait à partager le compte, avec le droit de modifier les livres et d'effacer les sauvegardes.

  **Le rôle est lu dans la base à chaque requête, jamais dans le jeton.** Un jeton d'accès vit une heure ; si le rôle y était inscrit, rétrograder ou désactiver quelqu'un le laisserait agir avec ses anciens droits pendant tout ce temps — une heure durant laquelle on croit avoir coupé l'accès. Vérifié sur un serveur réel : avec le **même** jeton, 403 → 201 → 403 au fil des changements de rôle, sans aucune reconnexion.

  `middleware.RequireAdmin`, qui lisait le drapeau administrateur dans le jeton, a été **supprimé** plutôt que déprécié : laissé disponible, il serait repris par réflexe sur la prochaine route d'administration, et le défaut reviendrait sans que rien ne le signale.

  **Deux barrières indépendantes.** Les permissions par route dépendent de ce qu'on a pensé à déclarer ; un filtre global refuse en plus toute méthode d'écriture à un rôle en lecture seule, quelle que soit la route — y compris celles qui n'existent pas encore. Un test exerce précisément une route d'écriture sans garde déclarée.

  **Trois refus protègent l'installation d'elle-même** : on ne retire pas le dernier administrateur, ni en le rétrogradant ni en le désactivant, et on ne change pas son propre rôle. Un administrateur désactivé ne compte pas comme recours. Un compte se désactive et ne se supprime pas — les écritures portent l'identifiant de leur auteur (CO art. 957a al. 2 ch. 5).

### Corrigé

- **Le premier compte d'une installation neuve n'était pas administrateur.** Le bootstrap posait `is_admin = 1` sans toucher au rôle, qui prenait donc sa valeur par défaut — « comptable ». Les droits étant lus dans le rôle, l'installation naissait **inadministrable** : impossible de créer un compte, de restaurer une sauvegarde, ni de se donner le droit de le faire. La migration traitait bien les lignes existantes ; c'est le chemin d'insertion qui avait été oublié. Trouvé en exerçant un serveur réel, pas en relisant le code.

- **Roadmap : « base en clair » n'était plus vrai.** Le tableau de conformité suisse l'affirmait encore alors que le chiffrement de la base est livré depuis la v1.4.8-rc1. Un tableau de conformité qui se trompe sur son propre produit est exactement ce qui apprend à ne plus le lire.

### Modifié

- **Revenir à des sauvegardes en clair demande maintenant confirmation, et dit ce que personne ne devine.** Le bouton agissait au premier clic. Or l'opération n'efface pas seulement la protection des copies à venir : elle **efface la phrase de passe conservée**. Les sauvegardes déjà chiffrées, elles, le restent — et si personne ne l'a notée ailleurs, elles deviennent définitivement illisibles. Le dialogue annonce donc **combien** de copies chiffrées sont en jeu, ce que l'opération ne touche pas, et pourquoi le dossier de sauvegarde est justement celui qui voyage.


- **Les tableaux respirent.** Les montants d'une ligne de facture touchaient le bord : le pas passe de 14 à 13 px, les cellules de bord reçoivent une marge supplémentaire — elles longent un bord arrondi, dont le rayon mange visuellement le padding — et les chiffres passent en `tabular-nums`, ce qui les aligne en colonne et permet de comparer deux totaux d'un coup d'œil.

---


### Ajouté

- **Point 8 — les factures passent au journal à leur envoi.** Débiteurs au débit du total TTC, produits au crédit du hors-taxe, TVA due au crédit du reste ; la note de crédit contrepasse à l'identique. Jusqu'ici aucun document ne passait d'écriture — seuls les paiements étaient automatisés — si bien qu'une facture n'apparaissait au journal qu'à son encaissement, ce qui décale le produit d'un exercice à l'autre.

  **Le réglage est éteint sur les installations existantes**, et c'est délibéré : qui tenait une comptabilité complète saisissait ces écritures à la main, et les automatiser d'office doublerait le produit et la TVA due sur tout un exercice. La migration éteint le réglage pour les fiches déjà présentes et le laisse actif pour les nouvelles. Paramètres → Facturation pour l'allumer après vérification.

- **Point 10 — rapprochement bancaire.** L'import camt.053 existait et ne gardait rien : le relevé était analysé, renvoyé au navigateur, puis oublié. Les écritures sont conservées, avec au plus une suggestion et la raison qui l'a désignée — référence du bulletin (certaine), montant exact sur une seule facture ouverte (probable), et *rien du tout* quand plusieurs factures correspondent, parce que désigner la première serait un tirage au sort présenté comme une analyse.

  **Rapprocher n'encaisse pas** : identifier un versement et enregistrer un paiement restent deux gestes. Solder une créance parce qu'un montant correspondait est une erreur qu'on ne découvre qu'en relançant un client qui a déjà payé.

- **Point 12 — dossier de validation SIX.** Chaque facture produit une archive à déposer sur le portail des Swiss Payment Standards : le payload exact du QR, le bulletin, la marche à suivre. Le payload sort de la **même fonction que l'impression**, extraite pour cela — deux constructions séparées divergeraient, et on ferait alors valider autre chose que ce qu'on envoie aux clients. Le compte du portail reste à créer par l'utilisateur : un logiciel n'ouvre pas de compte au nom de quelqu'un d'autre.

### Écarté

- **Point 11 — eBill.** Vérifié sur la spécification publique de SIX : eBill n'est pas un format qu'on produit mais une **API REST OAuth 2.0 exposée par un « Network Partner »**, avec un contrat entre votre entreprise et ce partenaire, un identifiant émetteur attribué par lui, et des jetons contre son serveur. LedgerAlps ne peut rien envoyer sans un contrat qu'il n'est pas en position de signer — et transmettre supposerait des appels sortants, ce que le produit ne fait pas.

  Ce que le réseau accepte, LedgerAlps le produit déjà : la charge utile d'un dépôt est un PDF, l'en-tête `X-BCFORMAT` pouvant valoir `QRBill`. Qui signe avec un Network Partner dépose donc le bulletin imprimé par LedgerAlps.

---


### Ajouté

- **La protection se choisit à l'installation.** L'assistant du premier lancement enchaîne désormais trois écrans : l'entreprise et le compte, puis la protection de la base, puis celle des sauvegardes.

  C'est le bon moment, et pas seulement pour l'ergonomie : **la base n'existe pas encore**. En posant la clé avant que le serveur démarre, elle naît chiffrée — aucune conversion, aucun redémarrage, et la comptabilité n'est jamais écrite en clair, pas même le temps d'une migration. La phrase de passe des sauvegardes est enregistrée dans le même mouvement, si bien que l'instantané pris au tout premier démarrage est déjà chiffré.

- **Changement de la phrase de récupération** sans toucher à la clé ni aux données (Paramètres → Maintenance → Sécurité). Le chiffrement se décidant maintenant à l'installation, quelqu'un qui a mal noté sa phrase — ou qui l'a montrée — devait pouvoir en changer autrement qu'en déchiffrant toute la base.

- **Déconnexion automatique après inactivité**, dix minutes par défaut, réglable de deux minutes à une heure ou désactivable.

  Une minute d'avertissement compté à rebours précède la coupure, avec un bouton pour rester. Ce n'est pas de la politesse : LedgerAlps n'enregistre aucun brouillon automatique, et une facture de quinze lignes disparaîtrait sans un mot. L'activité comptée est celle de l'utilisateur — clavier, souris, molette — et non le trafic réseau : une page qui recharge ses données toutes les trente secondes ne doit pas maintenir la session de quelqu'un parti déjeuner. Pendant l'avertissement, il faut cliquer : un mouvement de souris ne suffit plus, sinon un écran tactile dans une poche garderait la session ouverte indéfiniment.

### Modifié

- **La clé de signature est régénérée automatiquement**, chaque jour par défaut, au lieu d'attendre un clic. Un bouton de sécurité qu'il faut penser à presser n'est pas une mesure, c'est une intention — et dans les faits la clé ne tournait jamais.

  Elle tourne **au démarrage, jamais en cours de session** : la régénérer invalide toutes les sessions, et couper au milieu d'une saisie ferait perdre le travail. Au démarrage, la seule conséquence est une reconnexion, au moment où l'on ouvre l'application de toute façon. La périodicité se règle, et le bouton manuel reste : il couvre ce que la périodicité ne couvre pas — la fuite dont on vient de s'apercevoir, où attendre le prochain cycle serait trop long.

- **Les deux phrases de passe sont distinguées partout où on les saisit.** Elles ne protègent pas la même chose et les confondre est le vrai risque : celle des **sauvegardes** ouvre les fichiers `.db.enc` et reste le seul chemin de retour quand la machine a disparu ; celle de **récupération** ne sert qu'à retrouver la clé de la base sur un autre compte Windows et n'ouvre aucune sauvegarde. L'interface le dit à l'endroit où l'on tape, et signale le cas où les deux sont identiques.

- **La robustesse est évaluée pendant la frappe** sur les deux, et non plus sur la seule phrase de sauvegarde. La jauge encourage la longueur au-delà du minimum plutôt que les astuces de composition : face à une attaque hors ligne, c'est elle qui décide.

- **Le panneau « Chiffrement de la base » change de forme** une fois le chiffrement en place : il n'invite plus à faire ce qui est fait, il propose de changer la phrase de récupération ou de revenir en arrière. Il n'est pas retiré pour autant — les installations antérieures à l'assistant n'ont jamais vu la question, et quelqu'un qui a décliné doit pouvoir changer d'avis.

### Corrigé

- **Une clé configurée sans fichier de base créait la base EN CLAIR.** L'ouverture se fiait à l'en-tête du fichier, qui ne répond rien quand il n'y a pas de fichier. C'est exactement le chemin de l'assistant — la clé est posée avant que le serveur démarre — et c'est aussi ce qui se produit si la base disparaît sur une installation chiffrée. Trouvé en écrivant le test avant le code de l'assistant.

---


### Sécurité

- **Toute route réservée aux administrateurs répondait à n'importe quel utilisateur connecté.** `RequireAdmin` appelait `RequireAuth` comme une fonction ordinaire — or celle-ci se termine par `c.Next()`, qui exécute la suite de la chaîne, handler compris. Le contrôle du privilège n'intervenait donc qu'**après** que le handler avait répondu : le 403 arrivait sur une réponse déjà partie en 200, et finissait collé derrière la charge utile. Sauvegardes, restauration, contrôle d'intégrité, données personnelles : tout était lisible et actionnable par un compte non administrateur. Trouvé en appelant `GET /api/v1/backups` sur un serveur qui tourne, avec un jeton non-administrateur, et en lisant les octets renvoyés.

- **Les sauvegardes automatiques ne sont plus écrites en clair par défaut.** Elles ne l'étaient que si la variable d'environnement `BACKUP_PASSPHRASE` existait — c'est-à-dire jamais. Mesuré sur une installation réelle : jusqu'à quatorze copies complètes de la comptabilité, en-tête SQLite, numéro de TVA, adresses e-mail et IBAN lisibles sans aucune clé, dans le dossier que l'on copie précisément sur un NAS ou une clé USB. Une phrase de passe se règle maintenant dans **Paramètres → Sauvegardes**, LedgerAlps la retient (scellée au compte Windows par DPAPI), et elle s'applique à toutes les copies. Les copies déjà présentes peuvent être chiffrées en une fois — chacune est relue et vérifiée avant que la version en clair disparaisse.

- **La sauvegarde manuelle ignorait cette phrase de passe.** Le chemin automatique consultait la politique, le bouton « Créer une sauvegarde » non : sur une installation dont les sauvegardes automatiques étaient chiffrées, il produisait quand même un fichier lisible — et c'est le chemin qu'on emprunte juste avant de copier le fichier sur une clé USB. Une copie en clair reste possible, mais elle se demande désormais explicitement.

### Ajouté

- **Chiffrement de la base de données**, en option, désactivé par défaut (Paramètres → Maintenance → Sécurité). Le fichier `ledgeralps.db` est chiffré par blocs de 4 Kio, journal WAL compris.

  Cela était porté comme impossible, et le diagnostic était faux : le blocage n'était pas SQLCipher mais le pilote SQLite. Le paquet `vfs` de `modernc.org/sqlite` est en lecture seule, donc rien ne pouvait s'insérer dessous ; `github.com/ncruces/go-sqlite3` expose un VFS écrivable et livre `vfs/adiantum`. Aucune dépendance C n'a été ajoutée, `CGO_ENABLED=0` et la compilation croisée sont conservés.

  Ce que cela ajoute au chiffrement du disque : une seule chose, mais elle est réelle — la protection **suit le fichier**. Une base copiée sur un NAS ou dans un dossier synchronisé reste illisible, ce qu'un disque chiffré ne fait pas. Ce que cela n'ajoute pas : rien contre un programme lancé sous le même compte Windows, qui peut demander la clé comme LedgerAlps le fait. BitLocker reste donc le premier conseil.

- **Mode récupération.** Si la base est chiffrée et que sa clé ne se descelle pas sur ce compte — nouveau PC, Windows réinstallé, profil recréé — LedgerAlps démarre sur une page unique qui demande la phrase de récupération, la desselle, la rescelle et relance l'application. Sans ce mode, le logiciel s'arrêtait avec un message dans un journal que personne ne lit et le point d'entrée de récupération devenait injoignable : la mesure censée protéger dix ans de pièces (CO art. 958f) créait la panne qu'elle devait empêcher. Constaté en effaçant le coffre à secrets sur un serveur réel.

- **Coffre à secrets scellé au compte** (`internal/core/secretstore`). DPAPI sous Windows — aucun droit administrateur, aucune invite. Ailleurs, un fichier `0600`, et l'interface dit que les droits du fichier sont toute la protection plutôt que de laisser croire à un scellement absent.

### Modifié

- **Le conseil « activez BitLocker » est devenu suivable.** Il l'était mal : sous Windows Famille, ce panneau n'existe pas — la fonctionnalité s'appelle « Chiffrement de l'appareil » et se trouve ailleurs. LedgerAlps lit maintenant l'édition de Windows, nomme la bonne fonctionnalité, affiche la marche à suivre et ouvre le bon réglage. Un conseil qu'on ne peut pas suivre vaut à peine mieux que pas de conseil, et il abîme la confiance dans les autres.

- **L'avis nLPD art. 8 affirmait que la base « restera en clair ».** Il le disait encore une fois la fonctionnalité livrée. Le garde-fou du dépôt a fait son travail — basculer la capacité `encrypted_database` a fait échouer la compilation en nommant l'avis. Il parle désormais de l'état constaté du disque de la machine, et non d'une limite du produit. Un troisième ancrage a été reconnu pour les avis ouverts : une **condition** évaluée à chaque affichage, qui est une garantie de fraîcheur plus forte qu'une capacité, pas plus faible.

- **Pilote SQLite** : `github.com/ncruces/go-sqlite3` remplace `modernc.org/sqlite`. Mesuré sur la charge réelle de l'application (2 000 requêtes typiques) : 1,00 ms par requête contre 0,64 ms, soit +0,36 ms ; mémoire du processus 17 → 81 Mo ; binaire +2,4 Mo. La suite de tests complète passe sans qu'une seule requête SQL ait été modifiée.

### Corrigé

- **`VACUUM INTO 'chemin'` échoue depuis une connexion chiffrée** — la cible hérite du VFS de la connexion, sans clé. C'est la ligne qui produit toutes les sauvegardes : sans le voir, la migration les aurait cassées en silence. La cible nomme désormais le VFS par défaut explicitement, ce qui garantit aussi que la sauvegarde sort **en clair** avant d'être rechiffrée avec la phrase de passe de l'utilisateur — une sauvegarde chiffrée avec la clé de la machine deviendrait illisible le jour où cette machine n'est plus là.

- **Une restauration ramenait une installation chiffrée en clair, sans un mot.** Une restauration écrit un instantané en clair par-dessus la base ; l'interface aurait continué d'afficher « chiffrée ». L'état est désormais réconcilié à chaque démarrage entre le fichier, la clé et la demande en attente.

---

## [1.4.7] — 2026-08-04

### Corrigé
- **Le nom des sauvegardes était écrit en heure UTC** alors que l'interface affiche l'heure du fichier, que le système rend en heure locale. Une sauvegarde prise à 16 h 23 en Suisse s'appelait `…T14-23-05` et s'affichait « 16:23 » : deux heures d'écart entre l'explorateur de fichiers et le logiciel, sur des fichiers qu'il faut savoir identifier pendant dix ans (CO art. 958f). Le nom porte désormais l'heure locale suivie du décalage — `…T20-10-51+0200` — le décalage levant l'ambiguïté de la nuit du passage à l'heure d'hiver, où 02 h 30 existe deux fois.
- **L'ordre des sauvegardes reposait sur l'ordre alphabétique de leur nom.** En heure locale il se casse cette même nuit-là, et comme c'est cet ordre qui désigne la plus ancienne à purger, la purge aurait effacé la mauvaise copie. Il se lit maintenant sur la date du fichier, qui est un instant. Les sauvegardes nommées à l'ancien format restent listées et datées correctement.

### Documentation
- **Roadmap refondue** : sommaire scannable, priorités en une ligne chacune, décisions détaillées en dessous plutôt que dans des cellules de tableau — l'une atteignait 4 677 caractères. Trois diagrammes, dont le critère d'admission d'une fonctionnalité, qui dit ce qu'aucun tableau ne dit.
- **Doublons retirés** : le point « Suppression & droit à l'effacement » décrivait un travail déjà livré dans Maintenance & Système ; l'ancien tableau répétait la conformité nLPD à trois endroits.
- **Modules métier inscrits** comme piste retenue mais non engagée, avec les trois règles qui décident si un module reste un module — la troisième, « aucun champ dans le noyau », étant celle qui sera difficile à tenir.
- **README refondu** : diagramme du trajet des données, capacités en grille, questions fréquentes repliables. Ajout de la réponse sur le non-assujetti à la TVA et sur ce que l'anonymisation ne fait pas.
- Le périmètre du produit et l'horodatage des sauvegardes sont documentés dans `docs/ARCHITECTURE.md` et `docs/PRODUCTION.md`.

---

---

## [1.4.6] — 2026-08-04

### Sécurité
- **Le plafond des notes de crédit se contournait en modifiant la note après coup.** Il était vérifié à la création et nulle part ailleurs : une note de 500.- sur une facture de 500.- pouvait passer à 1500.- en HTTP 200, sous-évaluant d'autant la TVA due. Mesuré sur un serveur réel avant correction. La modification est désormais bornée elle aussi — réduire une note reste possible, un crédit partiel étant légitime.
- **Modifier un document changeait sa nature.** Le champ `document_type` omis dans une modification retombait sur « facture » : éditer une note de crédit la transformait en facture, le lien vers la pièce corrigée survivant dans la base pendant que le document changeait de sens — ce que la LTVA art. 27 al. 4 ne prévoit pas. Un champ absent veut désormais dire « inchangé », et un changement explicite est refusé.
- **Facturer de la TVA sans numéro de TVA est refusé à la source.** Le contrôle de cohérence le signalait après coup ; il fallait alors corriger des factures déjà envoyées. La LTVA art. 27 al. 1 interdit à qui n'est pas assujetti de faire figurer l'impôt, et l'al. 2 l'en rend redevable même sans l'avoir encaissé. Une installation neuve, sans fiche société encore remplie, n'est pas bloquée : refuser sa première facture ressemblerait à une panne.
- Ces refus répondent **409** avec un motif exploitable, et non 500 : la demande est bien formée, elle se heurte à une règle. L'écran affiche une phrase qui dit ce qui est refusé avant de donner les montants.
- **L'empreinte d'intégrité des écritures ne couvrait pas ce qui était enregistré.** L'empreinte SHA-256 était calculée sur un `after_state` rédigé séparément de celui inséré, et sur un horodatage venu de Go alors que la colonne était remplie par le `DEFAULT CURRENT_TIMESTAMP` de SQLite. **Aucune écriture comptabilisée ne pouvait se revérifier**, et la garantie affichée au titre du CO art. 957a n'en était pas une. L'empreinte couvre désormais exactement les valeurs stockées, `created_at` compris, écrit explicitement.
- **Les entrées antérieures ne sont pas rattrapables** : les valeurs du calcul n'ont jamais été persistées. Marquées `hash_version = 1`, elles sont signalées **non vérifiables**, jamais altérées. Leur chaînage reste vérifié — une suppression y demeure détectable.
- **Verrouillage de période** (CO art. 958f, Olico art. 3). Un exercice clos refuse la création **et** la comptabilisation d'écritures, antidatage compris. Le second contrôle porte le cas qui compte : un brouillon créé avant la clôture, comptabilisé après. La correction se passe dans l'exercice ouvert.
- **Régénération de la clé de signature** (Paramètres → Maintenance → Sécurité, administrateurs). À faire si le fichier de configuration a pu être vu : qui le détient forge un jeton valide pour n'importe quel compte **sans connaître aucun mot de passe**, et rien ne permettait d'y répondre autrement qu'en éditant le fichier à la main. La confirmation énonce la portée exacte — cela déconnecte les sessions, et rien d'autre.
- **La lecture du maillon précédent de la chaîne se fait désormais dans la transaction.** À l'extérieur, deux comptabilisations concurrentes obtenaient le même prédécesseur et fourchaient la chaîne, que la vérification aurait signalée comme rompue sans que personne ait rien falsifié.

### Corrigé
- **La colonne « TVA% » s'affichait sur toutes les factures**, y compris à 0 %. Le bloc des totaux avait été corrigé, pas le tableau des lignes. « 0.0 % » est une mention de l'impôt — elle affirme un taux — et une entreprise non assujettie n'a pas le droit de la faire figurer (LTVA art. 27 al. 1). La colonne disparaît quand aucune ligne ne porte de TVA, et sa largeur revient à la description.
- **Une facture longue débordait sur le bulletin de versement.** La pagination automatique était désactivée et rien ne la remplaçait : au-delà d'une vingtaine de lignes, le texte s'imprimait par-dessus la zone QR, rendant la facture illisible *et* le bulletin inutilisable par la banque, qui lit une zone à position fixe.
- **Une facture sans TVA était enregistrée à 8,1 %.** Le taux porté par l'en-tête valait 8,1 par défaut et n'était remplacé que si la première ligne portait un taux *strictement positif* — ce qui confondait « 0 % » et « non renseigné ». Ce n'est pas un défaut d'affichage : la déclaration TVA agrège les factures **en groupant par ce taux**, si bien que le chiffre d'affaires d'une entreprise non assujettie remontait comme taxable à 8,1 % avec un impôt nul. Le taux suit désormais les lignes.
- **La ligne « TVA » s'imprimait sur le PDF même à 0 %.** Une entreprise non inscrite au registre des assujettis n'a pas le droit de faire figurer l'impôt sur ses factures (LTVA art. 27 al. 1), et celle qui le fait quand même en devient **redevable** (al. 2), qu'elle l'ait encaissé ou non. La ligne n'apparaît plus que s'il y a de la TVA.
- **Le numéro IDE n'était jamais lu depuis les réglages** : il n'apparaissait donc sur aucune facture, alors qu'il identifie l'entreprise au registre, assujettie ou non.
- **Le sélecteur d'anonymisation ne chargeait aucun contact.** `GET /contacts` renvoie un tableau JSON nu, pas un objet paginé ; l'écran lisait `data.items`, obtenait `undefined`, et affichait une liste vide. Tous les autres écrans lisaient déjà la réponse correctement.
- **Le filtre Clients / Fournisseurs de la page Contacts ne filtrait rien.** L'interface envoyait `contact_type`, le serveur l'ignorait : cliquer sur l'un ou l'autre renvoyait la liste entière. Un filtre qui ne filtre pas est pire qu'un filtre absent — on croit avoir restreint la vue, et on lit la mauvaise. Un contact « client et fournisseur » apparaît désormais dans les deux filtres, sans quoi un partenaire chez qui on achète et à qui on vend disparaîtrait des deux vues.
- **La validation d'IBAN se limitait à la clé de contrôle MOD-97.** Elle attrape la faute de frappe la plus courante mais laisse passer deux erreurs qu'une banque rejette : un IBAN de **mauvaise longueur pour son pays** — un CH à 20 caractères a environ une chance sur 97 de passer le seul MOD-97 — et une **structure invalide**, le code pays devant être deux lettres et la clé deux chiffres. Le registre officiel des longueurs (ISO 13616) et le contrôle de structure sont désormais appliqués, à la création comme à la modification d'un client ou d'un fournisseur. Découvrir le problème à la remise du fichier de virements, c'est le découvrir après avoir cru les paiements partis.
- Un code pays absent du registre embarqué n'est **pas** rejeté : le registre évolue après la compilation du binaire, et empêcher quelqu'un de facturer coûterait plus qu'accepter un IBAN qu'on ne peut pas entièrement vérifier. La table sert à attraper les erreurs, pas à tenir une liste blanche.
- Un IBAN laissé vide dans un formulaire n'est plus traité comme un IBAN invalide : c'est l'absence d'IBAN, le cas courant d'un client. Les espaces de saisie sont retirés avant enregistrement, de sorte que le même compte saisi par groupes de quatre ou d'un seul tenant donne la même valeur.
- **Une facture ne conservait pas l'identité de son destinataire.** Elle ne stockait que `contact_id`, et le PDF — régénéré à la demande — relisait le contact **vivant** : renommer un client, corriger son adresse ou le voir déménager réécrivait rétroactivement toutes ses factures passées. Réimprimer une facture de 2024 la montrait avec les coordonnées d'aujourd'hui. Une pièce comptable dont le contenu change n'est plus celle qui a été envoyée : le CO art. 958f impose de la conserver telle quelle, et la LTVA art. 26 exige qu'elle nomme son destinataire. L'identité est désormais figée à l'émission. Les factures antérieures sont complétées au démarrage depuis leur contact et **marquées comme reconstituées** — une reconstitution et une pièce d'origine ne se valent pas devant un réviseur.
- **La rétention des adresses IP était annoncée mais jamais appliquée.** La table des événements de sécurité porte depuis sa création un commentaire disant qu'elle est à durée limitée ; rien ne purgeait quoi que ce soit. Une adresse IP est une donnée personnelle et la nLPD art. 6 al. 4 impose de la détruire ou de l'anonymiser dès qu'elle n'est plus nécessaire. Un commentaire décrivant une garantie inexistante est pire que l'absence de garantie : il empêche de voir le manque. Les adresses sont désormais anonymisées après 90 jours et l'événement supprimé après un an, à chaque démarrage.
- **La clôture d'exercice ne faisait rien, et répondait « close ».** `fiscal_year_id` n'était renseigné nulle part — ni sur les écritures, ni sur les factures. La clôture, qui sélectionne les soldes à virer en filtrant sur ce champ, ne voyait donc aucune écriture : elle marquait l'exercice clos **sans produire la moindre écriture de clôture**. Mesuré sur un serveur réel : 10 000.- de produits jamais virés au résultat. Le contrôle des brouillons, filtré pareil, laissait passer un exercice contenant des écritures non comptabilisées.
- **L'écriture de clôture échappait à la chaîne d'intégrité.** Insérée directement en `posted`, sans empreinte ni maillon d'audit, la pièce qui vire le résultat de l'exercice était la seule hors de la chaîne du CO art. 957a. Le contrôle de cohérence la signalait déjà comme « écriture postée sans empreinte », sans que le lien soit fait.
- **Il n'existait aucune façon de déclarer un exercice comptable.** L'installation n'en sème aucun et seule la clôture en créait un — le suivant. Un exercice décalé (juillet–juin, fréquent en Suisse) était donc impossible à poser. `POST /fiscal-years` comble ce manque, et sans exercice couvrant une date, LedgerAlps crée l'année civile plutôt que de refuser l'écriture.
- Les bases existantes sont rattrapées au démarrage : les écritures et documents orphelins sont rattachés à leur exercice, celui-ci étant créé si nécessaire. Le rattrapage est idempotent et ne pose jamais une année civile par-dessus un exercice décalé déjà déclaré.

### Ajouté
- **Coordonnées bancaires pour un paiement par virement** — nom de la banque, adresse et BIC/SWIFT, dans Paramètres → Banque. L'IBAN suffit à la QR-facture ; un virement depuis l'étranger demande le nom de la banque et le BIC, et un client qui saisit le virement à la main veut vérifier où part l'argent. Facultatif : le bloc n'apparaît sur la facture que si quelque chose a été renseigné, un intitulé suivi de rien ferait douter du reste.
- **Factures sur plusieurs pages.** Les lignes se répartissent sur autant de pages que nécessaire, l'en-tête du tableau est répété, et une ligne n'est jamais coupée en deux entre deux pages — une ligne de facture scindée est illisible et se prête aux contestations. **Le bulletin de versement reste en bas de la dernière page** ; s'il n'y a plus la place, une page lui est ajoutée.
- **Numérotation « Page n/N »** dès qu'il y a plus d'une page. Sur une pièce comptable de plusieurs feuillets, c'est ce qui permet de constater qu'il n'en manque pas — et le CO art. 958f impose de la conserver complète dix ans. Une facture d'une seule page n'en porte pas : le dire n'apprendrait rien.
- **Les descriptions longues s'enroulent** au lieu de déborder sur la colonne voisine, la hauteur de ligne suivant le texte. C'est ce qui permet de justifier une facturation en détail sans écraser le montant d'à côté.
- **Téléphone et courriel de l'entreprise sur la facture**, saisis dans Paramètres → Identité. La LTVA art. 26 ne les exige pas ; une facture qu'on ne peut pas contester facilement se paie tard, ou pas.
- **Les constats du contrôle de cohérence nomment les documents concernés**, avec un lien vers chacun. « 1 facture porte des notes de crédit qui dépassent son montant » n'aide personne sans le numéro : il restait à la chercher à la main dans toute la liste, et c'est ce qui fait qu'on ne corrige pas. Au-delà de vingt pièces, seul le compte est affiché — à ce stade c'est lui qui informe, pas l'énumération.
- **Chiffre d'affaires par année, par mois ou par client**, sur le tableau de bord et sur la période de votre choix. La courbe six mois montrait une tendance ; ce tableau donne les montants, avec une barre proportionnelle pour repérer d'un coup d'œil ce qui pèse. La **convention de calcul est affichée sous le tableau** — brouillons et annulées exclus, offres exclues, notes de crédit déduites, la même que la déclaration TVA. Un total sans sa définition invite à le comparer à un autre calculé autrement, et c'est ainsi qu'on croit à un écart qui n'existe pas.
- **Documents filtrables et téléchargeables depuis la fiche client.** Filtres par type, statut et période, sélection ligne à ligne ou globale, et téléchargement : **un PDF si un seul document est sélectionné, une archive ZIP si plusieurs**. Emballer un unique PDF obligerait à le dézipper pour le lire.
- Les filtres portent côté serveur, comme la pagination : un filtre appliqué dans le navigateur ne verrait que la page chargée et donnerait un résultat différent selon l'endroit où l'on se trouve.
- Un document disparu entre l'affichage de la liste et le clic est omis de l'archive **et signalé**. Une archive plus courte que la sélection, sans un mot, se remarque trop tard — souvent une fois transmise.
- **IDE et numéro de TVA sur la facture.** Ce sont deux choses distinctes : l'IDE identifie l'entreprise au registre, le numéro de TVA n'existe que pour un assujetti et la LTVA art. 26 al. 2 let. a l'exige sur toute facture portant de la TVA. Quand le second contient déjà le premier, une seule ligne les présente — les répéter donnerait deux fois le même numéro à un lecteur qui y chercherait une différence.
- **Contrôle de cohérence : « TVA facturée sans numéro de TVA ».** Le piège le plus coûteux pour un indépendant qui démarre, LedgerAlps appliquant 8,1 % par défaut. Le contrôle signale sans corriger : il ne peut pas savoir si vous êtes assujetti et avez oublié votre numéro, ou si vous ne l'êtes pas et facturez à tort — les deux se règlent différemment, et deviner à votre place serait pire.
- **La phrase de passe de sauvegarde peut être affichée en clair** pendant la saisie. Une phrase qu'on ne relit pas se saisit de travers, et l'erreur ne se découvre qu'à la restauration — au pire moment. Elle repart masquée à chaque ouverture du dialogue.
- **L'exemple de phrase de passe n'est plus copiable** — ni sélection, ni copier-coller, ni glisser-déposer. Il est publié dans le code source et la documentation : le copier en ferait la phrase la plus répandue du produit, donc la première qu'un attaquant essaierait. Le lire et s'en inspirer reste le but.
- **L'anonymisation dit ce qu'elle ne fait pas** : les sauvegardes déjà prises conservent les coordonnées effacées. Une sauvegarde est une copie figée, et la réécrire lui retirerait la valeur qu'elle a pour vos livres (CO art. 958f). L'écran énonce donc la condition qui vous revient — ne pas restaurer une copie antérieure pour retrouver ces données.
- **Confirmation avant toute action qui ne se défait pas.** Marquer une facture envoyée ou payée, l'annuler, l'archiver, émettre une note de crédit, convertir une offre en facture, la marquer refusée ou expirée : chacune demande confirmation, et chacune **énonce ses conséquences propres** plutôt qu'un « êtes-vous sûr ? ». Une question générique n'apprend rien et, répétée, devient un réflexe : le clic précède la lecture. « Marquer payée » dit donc que le paiement passe au journal et que la facture ne sera plus modifiable ; « note de crédit » dit que la facture reste conservée, parce qu'une note de crédit corrige et n'efface pas.
- Le raccourci « Payer » de la liste des factures est confirmé lui aussi, en rappelant le montant et le client. C'est le bouton le plus exposé de l'application : aligné avec ceux des autres lignes, un clic décalé encaissait la mauvaise facture.
- Le focus s'ouvre sur « Annuler », jamais sur la confirmation : une frappe sur Entrée juste après l'ouverture ne doit pas valider l'action.

### Modifié
- **Le nom de l'émetteur est plus discret sur la facture** (11,5 points au lieu de 14) : il concurrençait le titre du document et débordait sur une raison sociale longue.
- **L'onglet Maintenance est réorganisé en cinq sections**, découpées par la question à laquelle chacune répond plutôt que par le module qui l'implémente : Diagnostic (« est-ce que quelque chose ne va pas ? »), Conformité (« puis-je le prouver ? »), Piste d'audit (« mes livres ont-ils été modifiés ? »), Données personnelles (« que sait LedgerAlps sur mes clients, et pour combien de temps ? »), Sécurité & réseau (« qui peut atteindre cette installation ? »). Tout tenait auparavant dans un seul défilement, où atteindre les réglages réseau supposait de traverser la conformité. Le nombre d'anomalies détectées s'affiche sur l'entrée Diagnostic : un contrôle qu'on n'ouvre pas ne sert à rien.
- **Anonymisation d'un contact** (nLPD art. 6 al. 4 et art. 32), depuis Paramètres → Maintenance → Données personnelles. Efface nom, adresse, courriel, téléphone, IBAN, numéro de TVA et notes ; **conserve les documents comptables**, qui portent l'identité de leur destinataire telle qu'elle était à l'émission. La confirmation dit que l'opération est irréversible — c'est ce qui a été promis à la personne concernée — et la réponse énumère ce qui a été effacé et ce qui a été gardé.
- L'écran affiche les **chiffres réels** de la rétention plutôt que la règle : combien d'événements sont conservés, combien portent encore une adresse IP, combien de contacts ont été anonymisés. Annoncer une durée sans montrer qu'elle s'applique est exactement ce qui vient d'être corrigé.
- **Paramètres → Maintenance → Piste d'audit** — la chaîne d'empreintes du CO art. 957a devient visible, avec **vérification de la chaîne entière**. Vérifier une entrée isolée ne détecte que la modification de son contenu : **supprimer une ligne laisse l'empreinte de toutes les autres parfaitement valide**. Le parcours contrôle en plus le chaînage et la continuité des numéros, et nomme la rupture — contenu modifié, chaînage rompu, entrée supprimée, début de chaîne manquant. L'écran énonce sa limite : une troncature **en fin** de chaîne reste cohérente, seule la sauvegarde y répond.
- **Paramètres → Maintenance → Conformité & clôture** — exercices comptables, déclaration d'un exercice décalé, et clôture dont la confirmation énonce les conséquences plutôt qu'un avertissement vague.
- **Attestation d'intégrité (Olico art. 9)**, téléchargeable et destinée à un tiers — fiduciaire, réviseur, AFC. État de la chaîne, empreinte de tête, périmètre couvert, et **ses limites** : l'horodatage vient de l'horloge du poste, pas d'une autorité tierce, ce qui établit l'ordre des enregistrements mais pas une date opposable au sens d'un horodatage qualifié (RFC 3161). Une attestation qui promettrait davantage serait détruite au premier examen sérieux.
- **Export de réversibilité** — l'archive légale contient désormais un dossier `csv/` en plus du JSON : point-virgule et BOM UTF-8 pour qu'Excel affiche « Genève » correctement en Suisse, lignes d'écritures et de factures extraites dans leurs propres fichiers avec clé étrangère, et un LISEZ-MOI documentant les relations. Le verrouillage fournisseur ne tient pas au refus d'exporter, il tient au format de l'export.
- **Contrôles QR-facture dans le contrôle de cohérence** — la validation SIX IG v2.4 existait sans écran pour l'exercer. IBAN de la société absent ou invalide, adresse structurée incomplète, IBAN de contacts invalides : autant de bulletins que la banque du client aurait refusés, signalés avant l'envoi. Silencieux quand il n'y a rien à dire.

---

## [1.4.5] — 2026-08-03

### Sécurité
- **Le serveur n'écoute plus sur toutes les interfaces par défaut.** Il écoutait sur `0.0.0.0` : un portable sur un réseau public ou dans une salle d'attente servait sa comptabilité à ce réseau, en clair, sans que personne l'ait demandé. Le défaut est désormais `127.0.0.1` — joignable depuis cette machine seulement
- **HTTPS natif dès que LedgerAlps est joignable depuis le réseau.** Définir `HOST` sur autre chose que loopback active TLS : votre certificat via `TLS_CERT`/`TLS_KEY`, ou un **certificat auto-signé généré** dans `<données applicatives>/tls`. Il couvre `localhost`, le nom de la machine et ses adresses IP, vaut dix ans, et est **réutilisé d'un démarrage à l'autre** pour que l'exception accordée au navigateur tienne — la régénérer à chaque fois apprendrait à cliquer sans lire
- `ALLOW_INSECURE_HTTP=true` conserve le clair pour le seul cas légitime — un reverse proxy terminant TLS sur la même machine — et l'écrit au journal, parce que c'est aussi le drapeau vers lequel on se tourne pour faire taire un avertissement
- Sans cela, mot de passe de connexion, jeton de session et **phrase de passe de chiffrement des sauvegardes** traversaient le réseau en clair (LPD art. 8, OPDo art. 3 al. 1 let. c)

> **Changement de comportement.** Si vous accédiez à LedgerAlps depuis un autre poste sans reverse proxy, il faut désormais définir `HOST` explicitement — et l'accès passera en HTTPS. C'est délibéré : cette configuration servait des identifiants en clair. Sur un poste unique, rien ne change.

### Ajouté
- **Paramètres → Maintenance** — première tranche du point Maintenance & Système de la roadmap. **Contrôle de cohérence** : équilibre débit/crédit, écritures et documents vides, empreintes d'intégrité manquantes, totaux ne correspondant pas aux lignes, contacts orphelins, factures créditées au-delà de leur montant, offres sans réponse depuis 90 jours. Il **ne répare rien** — une comptabilité incohérente se corrige par une écriture, pas par un bouton ; réparer en silence effacerait la trace (CO art. 957a al. 2 ch. 5). Chaque constat dit quoi faire. **État du système** : moteur, version, volumétrie, sauvegardes et combien sont chiffrées, exposition réseau, chiffrement du disque, et les capacités déclarées du produit
- **Paramètres → Maintenance → Réseau & chiffrement** — interface d'écoute, certificat et clé, réglables sans éditer de JSON dans `%APPDATA%`. Les réglages sont écrits dans `config.json` **en préservant les clés inconnues** (sérialiser une structure supprimerait `jwt_secret` et déconnecterait tout le monde), via une écriture puis un renommage — une coupure laisserait sinon un fichier tronqué et l'application ne redémarrerait plus. Certificat et clé sont exigés ensemble et leurs chemins vérifiés avant enregistrement : une demi-configuration TLS empêcherait le démarrage. Un bouton « Redémarrer maintenant » applique le changement
- **Contrôle mécanique de cohérence des avis de conformité.** Un avis périmé coûte plus qu'un avis absent : l'utilisateur agit dessus, puis cesse de croire le suivant. Chaque avis déclare désormais les capacités qu'il suppose **absentes** du produit ; un test échoue dès qu'une de ces capacités existe, en nommant l'avis. Livrer une fonctionnalité en oubliant la bannière n'est plus possible — la construction s'arrête avant. Le contrôle a trouvé un second avis non protégé dès sa première exécution. Voir [`compliance/README.md`](compliance/README.md)
- **L'avertissement sur le chiffrement du disque ne s'affiche plus que sur les machines qui en ont besoin.** LedgerAlps lit l'état BitLocker au démarrage : si le disque est protégé, la bannière disparaît d'elle-même. Importuner quelqu'un qui a déjà fait le travail est précisément ce qui apprend à ignorer l'avertissement suivant — et le suivant peut compter. L'état figure aussi sur **Paramètres → Maintenance → État du système**

> **Limite assumée sur la détection BitLocker.** La faire correctement demande des droits d'administrateur : `Get-BitLockerVolume`, la classe WMI `Win32_EncryptableVolume` et `manage-bde` répondent tous « accès refusé » à un utilisateur normal — mesuré sur une vraie machine, pas supposé. LedgerAlps lit donc une clé de registre lisible sans élévation, qui ne couvre que le volume système. Un résultat **positif** fait taire l'avis ; tout autre résultat le laisse visible, formulé comme « à vérifier » et jamais comme « votre disque n'est pas chiffré ». Accuser à tort est le défaut que ce mécanisme existe pour éviter, donc le seul qu'il refuse de risquer.

### Corrigé
- **Les options `HOST`, `TLS_CERT`, `TLS_KEY` et `ALLOW_INSECURE_HTTP` étaient inatteignables sur une installation Windows.** `config.json` n'est écrit qu'au premier lancement et jamais retouché : une installation mise à jour ne contient donc que les clés de sa version d'origine, et il primait sur les variables d'environnement — le serveur lisait des clés absentes et retombait sur les valeurs par défaut. **Les variables d'environnement priment désormais sur le fichier** lorsqu'elles sont réellement définies ; une variable vide ou absente n'écrase rien. C'est la même précédence qui avait fait viser la base live lors d'une restauration de test
- **Le bouton « Redémarrer maintenant » tournait indéfiniment** après un changement de réglage réseau. La page sondait `/health` en **relatif** : elle est sur `http://localhost:8000` et le serveur pouvait repartir en **https sur le même port**, si bien que la requête en clair se heurtait à une poignée de main TLS et échouait jusqu'au délai d'attente. Sonder la nouvelle adresse n'aurait pas aidé non plus — avec un certificat auto-signé, le navigateur refuse toute requête tant qu'il n'a pas affiché son avertissement, qu'il ne montre que pour une navigation. L'interface attend donc l'**arrêt** du serveur actuel — seul signal fiable que le redémarrage a commencé — puis **navigue** vers la bonne adresse, en `http` ou `https` selon le réglage enregistré
- **Des textes s'affichaient sans couleur dans toute l'interface.** Les palettes `danger`, `warning` et `success` ne définissent que les nuances 100, 500 et 700 ; les classes en `-600`, `-50`, `-200` ou `-800` n'émettaient donc aucun CSS. Onze fichiers étaient concernés — messages d'erreur de formulaire, montants négatifs du plan comptable, avertissements TVA, boutons d'annulation
- **L'avis de conformité nLPD art. 8 était périmé** : il annonçait encore « base et sauvegardes en clair » alors que les sauvegardes sont chiffrables depuis la v1.4.4. Réécrit pour dire ce qui est vrai — les sauvegardes peuvent l'être, la base ne le sera pas (SQLCipher est incompatible avec le binaire unique), et l'action qui vous revient est d'activer le chiffrement du disque. Un avis faux est pire qu'un avis absent : il est cru

### Modifié
- **Les paquets `.deb` et `.rpm` ne sont plus publiés.** L'empaquetage suppose de suivre les conventions de plusieurs distributions, et personne ne le vérifiait sur une vraie machine — la même raison qui avait fait abandonner macOS et ARM, à plus petite échelle. L'archive `.tar.gz` reste publiée et `scripts/install.sh` l'installe. **Linux reste une plateforme de test** : la CI tourne sur Ubuntu, c'est là que `go test -race` s'exécute et que les assertions de permissions de fichiers ont un sens
- **Une option « Chiffrer aussi l'accès local » a été proposée en pré-version puis retirée.** Elle n'apportait aucune sécurité réelle — le trafic vers `127.0.0.1` ne quitte pas la machine — et coûtait un avertissement de certificat à chaque nouveau profil de navigateur. Dépenser la confiance des utilisateurs dans les avertissements sans rien protéger est un mauvais échange. Le chiffrement suit désormais l'exposition : local en clair, réseau en HTTPS. La clé `force_tls` est supprimée des fichiers de configuration qui la portaient
- Si votre politique exige TLS jusqu'au poste, la réponse est le chiffrement du **disque** (BitLocker, LUKS) : il protège la base de données elle-même, la vraie cible sur une machine accessible

### Documentation
- **Pourquoi la base de données est en clair**, et ce qui protège réellement : les quatre options possibles avec leur coût, et le point commun qui les condamne toutes sauf une — le serveur devant lire la base sans intervention humaine, la clé vit forcément sur la machine qu'elle protège. La réponse est le chiffrement du disque, qui couvre aussi le secret de signature. README pour l'essentiel, `docs/PRODUCTION.md` pour le détail
- **Rotation du secret de signature** inscrite à la roadmap, avec sa portée exacte vérifiée dans le code : le secret ne sert qu'à signer les jetons, le régénérer déconnecte les sessions et **rien d'autre** — mots de passe intacts, aucune donnée touchée, sauvegardes toujours utilisables

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

[Unreleased]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.6...HEAD
[1.4.6]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.5...v1.4.6
[1.4.5]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.4...v1.4.5
[1.4.4]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.3...v1.4.4
[1.4.3]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.2...v1.4.3
[1.4.2]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.1...v1.4.2
[1.4.1]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/kmdn-ch/LedgerAlps/compare/v1.3.15...v1.4.0
[1.0.0]: https://github.com/kmdn-ch/LedgerAlps/compare/v0.1.0...v1.0.0
[0.1.0]: https://github.com/kmdn-ch/LedgerAlps/releases/tag/v0.1.0
