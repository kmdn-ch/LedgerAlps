# Changelog

Toutes les modifications notables de LedgerAlps sont documentées ici.  
Format : [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/) — Versioning : [SemVer](https://semver.org/lang/fr/).

---

## [Unreleased]

## [1.5.6] — 2026-08-27

### Modifié

- **L'import camt.053 n'a plus qu'un seul point d'entrée.** Il vivait à deux endroits — Rapports et Paramètres → Banque — pour la même route et le même effet : les deux ÉCRIVAIENT en base. Mais celui de Rapports présentait une liste de lecture, « voilà ce que j'ai lu », sans dire que les écritures étaient persistées au passage : on croyait consulter un fichier, on alimentait la file de rapprochement. Deux boutons pour un même geste, dont l'un cache ce qu'il fait, ne sont pas une commodité. L'import vit désormais là où se fait le travail qui le suit, et Rapports porte un renvoi vers cet écran.

- **La régénération de la clé de signature tient dans une seule carte, et sa périodicité n'est plus réglable.** Deux encadrés parlaient de la même clé sur deux écrans, le premier renvoyant au second par « voir plus bas » : qui lisait le premier croyait devoir cliquer, alors que la machine le fait déjà chaque nuit. La périodicité, elle, offrait quatre valeurs — jamais / jour / semaine / mois — dont trois n'existaient que pour affaiblir la quatrième, « jamais » rendant à l'identique la situation que la rotation automatique venait corriger. Elle est désormais **constante, quotidienne**, l'écran l'énonce en toutes lettres, et il ne reste qu'une commande : régénérer tout de suite, pour la fuite dont on vient de s'apercevoir.

  Le réglage part avec son code : `rotation_days` quitte la route, `SetJWTSecretMaxAge` et le champ de configuration disparaissent, et un `jwt_secret_max_age_days` resté dans un `config.json` est **ignoré puis effacé** à la première rotation — le laisser ferait croire à qui ouvre le fichier que la valeur qu'il y lit s'applique encore. Une installation où la rotation avait été coupée tourne donc de nouveau, ce qui est le cas qui comptait.

- **La liste de la piste d'audit défile dans son cadre, pas dans la page.** Vingt-cinq lignes poussaient le pied de page — et les boutons « précédent / suivant » avec lui — hors de l'écran : pour changer de page il fallait faire défiler toute la page jusqu'en bas, puis remonter pour lire. Le tableau a maintenant une hauteur bornée et son propre défilement, et son en-tête reste collé en haut : à la quinzième ligne, on sait encore quelle colonne est laquelle.

- **Le logotype a été refait, et il apparaît en haut à droite de l'espace de travail.** L'ancien fichier était le décalque d'une image : il portait les artefacts de ce décalque, et surtout un rouge `#C42527` là où le drapeau suisse veut `#DA291C` (Pantone 485 C). Les lettres sont de nouveau vectorisées depuis le fichier fourni — écart mesuré après rendu, pixel à pixel : 0,76 sur 255 en moyenne. Le badge, lui, n'est plus décalqué mais **construit** aux proportions de l'ordonnance sur les armoiries (RS 232.21) : sur un carré de 32 unités, des bras de 6 unités de large et 20 d'un bout à l'autre, centrés. Un drapeau suisse est une géométrie, pas un dessin. L'écran de connexion suit le même fichier. Le monogramme « LA » du bureau et de l'installeur n'a pas changé.

### Corrigé

- **L'import d'un relevé bancaire annonçait toujours « 0 écriture(s) ajoutée(s) ».** L'écran de rapprochement lisait `imported` dans la réponse ; la route ne l'a jamais renvoyé — le compte était calculé côté serveur puis jeté par un `_ = imported`. Le message était donc faux à chaque import réussi, sur le seul chiffre qui dit si le geste a servi à quelque chose. La réponse porte maintenant `imported` et `duplicate`, vérifiés à l'écran : un relevé de trois écritures affiche « 3 écriture(s) ajoutée(s) », le même relevé réimporté en compte trois en doublon.

- **Le renvoi de Rapports vers le rapprochement déposait le lecteur sur « Identité ».** L'onglet visé s'écrit dans le fragment de l'adresse (`#banking`) et non en paramètre : une adresse qui ne correspond à aucun onglet ne produit aucune erreur, elle retombe en silence sur le premier. Attrapé au clic, pas à la compilation.

### Retiré

- **L'onglet « Légal » des paramètres.** Il affichait quatre phrases immuables sur le CO, identiques pour toutes les entreprises et sans jamais rien lire de la base consultée. Un onglet qui n'affiche rien de l'installation qu'on regarde apprend à ne pas être ouvert, et fait douter de ses voisins qui, eux, disent quelque chose. Ce que le logiciel a réellement à dire sur la conformité vit dans Maintenance → Conformité, qui lit les livres. Les six clés de traduction partent avec.

## [1.5.5] — 2026-08-26

### Ajouté

- **Le carnet du lait propose un exercice, et non la plage des autres exports.** La période de l'écran Rapports est commune au journal, au grand livre et à la balance : on l'ajuste pour regarder ce qu'on veut, et sa valeur par défaut (du 31 décembre au jour courant) n'était jamais un exercice. Or les deux seuils du carnet portent en droit sur le chiffre d'affaires du **dernier exercice** : ouvrir l'écran et cliquer ne donnait donc aucun verdict.

  Le carnet a maintenant son propre sélecteur. Il liste les **exercices déclarés** quand il y en a — c'est la donnée de l'utilisateur, et elle porte les décalages, les exercices raccourcis et les clôtures qu'aucun calcul ne devine —, à défaut ceux que le **mois de début d'exercice** de la fiche entreprise permet de déduire, en le disant. Il propose d'emblée le **dernier exercice révolu** : on établit un carnet du lait pour déclarer une année terminée, et proposer l'exercice en cours sortirait un document forcément incomplet que son lecteur prendrait pour un bilan. Une option **« Période personnalisée »** conserve le choix libre de dates.

  Les exercices à cheval sont nommés sur leurs deux millésimes — « 2025/2026 » et non « 2025 », qui induirait en erreur pour un exercice courant jusqu'en juin. Vérifié en navigateur dans les deux cas, exercice déclaré et exercice déduit, en français et en allemand.

### Corrigé

- **Le compte 2262 « TVA déductible » était typé comme une dette.** C'est une CRÉANCE : la TVA payée sur les achats, que l'AFC doit rembourser. Le bilan comme l'état du patrimoine calculant le solde d'un passif par `crédit − débit`, et ce compte étant débité à chaque achat, il s'affichait **parmi les engagements avec un montant négatif** — une créance présentée comme une dette négative, sur la pièce que l'on tend à l'administration fiscale. Les totaux, eux, étaient justes : un actif de +81 et un passif de −81 contribuent identiquement à la fortune nette comme à l'équation du bilan, ce qui explique que le défaut ait vécu si longtemps sans que rien ne le signale. La migration 0027 retype les installations existantes ; le plan semé est corrigé pour les bases neuves.

- **L'entête d'un logo n'est plus recopié depuis l'appelant.** Le contrôle qui accepte l'envoi cherche une sous-chaîne, et `data:text/html;image/png;base64,…` contient bien `image/png` : une image déjà assez petite ressortait avec cet entête forgé intact, stocké tel quel dans la fiche société — d'où il part dans les sauvegardes et dans l'archive légale. L'entête est désormais **reconstruit d'après le format réellement détecté dans les octets**, et un format que le produit n'accepte pas (GIF, WebP) est refusé au point où ce format est connu. Les octets de l'image, eux, restent intacts : les ré-encoder dégraderait un PNG déjà optimisé.

- **`db.Rebind` remplaçait les `?` sans distinguer le code SQL du reste.** Les chaînes littérales, les identifiants entre guillemets et les commentaires sont maintenant traversés sans y toucher — commentaires de bloc imbriqués compris, que PostgreSQL admet. Aucune requête du dépôt n'en contenait : le défaut était **latent**. Mais il aurait suffi d'un `LIKE '%?%'`, d'un message stocké en base ou d'un commentaire SQL portant une question pour qu'un `$n` de trop décale TOUS les paramètres suivants — sans rien signaler à la compilation, et sur une requête d'écriture, en écrivant la bonne valeur dans la mauvaise colonne.

- **`gofmt` est réactivé en CI, et le diagnostic qui l'avait fait désactiver était faux.** Le commentaire disait « pre-existing files have Windows CRLF inconsistencies ; run gofmt manually ». Le symptôme était réel — sur un poste Windows avec `core.autocrlf=true`, l'arbre de travail est en CRLF, `gofmt` refuse le CRLF, et `gofmt -l .` signale donc tous les fichiers. **Mais le dépôt stocke du LF** : vérifié octet par octet sur les 211 fichiers `.go`, zéro CR. C'est ce que la CI récupère sur un runner Linux, et `gofmt` n'y signalait rien. Le linteur ne cassait pas la CI ; il cassait une vérification *locale*, et on l'a désactivé pour tout le monde — après quoi la consigne de remplacement, appliquée par rien, a laissé treize fichiers dériver. `.gitattributes` pose en outre `*.go text eol=lf`, pour que le LF ne dépende plus du réglage de chaque poste.

- **Onze des quinze actions d'audit n'étaient jamais écrites.** Le journal annonçait une couverture qu'il n'avait pas : `ActionContactUpdated`, `ActionPaymentRecorded`, `ActionBankEntryMatched`, `ActionDocumentCreated`, `ActionCreditNoteIssued`, `ActionDataExported` et cinq autres étaient déclarées et appelées nulle part. Modifier l'IBAN d'un fournisseur, encaisser un paiement, générer un ordre de virement, anonymiser un contact, exporter toute la comptabilité : aucun de ces gestes ne laissait de maillon.

  Pire, l'en-tête de `audit_trace.go` affirmait **au passé** que trois de ces constantes « étaient déclarées et jamais appelées » — donnant la faute pour réparée alors que leur unique occurrence dans tout le dépôt était ce commentaire même. Un commentaire faux coûte plus cher qu'un commentaire absent : il fait renoncer à vérifier.

  **Les quinze actions sont désormais câblées**, et `internal/frontend/audit_actions_test.go` échoue si l'une cesse de l'être — y compris si elle ne subsiste que dans un commentaire qui prétend le contraire. Vérifié en insérant une constante morte, puis la même accompagnée d'un commentaire affirmant qu'elle est câblée : le test attrape les deux.

- **Le filet de test des permissions rapportait vert sur deux trous.** Il lisait trois lignes à partir de chaque route : une route sans permission **suivie** d'une voisine déclarée empruntait le `authorizer.Require(` de sa voisine et passait. Et le motif n'accrochait que `api.`, si bien qu'une route d'écriture montée sur `v1.` échappait à la fois au test et à toute la pile de filtres — le trou exact qu'était `POST /auth/register`. Le bloc s'arrête maintenant à la parenthèse fermante de son propre appel, le motif couvre les trois groupes, et les dix routes qui vivent délibérément hors du groupe protégé sont **nommées une par une**. Les deux trous ont été reproduits avant et après.

- **La détection des liquidités du carnet reposait sur une comparaison de chaînes.** `estLiquidite` comparait lexicographiquement dans `["1000","1029"]` : « 10200 » était liquide et « 10290 » ne l'était pas, et « 1000A » passait — or `POST /accounts` n'impose aucun format. Surtout, les comptes de **virement interne** 1090/1091 du plan PME tombaient hors fenêtre : chaque virement était compté comme une recette ou une dépense, et tout paiement fait depuis un second compte bancaire créé à la main disparaissait du document. La comparaison porte désormais sur un nombre, la borne monte à 1099, et le sous-adressage est validé.

- **La trace de la fiche entreprise décrivait la requête, pas ce qui avait été écrit.** Basculer l'entreprise en « non assujettie » efface le numéro de TVA après coup : l'audit enregistrait pourtant le numéro demandé, que la base ne contenait plus. Et `vat_status`, `auto_post_invoices` et `bank_address` ne figuraient dans aucun des deux états — changer le statut TVA, qui décide de ce qui s'imprime sur toute facture émise ensuite (LTVA art. 27 al. 1), ne laissait aucune trace. L'état « après » est maintenant lu dans la ligne relue.

- **La branche « création » de la fiche entreprise était morte.** La branche `INSERT` remplissait `existingID`, si bien que le test `existingID != ""` était toujours vrai : la toute première écriture était enregistrée comme une *modification depuis `{}`* et l'écran d'audit affichait « 18 champs modifiés » sur une création qui n'avait rien remplacé.

- **Les transactions SQLite étaient *deferred*.** Lire le maillon d'audit précédent puis insérer le suivant fige un instantané ; une écriture concurrente rendait `SQLITE_BUSY_SNAPSHOT`, que `busy_timeout` ne réessaie pas — attendre ne rajeunit pas un instantané. Sur les deux points de trace où l'échec est journalisé et non remonté, le maillon disparaissait en silence. `_txlock=immediate` transforme le conflit en attente.

- **Treize fichiers Go n'étaient pas formatés.** `gofmt` a été retiré des vérificateurs de `.golangci.yml` pour contourner un problème de fins de ligne, remplacé par la consigne « run gofmt manually » — que rien n'applique. Les écarts sont cosmétiques (alignement de champs, lignes vides finales) ; ils sont corrigés. **Réactiver `gofmt` en CI reste à faire** et suppose de normaliser les fins de ligne des 208 fichiers Go du dépôt, qui stocke aujourd'hui du CRLF : c'est un changement à isoler dans son propre commit.

- **Le carnet du lait perdait une contrepartie sur toute écriture composée.** Le classement se faisait par écriture entière — recette si l'argent entrait, dépense s'il sortait —, puis chaque contrepartie n'était lue que du côté OPPOSÉ au mouvement. Celle qui se trouvait du même côté que la liquidité portait zéro et disparaissait en silence. Un encaissement de 1000 francs diminué de 50 de frais bancaires annonçait donc un résultat de **1000** au lieu de 950, les frais n'apparaissant nulle part ; un règlement fournisseur de 1000 avec 20 d'escompte annonçait **−1000** au lieu de −980, ce qui **sous-déclarait le bénéfice** ; une caisse de fin de journée faisait s'évaporer le salaire payé du tiroir. Ces trois formes sont la manière normale de passer ces opérations.

  Un crédit de contrepartie est désormais une recette, un débit une dépense, et les deux sont comptés. **Le document refuse en outre de sortir s'il ne concorde pas** : le résultat doit égaler le mouvement net de trésorerie au centime, faute de quoi une erreur est rendue plutôt qu'un carnet. Sur une pièce remise à l'administration, l'absence de document est un problème d'exploitation ; un document faux est un problème fiscal. Le fabricant d'écritures des tests ne produisait que des écritures à deux lignes, où le défaut ne pouvait pas se manifester — c'est ce qui l'avait laissé passer.

- **Les seuils légaux étaient appréciés sur n'importe quelle période.** Ceux du CO art. 957 al. 2 ch. 1 (500 000 francs) et de la LTVA art. 10 al. 2 let. a (100 000) portent en droit sur le chiffre d'affaires du **dernier exercice**. Ils étaient comparés au chiffre d'affaires de la période demandée, sans contrôle de sa longueur : une entreprise à 1,6 million qui demandait un carnet trimestriel recevait un PDF affirmant **en vert, sous la référence légale**, que la comptabilité simplifiée lui était admise — et le carnet mensuel d'une entreprise à 300 000 annonçait la libération de l'assujettissement TVA. Deux affirmations de droit fausses sur la pièce que l'on tend à l'administration.

  Sur une période plus courte qu'un exercice, l'écran, le CSV et le PDF **suspendent leur verdict** et disent pourquoi, dans les quatre langues. L'avertissement de dépassement, lui, subsiste : un chiffre d'affaires déjà au-dessus du seuil sur un trimestre y restera sur l'année, et c'est la mention qui protège le plus l'utilisateur.

### Supprimé

- **`docs/REVUE-IA.md`.** Le mode d'emploi et le prompt pour faire auditer le code par un réviseur IA depuis une copie locale. Le lien qui y mène depuis l'entrée de la v1.5.2 plus bas ne pointe donc plus sur rien : cette entrée-là décrit ce qui a été ajouté à sa date et n'est pas réécrite.

### Sécurité

- **L'extraction d'images d'un PDF borne désormais ce qu'elle écrit sur le disque.** Les bornes posées précédemment (nombre d'images, cumul de pixels) protégeaient la mémoire, mais s'appliquaient APRÈS `ExtractImagesFile`, qui avait déjà tout déversé. Un PDF fournisseur dont les images se décompressent massivement remplissait le disque avant que la première ne soit décodée — et ce disque porte aussi la base comptable. L'extraction se fait maintenant **page par page**, en mesurant entre chaque : au-delà de 256 Mio ou de 20 pages, on s'arrête. On s'arrête plutôt qu'on échoue — le bulletin QR est presque toujours dans les premières pages, et une facture volumineuse mais légitime doit rester lisible.

  Le chemin PDF complet est par ailleurs testé pour la première fois : un vrai PDF est fabriqué, portant un vrai QR, et relu de bout en bout — y compris le cas du bulletin sur la **troisième** page, que l'extraction page par page aurait pu casser en s'arrêtant trop tôt.

- **Les trois vulnérabilités Go signalées sont corrigées.** Elles se réglaient toutes par une montée de CORRECTIF, sans changement d'interface : `golang.org/x/image` v0.44.0 → v0.45.0, `quic-go` v0.59.0 → v0.59.1, `pgx/v5` v5.9.1 → v5.9.2. Le `go get -u` prévu au plan d'action du premier audit n'avait jamais été exécuté — `go.mod` était inchangé depuis la v1.5.2.

  *Note sur `x/image`* : `govulncheck` traçait GO-2026-6222 par `imgsafe.Decode → image.DecodeConfig → vp8l.Decode`, ce qui faisait de la garde ajoutée en v1.5.3 le point d'entrée du défaut. Lecture de `webp/decode.go:113` : `DecodeConfig` appelle `vp8l.DecodeConfig` puis **retourne**, `vp8l.Decode` n'est jamais atteint — l'outil ne voyait pas que `configOnly` valait vrai. C'était un faux positif, et la garde tenait. La montée le referme quand même.

- **La base comptable était en 0755 sur toute installation Linux.** `scripts/install.sh` créait `/var/lib/ledgeralps` par `mkdir -p` puis `chown`, **sans `chmod`** : sous un umask root normal (022), le répertoire naissait en 0755, et SQLite y crée son fichier en 0644. Tout compte local lisait donc la comptabilité entière — factures, contacts, IBAN, dix ans de pièces (CO art. 958f). `ProtectSystem=strict` et `NoNewPrivileges` protègent le processus, pas les lecteurs. Le durcissement de `/etc/ledgeralps` (la clé JWT) avait été posé sans celui-ci, alors que le geste existait déjà dans `infrastructure/linux/preinstall.sh` — un fichier devenu du code mort depuis l'abandon des paquets .deb/.rpm en v1.4.5. **Les installations existantes ne sont pas corrigées par la mise à jour** : passez `chmod 750 /var/lib/ledgeralps /var/log/ledgeralps`.

- **`syft` était installé depuis une branche mouvante, dans le job qui publie.** Le workflow versait dans `sh` un script relu à chaque exécution depuis `anchore/syft@main`, sans empreinte, dans un job portant `contents: write` et le `GITHUB_TOKEN`. La *version* de l'outil était épinglée ; le script qui l'installe ne l'était pas. Le binaire publié est désormais téléchargé et **vérifié par son empreinte SHA-256** avant d'être installé.

- **L'installeur Windows, chemin recommandé, n'avait aucune empreinte.** GoReleaser écrit et publie `checksums.txt` depuis un job qui s'achève avant que l'installeur ne soit construit : vérifié sur les releases v1.5.3 et v1.5.4, le fichier contenait quatre lignes et aucune pour `LedgerAlps_Setup_*.exe`. Les deux scripts d'installation vérifiaient les leurs ; le téléchargement que la note de version **recommande** était le seul que personne ne pouvait vérifier. Son empreinte est maintenant ajoutée au `checksums.txt` de la release. *(Cela ne remplace pas une signature Authenticode, qui suppose un certificat OV/EV.)*

- **L'arbre npm était résolu à neuf à chaque publication.** Aucun `package-lock.json` n'était suivi et la CI faisait `npm install --no-package-lock` : le JavaScript compilé puis embarqué dans `ledgeralps-server.exe` n'était ni reproductible ni auditable, et une version malveillante d'une dépendance transitive y serait entrée sans laisser de trace. Le SBOM n'y changeait rien — syft inventorie les modules Go, et le JavaScript minifié vit dans un `embed.FS`. **Le verrou est désormais suivi et la CI utilise `npm ci`.** Dans la foulée, les correctifs non cassants ont été appliqués : **13 vulnérabilités (7 hautes) → 4 (1 haute)**, dont axios 1.14.0 → 1.19.0. Les quatre restantes (vite, esbuild, react-router) demandent des montées de version majeures.

- **Un PDF fournisseur pouvait épuiser la mémoire du serveur.** La garde `imgsafe` refuse une image de plus de 25 mégapixels, mais rien ne bornait ni le **nombre** d'images extraites d'un même document ni le **cumul** de leurs pixels — toutes étant conservées en mémoire simultanément. Un PDF de 10 Mo peut porter des centaines d'aplats qui passent chacun sous la garde. Le plafond de 32 Mo posé en v1.5.3 bornait le téléversement, pas l'expansion. Deux bornes ajoutées : 64 images, et quatre fois le budget d'une image seule.

- **Le masquage nLPD cassait sur un guillemet, et laissait fuir ce qu'il masquait.** Le masquage opérait par expression régulière sur du JSON déjà encodé et terminait la valeur au premier guillemet **échappé**. Une raison sociale banale — *Au « Bon » Vin Sàrl* — produisait un `after_state` invalide **et** laissait un fragment du nom de l'indépendant en clair dans une table conservée dix ans. L'écran d'audit, qui parse ce JSON, affichait alors « aucun champ modifié » sur un changement d'IBAN. Et la chaîne d'empreintes restait valide, puisqu'elle porte sur la chaîne stockée : le maillon corrompu se vérifiait. **Le masquage opère désormais sur la structure, avant encodage**, et couvre au passage les valeurs non textuelles, les objets imbriqués et les clés en casse différente (`Email`, `IBAN`, `customerName`).

- **Toutes les actions GitHub sont épinglées par SHA.** Les 28 déclarations `uses:` pointaient sur des étiquettes mouvantes, dont `goreleaser/goreleaser-action@v6` dans le job portant `contents: write` — déplacer l'étiquette suffisait à publier sous le nom LedgerAlps. Les versions majeures sont inchangées. `golangci-lint` passe de `version: latest` à `v1.64.8` : le `.golangci.yml` du dépôt est au schéma v1, que la v2 refuse.

- **Bloc `$GITHUB_ENV` à délimiteur fixe alimenté par des données réseau.** `compliance-watch.yml` écrivait `REPORT<<EOF` … `EOF` directement dans `$GITHUB_ENV`, où le contenu vient de réponses HTTP. Une ligne valant exactement `EOF` fermait le bloc en avance et le reste devenait des affectations de variables, dans un job portant `issues: write`. Délimiteur aléatoire, et refus si la valeur le contient.

- **L'installeur lançait LedgerAlps en Administrateur.** `MUI_FINISHPAGE_RUN` est exécuté par MUI depuis le processus de l'installeur, qui porte le jeton élevé : le serveur HTTP, le navigateur, et surtout `config.json` — porteur de `jwt_secret` — et la base naissaient sous le `%APPDATA%` de l'administrateur. La reprise passe désormais par `explorer.exe`, qui tourne déjà sous le compte de l'utilisateur. *(Le greffon `ShellExecAsUser` aurait été plus direct mais ne fait pas partie de la distribution NSIS standard — vérifié au compilateur.)*

- **`Bootstrap` n'était pas atomiquement à un coup.** Le `COUNT(*)` et l'`INSERT` étaient séparés par un hachage bcrypt d'une centaine de millisecondes, hors transaction : deux requêtes simultanées produisaient deux administrateurs. Contrôle et insertion sont désormais dans la même transaction, qui prend le verrou d'écriture dès son ouverture.

- **La table des tentatives du mode récupération ne s'élaguait jamais.** Elle ne purgeait une adresse que si cette même adresse revenait : un balayage la faisait croître sans terme, sur un point non authentifié, dans un processus fait pour attendre des heures. Purge générale à chaque passage, et trois délais HTTP ajoutés au serveur de récupération.

- **`install.ps1` : empreinte cherchée par sous-chaîne, et chemins non validés.** La recherche retenait toute ligne *contenant* le nom de l'archive — or `…zip.sbom.json` contient `…zip` — et ne tombait juste que par une propriété de tri que rien ne garantit. Comparaison de champ, alignée sur `install.sh`. Par ailleurs `-InstallDir` et `-DataDir` n'étaient pas validés, contrairement à leur pendant Linux, alors que le premier finit dans le PATH machine : un `;` y injectait des entrées pour tous les comptes.

## [1.5.4] — 2026-08-25

### Ajouté

- **La comptabilité simplifiée — le « carnet du lait » — dans Rapports.** Une entreprise individuelle dont le chiffre d'affaires reste sous 500 000 francs peut se limiter à « une comptabilité des recettes et des dépenses ainsi que du patrimoine » (CO art. 957 al. 2 ch. 1). LedgerAlps produit désormais ce document, à l'écran, en CSV et en **PDF** — celui que l'on tend à l'administration.

  **LedgerAlps ne devient pas une comptabilité simplifiée pour autant.** Le produit continue de tenir la partie double, qui dépasse le minimum légal ; le carnet en est une présentation extraite. C'est un avantage à faire valoir devant l'administration, et cela explique qu'il n'y ait rien à « activer ».

  **Base caisse, et non compte de résultat.** « Recettes et dépenses » veut dire qu'on compte l'argent au moment où il entre et où il sort. Dériver le document du compte de résultat aurait produit un compte de résultat portant un autre nom, avec les factures émises non encaissées comptées comme des recettes — un écart qu'un contrôleur voit dès qu'il le rapproche du relevé bancaire. Les mouvements viennent donc du journal : tout mouvement d'argent touche un compte de liquidités, un débit est une entrée, un crédit une sortie, et la contrepartie donne la nature.

  **Le virement interne est écarté.** Un retrait au bancomat touche deux comptes de liquidités : ce n'est ni une recette ni une dépense, mais une lecture ligne à ligne l'aurait compté **deux fois** et gonflé les deux colonnes — donc annoncé à l'administration un chiffre d'affaires qui n'existe pas.

  **Le document dit s'il suffit.** Au-delà de 500 000 francs, l'écran et le PDF écrivent que la partie double et les comptes annuels sont obligatoires (CO art. 957 al. 1) et que ce carnet ne peut pas être présenté seul. À partir de 100 000 francs, ils rappellent l'assujettissement TVA (LTVA art. 10). Deux lois différentes, deux seuils qui ne décident pas de la même chose : l'un porte sur la forme des livres, l'autre sur un impôt.

  **Le document sort dans la langue de l'interface au moment du clic.** Écran, CSV, PDF et jusqu'au nom du fichier : établi depuis un écran allemand, le carnet est en allemand. C'est la pièce que l'on tend à une administration cantonale, et elle doit parler la langue de son destinataire. Les **références légales changent de nom**, pas seulement de langue — le Code des obligations est l'*OR* en allemand, la loi sur la TVA la *MWSTG* ; un document remis à Zurich qui citerait « CO art. 957 » citerait une loi qui n'y porte pas ce nom. Les noms de comptes, eux, restent tels que l'utilisateur les a saisis : c'est sa donnée, pas un libellé du produit.

  Vérifié de bout en bout sur un serveur réel, avec des écritures saisies par l'API : les 8 000 francs d'une facture non encaissée restent hors des recettes mais comptent dans le chiffre d'affaires, le retrait au bancomat n'apparaît nulle part, et l'amortissement non plus. Le carnet a été produit dans les quatre langues depuis un navigateur, en basculant l'interface avant de cliquer.

### Corrigé

- **La ponctuation typographique sortait en « ? » dans les PDF.** Le générateur n'accepte que du Latin-1 et remplaçait tout le reste par un point d'interrogation : « Recettes **?** CO art. 957 ». Les tirets cadratins, apostrophes typographiques, points de suspension et le symbole € sont désormais translittérés. **Cela corrigeait aussi les factures**, où un tiret saisi dans une description produisait déjà ce défaut.

- **L'audit différentiel : la piste dit maintenant ce qu'une action a REMPLACÉ.** Le journal capturait l'état après chaque modification, jamais celui d'avant — on savait qu'une facture valait 1500.- après coup, sans pouvoir dire qu'elle valait 1000.- avant. Les six points de trace transmettent désormais une transition explicite (création, modification, suppression), et l'écran **Paramètres → Maintenance → Piste d'audit** porte une colonne **« Champs modifiés »** qui nomme ce qui a bougé.

  **Le masquage rendait le cas principal invisible, et c'est ce qui a guidé la conception.** Les champs personnels sont remplacés par `[MASKED]` (nLPD art. 6) : un IBAN modifié aurait donné `[MASKED]` avant *et* après, faisant disparaître exactement le changement qui motive la fonctionnalité — cet IBAN est le compte qui reçoit les virements de tous les clients. La liste des champs modifiés est donc calculée sur les valeurs **brutes, avant masquage**. On sait que l'IBAN a changé, et qui l'a changé, **sans conserver aucun des deux IBAN** : plus utile que deux valeurs masquées, et moins de données personnelles retenues — la minimisation de la nLPD va ici dans le même sens que l'utilité.

  **La liste entre dans l'empreinte.** L'effacer pour cacher qu'un IBAN a bougé casse la chaîne, comme le fait de réécrire l'état antérieur. Une trace des changements que l'on pourrait réinscrire ne prouverait rien. Deux tests le vérifient en falsifiant réellement la base.

  **Aucune migration, aucune rupture.** Une création écrit `NULL` et la vérification relit `COALESCE(before_state, '')` : les maillons écrits avant cette version se recalculent à l'identique, et se vérifient à côté des nouveaux. Vérifié de bout en bout sur un serveur réel — deux modifications de la fiche entreprise, chaîne `verified: true`, et la colonne affiche `iban` puis `address_city, company_name`.

  `from`/`to` écrits à la main dans l'état suivant disparaissent au passage : la transition a désormais un endroit pour se dire.

### Sécurité

- **Le masquage des données personnelles couvre les variantes composées.** Il ne reconnaissait que les clés exactes — `name`, `address`, `iban` — et laissait donc passer `company_name`, `legal_name`, `supplier_name`, `address_street`, `address_city` : chez un indépendant, son propre nom et son adresse privée, écrits en clair dans une table conservée dix ans (CO art. 958f). Le terme sensible est maintenant reconnu comme mot, préfixé ou suffixé. Les clés voisines restent lisibles — `number` ne contient pas `name` —, sans quoi une piste entièrement masquée n'apprendrait plus rien. Les maillons existants gardent leurs valeurs et leur empreinte ; seuls les suivants sont plus discrets.

## [1.5.3] — 2026-08-24

### Sécurité

- **Une image ne peut plus faire tomber le serveur par sa seule taille.** Un plafond posé sur les octets ne borne rien : les formats d'image compressent, et un aplat uniforme compresse énormément. Un PNG de 20 000 × 20 000 pixels d'une seule couleur pèse environ 1,5 Mo et faisait réserver **1,6 Gio** — `image.Decode` alloue `largeur × hauteur × 4` **avant** de lire le premier octet de pixel. Sur les 8 Go d'un poste de bureau, le système tuait le processus, et ce processus portait la comptabilité en cours. Les images sont désormais refusées sur leurs **dimensions**, lues dans l'en-tête sans rien allouer, aux trois portes concernées : l'envoi du logo, le dépôt d'une facture fournisseur, et les images extraites d'un PDF reçu. La dernière est la plus réaliste — elle ne demande à l'attaquant qu'un courriel. Le test de non-régression construit une vraie bombe : 48 Ko sur le disque, 34 mégapixels une fois décodés.

- **La chaîne d'audit ne peut plus se fourcher.** L'écriture d'un maillon lit le précédent puis insère le suivant, et cela se faisait **hors transaction** alors que la fonction documentait elle-même exiger le contraire — `execQuerier` étant satisfait aussi bien par une connexion que par une transaction, rien ne le signalait à la compilation. Deux écritures concurrentes lisaient donc le même prédécesseur et produisaient deux maillons de même numéro. La vérification lisait alors « lien rompu » puis « -1 entrée supprimée » et posait un échec **définitif** : le produit accusait l'utilisateur d'avoir altéré ses livres alors que personne n'avait rien touché — et une fausse alerte n'est crue qu'une fois. Les deux chemins d'écriture passent maintenant par une transaction, et un index **unique** sur le numéro de séquence rend la fourche impossible même si un chemin futur l'oubliait. Les bases déjà fourchues sont renumérotées par la migration plutôt que refusées : aucune empreinte n'est touchée, le numéro de séquence n'entrant pas dans leur calcul.

- **Le corps des requêtes est borné avant d'être lu.** `c.FormFile` analyse la totalité du corps avant que le gestionnaire puisse regarder la taille du fichier : le contrôle des 10 Mo s'appliquait à un fichier **déjà écrit sur le disque**. Au-delà de la mémoire allouée, `multipart` déverse sans plafond — un envoi de plusieurs gigaoctets remplissait le disque qui porte aussi la base comptable.

- **Le certificat auto-signé n'est plus une autorité de certification.** Il portait `IsCA` et le droit de signer, sans contrainte de nom. Le geste évident pour faire taire l'avertissement du navigateur est de l'importer dans les autorités racines de confiance — et à partir de là, quiconque lisait la clé dans `%APPDATA%` pouvait fabriquer un certificat valable pour **n'importe quel domaine**. Le rayon d'action passait d'un fichier local à l'interception de toute la navigation. Une feuille signée par elle-même produit le même avertissement, sans ce pouvoir.

- **Le serveur pose des délais.** Ni `ReadHeaderTimeout`, ni `ReadTimeout`, ni `WriteTimeout` : une connexion envoyant son en-tête octet par octet occupait une goroutine indéfiniment, et il en faut peu pour les occuper toutes. Le mode récupération en posait déjà un ; le serveur principal était le seul des deux à n'en poser aucun. L'écriture reste large à dessein — l'archive légale et le téléchargement groupé passent par là.

- **Les quatorze routes d'écriture qui ne déclaraient aucune permission le font maintenant.** Elles n'étaient pas exploitables : le garde global refuse toute écriture à un rôle en lecture seule. Mais il ne connaît que le binaire lecteur / non-lecteur, et ne distinguera jamais l'administrateur du comptable. L'écart se voyait déjà — `PUT /settings/company` exigeait un droit de gestion pendant que `POST /settings/logo`, qui écrit dans la **même ligne**, n'exigeait rien. Un test parcourt désormais la table des routes et échoue sur toute écriture non déclarée. Deux d'entre elles étaient des **lectures déguisées en POST** — le téléchargement groupé de factures et la déclaration TVA — et se voyaient refusées à la lecture seule, alors que produire ses factures est précisément la raison d'être de ce rôle.

- **L'obligation d'inscrire un second facteur ne se lève plus sur un défaut de lecture.** Le contrôle rendait « conforme » quand la base était illisible. Le motif d'origine — table absente sur une base antérieure à la migration — a disparu depuis que celle-ci s'applique au démarrage ; ne restait qu'un verrou ou un contexte expiré, et une règle levable ainsi est levable à volonté.

- **La rétention des données personnelles s'exécute chaque jour**, et non plus au seul démarrage. Une passe unique tient la promesse sur un poste qu'on éteint le soir, et pas du tout sur un serveur allumé toute l'année — qui est justement l'installation où les adresses s'accumulent, pendant que l'écran de maintenance affiche les durées comme si elles s'appliquaient.

- **`/uid-lookup` est limité en débit.** C'est le seul point public déclenchant un appel sortant ; rien n'empêchait un tiers de le faire émettre en boucle depuis une installation censée rester hors ligne. (Il n'y a pas de SSRF : l'entrée est validée avant toute construction d'URL.)

- **Durcissement de la chaîne de publication et de l'installeur** : `-trimpath` sur les quatre cibles — les binaires portaient les chemins absolus de la machine de compilation —, un **SBOM** publié à côté de chaque archive, `permissions: contents: read` sur les deux workflows qui n'en déclaraient pas, `SetShellVarContext all` autour des raccourcis (sous une élévation par identifiants, ils atterrissaient dans le profil de l'administrateur et l'utilisateur n'avait aucun raccourci), `SetRegView 64` pour que la clé de désinstallation cesse d'être reléguée dans `WOW6432Node`, et validation des chemins passés en variables d'environnement à `install.sh`, qui finissaient dans une unité systemd via un heredoc non protégé.

- **L'inscription publique est retirée.** `POST /auth/register` créait un compte **comptable actif**, sans mot de passe temporaire, sans qu'aucun administrateur l'ait voulu. Or le comptable détient `PermManage` : il règle la fiche entreprise — donc **l'IBAN qui s'imprime sur toutes les factures émises ensuite** —, écrit au journal, clôture un exercice. La chaîne complète a été exécutée sur un serveur réel : cinq requêtes, aucune authentification de départ, l'attaquant s'inscrivant lui-même son propre second facteur au passage, et la raison sociale comme l'IBAN modifiés en base.

  La route était par ailleurs **morte côté interface** : aucune page ne l'appelait. C'était un vestige d'avant les rôles, que `POST /users` (droit administrateur, mot de passe temporaire obligatoire) remplace depuis. Un vestige que personne ne voit à l'écran est justement celui auquel personne ne pense. Un compte se crée maintenant par un administrateur, et seulement ainsi ; le tout premier passe par `bootstrap`, qui ne fonctionne qu'une fois. Un test tient la décision : si la route revient, il échoue.

- **L'en-tête `X-Forwarded-For` n'est plus cru de personne par défaut.** gin fait confiance à `0.0.0.0/0` et `::/0` tant qu'on ne lui dit rien, si bien que l'adresse observée était celle que l'appelant écrivait lui-même. Trois choses en dépendaient, et les trois se cassaient en silence : le **verrouillage des connexions**, dont la clé est l'adresse — une valeur différente à chaque requête est une clé différente, et l'échelle 30 s → 1 h ne se déclenchait jamais ; le **verrouillage ciblé d'un tiers** — dix requêtes portant l'adresse du comptable le tenaient hors de son propre logiciel ; et l'**adresse scellée dans la chaîne d'audit** (CO art. 957a), qui devenait celle de l'attaquant. La chaîne restait parfaitement cohérente : elle scellait un mensonge.

  L'adresse retenue est désormais celle de la connexion. Une installation derrière un reverse proxy déclare le sien dans `TRUSTED_PROXIES` (adresses ou CIDR) — vérifié dans les deux sens sur un serveur réel : sans la variable, douze adresses forgées tombent sur le même compteur et le verrou arrive au onzième essai ; avec `TRUSTED_PROXIES=127.0.0.1`, chaque adresse retrouve le sien.

- **Le mode récupération ne sert plus la phrase en clair, et compte les essais.** Cet écran demande la phrase qui enveloppe la clé de dix ans de comptabilité. Il faisait `ListenAndServe` sans condition : sur une installation réseau à certificat — donc une installation où le produit a délibérément imposé TLS —, le formulaire partait **en HTTP clair sur le réseau local**. Il suit maintenant la même règle que le serveur normal ; le loopback reste en clair, parce qu'il n'y a pas de câble.

  **Trois essais par minute et par adresse**, là où rien ne bornait quoi que ce soit. Une phrase se tape à la main, une fois : la limite ne gêne personne et ferme la porte à l'essai automatisé. Elle est posée **avant** la dérivation de clé, qui coûte 64 Mio par tentative — la compter après revenait à la payer.

- **L'installation vérifie enfin ce qu'elle installe.** `install.sh` et `install.ps1` téléchargeaient, extrayaient et installaient — dans `/usr/local/bin` avec les droits du superutilisateur, dans `C:\Program Files` puis en service à démarrage automatique — **sans rien vérifier**, alors que GoReleaser publie déjà les empreintes SHA-256 à côté de chaque archive. Les deux modes d'emploi documentés (`curl … | bash`, `irm … | iex`) sont précisément ceux qui suppriment toute occasion d'inspecter ce qui arrive. L'empreinte est maintenant contrôlée **avant l'extraction**, et une archive qui ne correspond pas fait échouer l'installation en nommant les deux valeurs. Vérifié sur l'artefact publié en v1.5.2 : l'authentique passe, une copie altérée d'un seul octet est refusée.

- **`JWT_SECRET` n'est plus lisible par tout compte local.** `/etc/ledgeralps` naissait en `0755` et son gabarit en `0644` ; côté Windows, `C:\ProgramData\LedgerAlps` héritait des ACL de son parent, qui donnent la lecture au groupe `Users`. Ce répertoire porte la clé qui **signe les jetons de session** — et, sur Windows, la base comptable elle-même. N'importe quel compte de la machine, y compris un compte de service compromis d'une autre application, pouvait la lire et forger un jeton administrateur. Le durcissement de l'unité systemd (`NoNewPrivileges`, `ProtectSystem=strict`) était méticuleux et protégeait tout **sauf la clé qui ouvre l'application**. Désormais : `750` avec le groupe `ledgeralps` sous Linux, héritage coupé et accès limité à SYSTEM et aux administrateurs sous Windows, gabarit en `640`.

- **Le service Windows tourne sous un compte dédié, plus en LocalSystem.** `sc.exe create` était appelé sans `obj=`, donc le serveur HTTP servant la comptabilité avait les droits de la machine : toute exécution de code dans le processus devenait un contrôle total du poste. Le pendant Linux du même service tourne pourtant sous un compte dédié depuis toujours — le raisonnement de moindre privilège avait été fait une fois, puis pas porté sur Windows. Le service utilise maintenant le compte de service virtuel `NT SERVICE\LedgerAlps`, sans mot de passe à stocker, avec un accès accordé explicitement à ses seules données.

### Corrigé

- **Le service Windows ne pouvait pas démarrer, jamais.** Il était enregistré sur un fichier `.bat`, alors que le gestionnaire de services attend que le processus lancé appelle `StartServiceCtrlDispatcher` dans le délai imparti — ce que `cmd.exe` exécutant un script batch ne fait pas. L'échec était donc systématique (erreur 1053), **quel qu'ait été le contenu du script**. Celui-ci en avait par ailleurs deux propres : `%%A:~0,1%` n'est pas une syntaxe valide sur une métavariable de boucle `for`, si bien que le filtre de commentaires ne filtrait rien, et `set "%%A=%%B"` sortait de ses guillemets sur une valeur contenant `"` et `&`. Le wrapper est supprimé — et effacé s'il traîne d'une installation précédente ; l'environnement passe par la valeur `Environment` de la clé du service, que le gestionnaire lit nativement. Avec le nom d'archive corrigé plus bas, c'était la **seconde** raison pour laquelle l'installation Windows par script ne pouvait pas aboutir.

- **Les codes de retour de `sc.exe` étaient jetés.** Les quatre appels finissaient par `| Out-Null` sans qu'aucun `$LASTEXITCODE` soit lu, si bien que le script imprimait « registered » puis « installed successfully » après un échec. Le cas concret : `sc.exe delete` laisse le service marqué pour suppression tant qu'un handle reste ouvert, et l'attente d'une seconde ne suffit pas toujours — `create` échouait alors avec 1072, en silence. L'installeur attend maintenant la disparition réelle du service, et s'arrête en nommant le code sur chaque échec.

- **Le répertoire temporaire de l'installeur Windows était prévisible et réutilisé.** Nom fixe `ledgeralps-install` recréé avec `-Force` — donc acceptant un dossier préparé par un tiers, ACL ouvertes ou jonction — puis fouille récursive qui retenait le **premier** exécutable portant le bon nom, où qu'il se trouve, pour l'installer en service. Le cas réel est le script lancé en SYSTEM par un outil de gestion de parc, où `%TEMP%` vaut `C:\Windows\Temp` et où le groupe `Users` peut créer des dossiers. Le répertoire porte maintenant un GUID, sa création échoue s'il existe, et les binaires sont pris à leur chemin attendu.

- **Les deux scripts d'installation demandaient un fichier qui n'a jamais existé.** `install.sh` et `install.ps1` construisaient le nom d'archive avec le tag **entier** (`ledgeralps_v1.5.2_…`) alors que GoReleaser le nomme **sans le « v »** (`ledgeralps_1.5.2_…`). Le chemin de l'URL veut le tag avec, le nom de fichier le veut sans, et les deux scripts utilisaient la même variable pour les deux. Résultat : **404 sur les trois dernières versions**, vérifié artefact par artefact — la commande `curl -fsSL … | bash` imprimée dans les notes de chaque version n'a donc jamais fonctionné. L'installeur Windows n'était pas touché. Le workflow de publication faisait déjà ce retrait de son côté ; les scripts le font maintenant aussi.

- **`taskkill` était appelé sans chemin absolu par un installeur élevé.** `CreateProcess` cherche d'abord dans le répertoire de l'exécutable appelant — le dossier Téléchargements, dans la quasi-totalité des cas — puis dans le répertoire courant, et n'atteint `System32` qu'en troisième. Or la macro d'arrêt est insérée **avant** le `SetOutPath` de la section, et le script demande l'élévation : un `taskkill.exe` déposé à côté du programme d'installation s'exécutait **avec les droits Administrateur**. Le chemin est maintenant `$SYSDIR\taskkill.exe`.

- **Douze messages d'erreur illisibles.** Un remplacement global antérieur (`date` → `la date`) avait corrompu le préfixe des noms de champs : l'utilisateur qui se trompait de format de date lisait « issue_la date doit être au format AAAA-MM-JJ ». Les traductions allemande, italienne et anglaise, elles, étaient correctes depuis le début — seule la source française était cassée, et elle servait aussi de clé au catalogue. Corrigé des deux côtés, y compris dans un fichier que le rapport d'audit source n'avait pas repéré.

- **L'installeur Windows n'annonce plus une suppression qui n'a pas eu lieu.** Sous une élévation par identifiants, le dossier de données visé est celui de l'administrateur et non celui de l'utilisateur : la suppression ne portait sur rien, et l'écran affichait quand même « Données supprimées ». Pour un logiciel comptable soumis aux obligations d'effacement de la nLPD, c'est le pire des deux échecs possibles — la base survit à une demande de destruction, et l'utilisateur a une trace écrite du contraire. Un message distinct dit maintenant qu'aucune donnée n'a été trouvée, et où chercher.

- **`Add-ToPath` ne détruit plus l'indirection `%SystemRoot%` du PATH système.** La lecture développait les variables, la réécriture figeait la forme développée — définitivement, pour toutes les entrées. Sur un poste où `%SystemRoot%` n'est pas `C:\Windows` (image de déploiement, changement de lettre de volume), le PATH pointait ensuite sur des chemins morts : une panne à l'échelle de la machine, causée par un installeur applicatif.

- **`install.ps1` refuse ARM avec la raison**, au lieu de laisser un 404 se présenter comme un échec de téléchargement — `install.sh` le disait déjà pour Linux.

- Code mort retiré : `WinMessages.nsh` inclus sans qu'aucun de ses symboles soit utilisé, et le bloc PowerShell qui prétendait diffuser un changement d'environnement sans jamais appeler la fonction correspondante — deux vestiges d'une même intention abandonnée.

- **`before_state` de la piste d'audit** était calculé par un masquage de la chaîne vide : le code avait l'air de traiter un état antérieur, il n'en recevait aucun. La colonne vaut désormais `NULL`, ce qui distingue « non capturé » de « vide ». *L'audit différentiel lui-même — relire l'enregistrement avant de le modifier — reste à faire : c'est une fonctionnalité à porter sur les neuf points de trace, pas un paramètre à ajouter en passant.*

### Documentation

- `docs/API.md` annonçait « 5 échecs par IP » là où le code en fait **10**, et ne décrivait pas l'échelle de verrouillage. Corrigé, avec la règle sur les en-têtes d'adresse.
- `TRUSTED_PROXIES` documenté dans `.env.example` et `docs/PRODUCTION.md`.

## [1.5.2] — 2026-08-20

### Ajouté

- **Un nouvel écran de connexion, à deux panneaux.** À gauche la marque et ce que le produit garantit — données locales, conformité CO et nLPD ; à droite les identifiants. La mise en page et les couleurs viennent du gabarit validé, posées comme telles dans la configuration Tailwind plutôt qu'approchées avec la palette du reste de l'application.

  **Trois éléments du gabarit n'ont pas été repris.** Les CDN (Tailwind, Font Awesome, Google Fonts) : la politique de sécurité les bloque, et surtout chaque appel transmettrait l'adresse IP de l'utilisateur à un tiers, ce qui contredit « vos données restent sur votre machine ». Le logo redessiné en HTML : le fichier officiel existe, c'est lui qui s'affiche. Et « Oublié ? » avec « Rester connecté » : LedgerAlps n'envoie aucun courriel — un lien de réinitialisation ne mènerait nulle part — et il n'existe pas de session longue au niveau du mot de passe, la seule mémoire réelle étant celle du second facteur, qui a son propre réglage à l'étape suivante.

  **Le témoin « système opérationnel » interroge vraiment le serveur**, à l'ouverture puis toutes les trente secondes. Une pastille verte peinte en dur serait une décoration ; celle-ci passe au rouge quand l'API ne répond plus, ce qui distingue « mauvais mot de passe » de « serveur arrêté » au moment précis où la question se pose.

- **Le choix de la langue en pied de l'écran de connexion.** Il vivait dans Paramètres → Mon compte, c'est-à-dire derrière la connexion : un employé germanophone ou une fiduciaire tessinoise devait lire le français pour trouver comment ne plus lire le français.

- **Un conseil de sécurité sur l'écran de connexion**, tiré au sort parmi trente-deux et frappé à la machine, en gros et en gras — il occupe désormais seul le panneau de gauche, l'accroche qui s'y trouvait ayant été retirée : deux textes qui se disputent le même panneau, et on ne lit ni l'un ni l'autre. C'est le seul moment de la journée où l'utilisateur n'a rien d'autre à faire que regarder ; la même consigne dans un manuel n'est jamais lue. Le tirage évite celui de la dernière fois, et l'effet respecte `prefers-reduced-motion` — qui a demandé à son système de calmer les animations reçoit la phrase entière, d'un coup. Les trente-deux conseils existent dans les quatre langues.

- **Le verrouillage de connexion devient progressif.** Dix échecs verrouillent l'adresse, et chaque nouvelle série coûte plus cher : 30 s, 1 min, 5 min, 15 min, 1 h — le dernier barreau se répétant ensuite. Trente secondes ne gênent presque pas un humain et divisent déjà par mille la cadence d'un automate ; les barreaux suivants achèvent de rendre l'exercice sans intérêt. Une heure de silence après la fin d'un verrou ramène l'échelle à son premier barreau, et une connexion réussie l'efface entièrement.

  **L'écran dit maintenant combien de temps il reste** au lieu de « identifiants incorrects » : quelqu'un qui ne sait pas qu'il est verrouillé réessaie, et allonge l'attente à chaque série.

### Modifié

- **L'écran de connexion s'allège.** Le bandeau supérieur — « portail sécurisé » et le témoin d'état — disparaît, l'accroche « Vos livres, sur votre machine » aussi, et le pied du formulaire ne porte plus que **la version installée**, lue au serveur. C'est la première chose qu'on demande dans un ticket de support, et la dernière qu'on trouve.

### Corrigé

- **Le barreau le plus long du verrouillage effaçait sa propre mémoire.** Le balayage des enregistrements ne regardait que l'inactivité : après une heure de verrou, la dernière tentative datait de plus d'une heure, l'enregistrement partait, et l'échelle repartait de trente secondes. Un automate n'aurait donc jamais dépassé la première minute. Trouvé par le test de l'échelle, pas à la relecture.

- **L'écran de mise à jour partait avant qu'on ait pu le lire.** Il portait « Ouverture de LedgerAlps dans 5 secondes… » et se remplaçait tout seul. Or c'est le seul moment où le produit dit « vos données comptables ont été conservées » — la phrase que quelqu'un qui vient de remplacer un logiciel de comptabilité veut lire avant tout. Cinq secondes ne suffisent pas à la lire, et personne ne peut la relire ensuite : le message part avec la page.

  L'écran **attend maintenant le clic**, comme un installeur Windows attend « Terminer ». Le bouton est un simple lien vers `/ok`, qui redirige vers l'application : le chemin ne dépend ni d'un script, ni d'une minuterie, ni de l'ordre dans lequel le navigateur exécute les choses. La page ne contient plus une ligne de JavaScript.

  **Le garde-fou** : si la fenêtre est fermée sans cliquer, une limite d'une demi-heure ramasse le lanceur — assez longue pour que personne ne la rencontre, assez courte pour ne pas laisser un processus orphelin. Le témoin de réinstallation étant effacé dès l'entrée, l'écran ne réapparaît pas au lancement suivant.

- **Un mot de passe faux rechargeait la page sans rien dire.** Le 401 de `/auth/login` déclenchait le mécanisme de session expirée : appel à `/auth/refresh`, échec, puis `window.location.href = '/login'` — un rechargement complet qui effaçait « identifiants incorrects » avant qu'on ait pu le lire. L'écran clignotait et rendait un formulaire vide, sans dire si l'on s'était trompé ou si le logiciel avait planté. Les routes d'authentification sont maintenant écartées de ce mécanisme : sur elles, un 401 veut dire « ce n'est pas le bon mot de passe », pas « votre session a expiré ».

### Modifié

- **L'icône du bureau garde son fond blanc.** Elle se pose sur un bureau, une barre des tâches, un fond dont on ne sait rien : un monogramme bleu nuit sur fond transparent y disparaîtrait. Le fond du fichier officiel est donc conservé et étendu au cadre carré de l'icône.

## [1.5.1] — 2026-08-14

### Ajouté

- **Assujetti ou non à la TVA : la question est posée, et elle a des conséquences.** Paramètres → Banque. Jusqu'ici, la seule trace de ce statut était la présence d'un numéro de TVA — un champ vide voulant dire deux choses opposées que rien ne distinguait. LedgerAlps appliquait donc 8.1 % par défaut à chaque ligne, **puis refusait la facture** : le mur arrivait après le travail, et celui qui n'est pas assujetti ne pouvait le comprendre qu'en lisant la LTVA.

  **Trois états, pas deux.** « Non déclaré » n'est pas « non assujetti » : le confondre ferait tomber à 0 % les lignes d'un assujetti qui n'a pas encore saisi son numéro. Tant que la question n'a pas de réponse, rien ne change.

  **« Non assujetti » efface le numéro de TVA**, et ce n'est pas une commodité : ce numéro s'imprime sur la facture, et l'y laisser produirait un document qui affirme le contraire de la fiche — précisément ce que la LTVA art. 27 al. 1 interdit et dont l'al. 2 rend redevable. Le garde-fou de facturation fait passer la déclaration avant le numéro, si bien qu'un numéro résiduel dans une base restaurée ne rouvre pas la porte.

  **Le refus change de phrase selon la cause.** « Aucun numéro n'est enregistré » envoie chercher un numéro ; à qui s'est déclaré non assujetti, il n'y en a pas à chercher, et le message le dit.

  **Les installations existantes qui portent un numéro de TVA** sont marquées assujetties par la migration : ce numéro ne s'obtient qu'en s'inscrivant au registre. Celles qui n'en portent pas restent non déclarées — deviner « non assujetti » corrigerait leur facturation en silence, sur une hypothèse.

- **La marque officielle, telle quelle.** Les fichiers fournis — `LOGO.svg` et `icon.svg` — vivent intacts dans `infrastructure/brand/`. Ce que l'interface affiche en dérive sans intervention manuelle : mêmes tracés, seul le cadrage change (le `viewBox` se cale sur le dessin au lieu de la planche de 1408 × 768) et le fond blanc plein cadre est retiré pour que la marque se pose partout. Les polices étant déjà vectorisées, l'espacement et le style sont dans les coordonnées et rien ne les altère.

  **Où elle apparaît.** Le logotype sur l'écran de connexion — le seul écran où personne n'est encore identifié, donc celui où il faut lire sans ambiguïté quel logiciel demande un mot de passe — et au pied de la barre latérale, à côté du numéro de version. Pas en haut : le haut appartient à l'entreprise de l'utilisateur, et lui disputer cette place serait malpoli. Le monogramme, lui, tient la vignette carrée de la barre quand aucun logo d'entreprise n'est défini.

- **L'icône du bureau est celle de LedgerAlps.** Le raccourci et le menu Démarrer affichaient l'icône générique bleue de Windows : `ledgeralps.exe` ne portait aucune ressource d'icône. Il en porte une désormais — sept tailles, de 16 à 256 px, rendues depuis `icon.svg`. L'installeur et le désinstalleur la portent aussi.

  **Produit sans outil externe.** Go n'a pas de directive pour poser une icône : il lie les fichiers `.syso` du répertoire du paquet. Les outils habituels se récupèrent sur le réseau, et LedgerAlps se construit hors ligne ; la ressource est donc écrite par un script du dépôt, documenté dans `infrastructure/brand/README.md`. Vérifié en extrayant l'icône de l'exécutable construit : `#1C3656` et `#CA2E24`, les deux couleurs exactes du fichier officiel — contre `#0C7CCB` pour un exécutable sans ressource.

- **Un fichier ajouté à `public/` est enfin servi.** Chaque fichier statique avait sa route en dur (`/favicon.ico`, `/logo.svg`). Le piège ne se voit pas à la lecture : une route absente ne répond pas 404, elle tombe dans le repli de l'application et rend `index.html` — le navigateur reçoit du HTML là où il attendait une image et n'affiche rien. C'est exactement ce qui est arrivé aux deux fichiers de la marque. Le serveur sert maintenant ce que le paquet embarqué contient réellement à sa racine.

- **Le logo d'entreprise est ramené à 300 × 300 px.** Il est stocké en base64 dans la fiche société, laquelle part dans les sauvegardes, dans l'archive légale et dans chaque réponse que la barre latérale demande à l'ouverture. Une photo de 4000 px y pesait des mégaoctets pour s'afficher à 32 px de haut.

  Le navigateur réduit l'image **avant** l'envoi — un logo de 2400 × 600 part en 300 × 75, 60 Ko deviennent 6 Ko — et le serveur refait le travail de son côté : la route reste ouverte à qui forge une requête, et c'est elle qui décide de ce qui entre en base. Les deux ne sont pas redondants : l'un sert le confort, l'autre tient la règle.

  **Le rapport est conservé** : un logo horizontal ne devient pas un carré écrasé. **Aucune image n'est agrandie** : un logo de 120 px reste à 120 px, l'étirer ne créerait pas de détail. Et l'écran annonce ce que le SERVEUR a retenu, pas ce que le navigateur a envoyé.

- **Une liste de mise en route sur le tableau de bord.** Une installation neuve s'ouvrait sur quatre compteurs à zéro et un graphique vide : rien n'annonçait que sans adresse structurée le bulletin QR serait refusé au guichet, ni que sans IBAN le PDF sortirait sans section de paiement.

  **L'information existait déjà** — le contrôle de cohérence la produit — mais dans Paramètres → Maintenance → Diagnostic, à trois clics d'un endroit où un débutant ne va jamais. Ce n'était pas un manque de fonction, c'était un manque de placement.

  Cinq étapes : raison sociale et adresse, numéro IDE, IBAN, premier client, première facture. Chaque ligne mène **au champ**, pas à l'écran : `/settings#banking` ouvre l'onglet Banque, et les onglets de Paramètres sont désormais atteignables par ancre.

  **Ce qui bloque est nommé** : « il manque le code postal, la localité », pas « quelque chose est incomplet ». Un IBAN présent mais dont la clé de contrôle est fausse ne coche pas la case — il est pire qu'absent, puisqu'il produit un bulletin d'apparence normale que la banque refusera.

  **Rien n'est mémorisé.** L'état se relit des données à chaque ouverture : effacer l'IBAN décoche sa case et fait revenir la liste. Un assistant aurait retenu « fait » et menti à partir de là. La liste disparaît d'elle-même quand les cinq étapes sont faites, et ne s'affiche pas pour un compte en lecture seule — qui ne peut en accomplir aucune.

- **Un « i » à côté du titre de chaque écran.** Une phrase sous chaque titre aide le premier jour et encombre les mille suivants. La bulle s'ouvre au survol, s'épingle au clic, se ferme par Échap ou un clic ailleurs — le clic n'est pas un luxe : au doigt comme au clavier, il n'y a pas de survol.

  Sept écrans, et le texte dit ce que l'écran fait **et ce qu'il ne fait pas** là où la confusion est fréquente : qu'un brouillon ne compte ni à la balance ni au bilan, qu'un contact facturé s'anonymise au lieu de se supprimer, qu'on n'a rien à faire dans le plan comptable au quotidien.

- **Une attestation d'intégrité se vérifie maintenant** — Paramètres → Maintenance → Conformité → « Vérifier une attestation ». Elle était produite, scellée, remise à un tiers… qui n'avait aucun moyen de la contrôler. Un document invérifiable ne vaut pas mieux qu'une affirmation orale.

  **Trois contrôles, de valeur inégale, et l'écran le dit.** Le SCEAU détecte un fichier retouché à la main, rien de plus : qui a le logiciel recalcule l'empreinte. La CORRESPONDANCE est celle qui compte — l'empreinte de tête de l'attestation est comparée à celle que portent les livres aujourd'hui, au même maillon. L'ÉTAT ACTUEL reparcourt toute la chaîne.

  **Ce qui donne sa force au contrôle n'est pas cryptographique, c'est la garde du fichier.** Une attestation remise en janvier et conservée par la fiduciaire prouve, en juin, qu'aucune écriture couverte n'a été réécrite — parce que le client ne peut plus modifier la copie qu'elle détient.

  **La marche à suivre est écrite dans l'attestation elle-même** (section `how_to_verify`). Le sceau se recalcule avec `certutil` sous Windows ou `shasum` ailleurs : la fiduciaire n'a besoin d'aucun logiciel pour cette partie.

### Corrigé

- **Le journal de l'installeur affichait du charabia.** « Op‚ration r,ussieÿ: le processus […] a ‚t, arr^t,. » — et, juste en dessous, « Erreurÿ: le processus est introuvable », qui est le cas normal quand l'application n'est pas lancée.

  **Deux fautes en une.** `nsExec::ExecToLog` reversait dans le journal ce que `taskkill` écrit sur sa console. Cette sortie est dans la page de codes CONSOLE — CP850 sur un Windows français — que NSIS relit comme de l'ANSI : « é » devient « ‚ », « ê » devient « ^ », et l'espace insécable avant les deux-points ressort en « ÿ ». Et transcoder n'aurait pas suffi : ces lignes n'apprennent rien, et l'une d'elles alarme pour un état parfaitement normal.

  L'installeur ne recopie plus la sortie d'un programme tiers. Il lit le code de retour — `0` arrêté, `128` pas lancé, les deux étant des succès — et écrit ses propres phrases, **dans les quatre langues**. Seul un échec réel (accès refusé) produit désormais une ligne.

  **L'arrêt de l'application a aussi changé de place.** Il était dans `.onInit`, c'est-à-dire au lancement de l'installeur : LedgerAlps était fermé de force chez quelqu'un qui n'avait encore rien accepté et pouvait annuler à la page de licence. Il est maintenant en tête de la section d'installation, une fois celle-ci confirmée — et toujours avant l'écriture des fichiers.

- **Le reste de l'installeur parlait anglais** sur un système français, allemand ou italien : le bouton « Launch LedgerAlps » de la dernière page, les trois lignes de fin d'installation, l'info-bulle des raccourcis et le raccourci « Uninstall LedgerAlps » du menu Démarrer. Tout est traduit ; l'ancien raccourci anglais est effacé à la mise à jour pour ne pas laisser les deux côte à côte.

- **Deux textes de l'onglet Banque étaient écrits en dur en français** — l'aide du numéro de TVA et celle du paiement par virement. Ils s'affichaient tels quels sur un écran allemand.

- **« Créer en brouillon » affichait « Quitter cette page ? »** au lieu d'enregistrer. Le garde de saisie protégeait le formulaire contre une navigation — y compris celle qui suit l'enregistrement réussi. Le message était faux, la facture existait ; et qui le croyait cliquait « Annuler » et restait bloqué sur un écran dont le bouton principal semblait ne rien faire. Le garde se désarme désormais juste avant de naviguer.

- **La facture PDF affichait « NÂ° facture: » et « Ã‰chÃ©ance: ».** `metaRow` écrivait ses arguments sans les convertir : gofpdf attend du cp1252 pour ses polices de base, et l'UTF-8 passait tel quel. Corrigé au point de passage — la fonction convertit, pas ses vingt appelants.

  **Le test qui manquait.** Il en existait un contre le DOUBLE encodage ; aucun contre son ABSENCE. Ma première tentative cherchait les octets dans le fichier brut et passait au vert alors que le défaut était réintroduit exprès : les flux d'un PDF sont compressés. Le test décompresse maintenant, et sa capacité à échouer a été vérifiée.

- **Les critères du mot de passe temporaire et le sous-titre de Maintenance** s'affichaient en français, l'un en dur, l'autre en montrant la clé brute (`mt.conformiteHint`).

- **Le verdict de vérification se contredisait sur une chaîne vide** — cadre rouge, ligne « empreinte divergente », sous la phrase « Attestation vérifiée ». L'écran recalculait « c'est bon » par un ET des trois booléens, alors qu'une attestation émise sur des livres vides n'a aucune empreinte à faire correspondre. C'est le serveur qui tranche désormais : il rédige la phrase, il donne la couleur, et le contrôle sans objet s'affiche comme tel plutôt qu'en rouge.

- **`Lauschadresse`** — l'allemand disait « adresse d'écoute » au sens de l'écoute clandestine. C'est `Netzwerkadresse`.

### Modifié

- **Le menu « Facturation » devient « Ventes »**, en face d'« Achats ». Il nomme ce qu'on y fait, pas l'outil qui le fait, et les deux se lisent en paire.

## [1.5.0] — 2026-08-12

### Ajouté

- **Le serveur et les documents parlent aussi les quatre langues.** Il ne reste plus un mot de français sur une interface allemande : les 360 phrases que LedgerAlps adresse à l'utilisateur — refus, confirmations, facture PDF, bulletin QR, exports CSV, attestation d'intégrité — suivent le sélecteur.

  **Les documents suivent le SÉLECTEUR, pas la fiche du client.** Ce qu'on voit à l'écran est ce qui sort de l'imprimante. La facture PDF change de titre (`RECHNUNG`, `FATTURA`, `INVOICE`), d'en-têtes de colonnes, de libellés du bulletin de versement — et le vocabulaire du bulletin est celui des Implementation Guidelines de SIX, que la banque attend au mot près.

  **La traduction se fait au POINT DE PASSAGE, pas aux deux cents endroits qui refusent.** Un intercepteur relit les réponses JSON en sortie ; le catalogue est indexé par la phrase française elle-même. Trois conséquences : une route ajoutée demain est couverte sans qu'on y pense, les journaux du serveur et la trentaine de tests qui comparent les messages au caractère près ne bougent pas, et une phrase absente du catalogue ressort en français — ce que l'utilisateur voyait déjà, jamais une clé nue.

  **Un refus né dans la comptabilité traverse traduit** sans que le service connaisse la langue : « aucun numéro de TVA n'est enregistré » devient « Es ist keine MWST-Nummer hinterlegt ». Porter la langue de la requête jusqu'au calcul d'une écriture aurait mélangé deux choses qui n'ont rien à voir.

  **Les valeurs restent à leur place.** « le compte 1020 est désactivé » retrouve son moule et rend « Konto 1020 ist deaktiviert ». Un test vérifie que les quatre langues d'une phrase portent exactement les mêmes verbes de format, dans le même ordre : un verbe déplacé afficherait le montant à la place du numéro de compte — une phrase parfaitement lisible et fausse.

  **L'anglais qui atteignait l'utilisateur est corrigé au passage** : « journal entry not found », « invalid status transition », « missing or malformed Authorization header » s'affichaient tels quels sur un écran français.

  **L'installeur Windows parle allemand et italien**, en plus du français et de l'anglais.

  **Le garde-fou.** Un test relit les sources, retrouve tout ce qui part vers l'utilisateur, et échoue sur ce qui n'est pas au catalogue. Sa première version devinait « est-ce du français ? » d'après les accents : « identifiants incorrects » n'en a aucun, et le message le plus vu du produit est passé à travers en silence. La règle est désormais inversée — tout est à traduire, et les vingt exceptions de diagnostic sont déclarées une par une, avec leur raison.

- **L'interface est traduite à 100 % en allemand, italien et anglais (UK).** Les 36 écrans, du tableau de bord au chiffrement de la base : 998 clés dans les quatre langues. L'avertissement « Traduction en cours » a disparu du sélecteur — il n'a plus d'objet.

  **Ce qui suit la langue en plus des mots.** Un texte traduit posé sur des formats suisses reste à moitié étranger :

  - **Les dates** — `11.08.2026` en Suisse, `11/08/2026` pour un Britannique.
  - **Les montants** — `1'500.00 CHF` avec l'apostrophe suisse, `1,500.00 CHF` en anglais.
  - **Les noms de mois** du tableau de chiffre d'affaires viennent d'`Intl`, non d'une liste française en dur : « mars 2026 » devient « März 2026 » sans qu'il y ait quatre listes à tenir.
  - **Les badges de statut** — `Brouillon` / `Entwurf` / `Bozza` / `Draft`. Ils étaient une table figée dans les utilitaires, donc invisible aux relectures d'écran, et s'affichaient en français sur chaque ligne de chaque liste.

  **Les avis de conformité aussi.** Ils viennent du serveur, qui ne les rendait qu'en français et en anglais, et l'interface demandait toujours le français : un bandeau francophone barrait le haut de chaque écran allemand. Les quatre avis portent désormais les quatre langues, et l'écran demande la sienne. Un test refuse un avis auquel il manque une langue — le repli sur le français masquait l'oubli en donnant un affichage parfait.

  **Ce qui garantit que ça tienne.** `internal/frontend/i18n_test.go` relit les quatre catalogues et échoue si une clé manque, si une valeur est vide, si un repère d'interpolation a disparu, ou **si une valeur vaut encore le français**. Ce dernier point est celui qui compte : copier `fr.ts` en `de.ts` compile parfaitement et produit une interface allemande entièrement en français.

  **Ce qui n'est PAS traduit, et pourquoi.** Les messages de refus du serveur — « aucun numéro de TVA n'est enregistré : vous ne pouvez pas facturer de TVA » — restent en français, ainsi que les factures PDF, les exports CSV et l'attestation d'intégrité. Ce sont **97 messages et quatre familles de documents**, un lot distinct : la langue d'une facture PDF doit suivre le CLIENT, pas l'interface de celui qui l'émet.

- **Facturation et Contacts sont traduits** dans les quatre langues — listes, onglets, filtres, en-têtes de colonnes, états vides. La couverture passe de 10 à **13 %** (148 clés).

  **La phrase de lecture seule était une constante figée en français.** Elle s'affichait sur sept écrans par ailleurs traduits : c'est devenu une clé. Le motif est instructif — une chaîne sortie d'un composant pour être partagée échappe à la traduction précisément parce qu'elle n'est plus dans un écran.

  Les **pluriels comptés** passent par le catalogue : « 3 documents » se dit « 3 Dokumente », et le zéro suit la règle de chaque langue — le français le met au singulier, les trois autres au pluriel.

- **Le sélecteur de langue est livré — FR / DE / IT / EN**, dans Paramètres → Mon compte, **visible pour tous les rôles**. La langue n'est pas un réglage d'administration : une fiduciaire tessinoise à qui l'on ouvre les livres en lecture seule doit pouvoir lire en italien sans demander la permission à personne.

  **Pas besoin de se reconnecter** : le catalogue est embarqué, le changement est immédiat. La préférence survit à la déconnexion — sinon il faudrait se connecter en français pour pouvoir choisir l'italien.

  **Ce qui est traduit** : la navigation, les bandeaux de rôle, l'écran de connexion en entier, le second facteur, les statuts et le vocabulaire comptable — soit 111 clés dans les quatre langues. **Environ 10 % des chaînes de l'interface** ; les écrans eux-mêmes suivent, un par un.

  **Le panneau le dit lui-même** plutôt que de laisser découvrir : un avertissement « Traduction en cours » y explique où en est la couverture. Il disparaîtra quand elle sera complète.

  **Les abréviations légales basculent aussi** : le pied de page passe de « CO · nLPD » à « OR · DSG » en allemand, « CO · LPD » en italien, « CO · FADP » en anglais. Vérifié dans les quatre langues sur un serveur réel.


### Corrigé

- **Les messages du serveur parlent français.** « fiscal year "2025" is closed: no entry can be created or posted at 2025-12-01 » devient « l'exercice « 2025 » est clôturé : aucune écriture ne peut y être créée ni comptabilisée au 2025-12-01. Passez la correction dans l'exercice ouvert (CO art. 958f) ».

  **82 messages** traduits : l'anglais qui atteignait l'utilisateur — « invalid credentials », « supplier invoice not found », les quinze variantes de « must be YYYY-MM-DD », les pannes internes qui remontaient telles quelles — **et le français sans accents** trouvé au passage : « aucune facture selectionnee », « la date d'execution doit etre au format », « l'IBAN de votre entreprise n'est pas renseigne (Parametres -> Entreprise) ».

  Les erreurs **internes** restent en anglais — « begin transaction », « applying migration ». Elles vont au journal du serveur et servent au diagnostic ; les traduire n'aiderait personne et empêcherait de retrouver un message dans les sources.

### Ajouté

- **Les fondations de la traduction en allemand, italien et anglais.** Le sélecteur de langue **n'est pas encore visible** : un sélecteur qui traduit un tiers de l'écran est une maquette, et sur un logiciel comptable une étiquette non traduite à côté d'un champ de TVA est pire qu'une interface unilingue.

  **[docs/GLOSSAIRE.md](docs/GLOSSAIRE.md)** fixe d'abord le vocabulaire — à valider avant toute traduction d'écran. Une partie du texte n'est pas de l'habillage : « extourne », « impôt préalable », « exercice clôturé » désignent des notions définies par une loi qui existe dans les trois langues officielles. Deux pièges y sont documentés : une facture annulée se dit *storniert* et non *annulliert* — *annullieren* voudrait dire effacer, ce que le CO art. 958f interdit — et *Soll/Haben* n'est pas *Belastung/Gutschrift*, qui désigne des mouvements bancaires. Les abréviations légales basculent aussi : CO devient OR, LTVA devient MWSTG, nLPD devient DSG.

  **Pas de bibliothèque d'internationalisation.** Quatre langues connues à la compilation, un catalogue embarqué, des phrases simples : cent lignes suffisent, et une dépendance se paie en mises à jour et en surface d'attaque. Le revers est assumé et écrit — pas de pluriels slaves ni de formats ICU.

  **Quatre tests refusent une traduction bâclée** : clés identiques d'un catalogue à l'autre, aucune valeur vide, aucune valeur restée en français — avec la liste explicite des termes identiques par nature, comme IBAN ou Saldo — et les repères d'interpolation conservés. Vérifiés en introduisant les trois fautes classiques : ils les attrapent et les nomment.

  Les **formats** suivent la langue : `de-CH` et `fr-CH` séparent les milliers par une apostrophe là où `en-GB` met une virgule, et la position du symbole monétaire change.


### Ajouté

- **Retirer une facture de la liste des paiements.** La liste accumulait tout ce qui est comptabilisé et non réglé — factures payées hors LedgerAlps, saisies d'essai, doublons — et surtout les **factures bloquées**, celles qui ne peuvent pas être payées faute de QR-IBAN ou de référence valide : rien ne les en faisait jamais sortir. Une liste qu'on ne peut pas vider cesse d'être lue, et le jour où elle porte une vraie facture en retard, personne ne la voit.

  **Supprimer n'était pas la réponse.** Une facture comptabilisée est une charge dans les livres, avec sa dette au compte créanciers ; l'effacer ferait disparaître la charge d'un exercice tenu, et le CO art. 958f impose de conserver la pièce dix ans. Le geste est donc l'**annulation avec extourne** : la facture reste, marquée annulée, et une écriture inverse neutralise la charge et la TVA déductible. Un réviseur voit une correction, pas un trou.

  Par lot ou une par une, **réservé à l'administrateur et au comptable**. Chaque facture reçoit son propre verdict — un brouillon est supprimé, une facture comptabilisée est extournée, une facture déjà réglée est refusée parce que l'argent est parti — et le détail est rendu ligne par ligne.

- **L'attestation d'intégrité s'émet seule**, au démarrage puis chaque jour, dans `attestations/` à côté des sauvegardes. La chaîne d'empreintes rend une modification détectable **à condition d'avoir un point de comparaison** : qui peut écrire dans la base peut recalculer la chaîne entière, qui reste alors cohérente. L'ancrage est l'empreinte de tête conservée ailleurs, à une date connue — et une garantie qui suppose qu'on pense à cliquer chaque mois n'existe pas. Le fichier part avec les sauvegardes vers le NAS ou la clé USB ; c'est ce déplacement qui vaut ancrage. Rien n'est envoyé nulle part.

- **[docs/REVUE-IA.md](docs/REVUE-IA.md)** — le mode d'emploi et le prompt pour faire auditer le code par un réviseur IA depuis une copie locale, avec le format de rapport attendu pour que les constats se corrigent sans allers-retours.

### Corrigé

- **Annuler une facture comptabilisée ne vidait pas les livres.** Le statut passait à « annulée » et rien d'autre : l'écriture restait, la charge et la TVA déductible continuaient d'alimenter le résultat et la déclaration. L'écart ne se serait vu qu'au décompte trimestriel, des mois plus tard, sans rien qui pointe vers sa cause. Neuf tests portent désormais sur les **soldes des comptes**, pas sur le statut — c'est le seul endroit où le défaut se voyait.

- **Le journal d'audit ne couvrait que trois actions.** La chaîne du CO art. 957a existait, avec sa vérification et son attestation ; y entraient la comptabilisation d'une écriture, la clôture d'un exercice et le changement de statut d'une facture. Les constantes prévues pour les contacts, les paiements et les rapprochements étaient déclarées et jamais appelées. Un journal à trous est pire qu'un journal absent : l'absence d'une ligne se lit comme « cela n'a pas eu lieu » alors qu'elle veut dire « cela n'a jamais été écrit ». Les factures fournisseurs — création, modification, comptabilisation, annulation, suppression — et les changements de coordonnées de l'entreprise y entrent maintenant, **refus compris**.


### Sécurité

- **Un compte en lecture seule créait une facture depuis le tableau de bord.** Le bouton « Nouvelle facture » y était un simple lien : aucun appel de mutation dans le fichier, donc absent du recensement qui avait servi à fermer les autres écrans. Chercher les commandes une par une revient à refaire la recherche à chaque écran ajouté, avec la même chance de se tromper.

  **La barrière porte désormais sur l'ADRESSE.** `/invoices/new` et `/invoices/:id/edit` n'existent que pour écrire : elles sont enfermées dans une garde de route qui renvoie au tableau de bord, quel que soit le chemin emprunté — bouton oublié, lien collé, favori, bouton « précédent ».

  **Et trois tests parcourent les sources** pour que l'oubli suivant échoue en intégration continue, pas chez vous : les routes d'écriture sont bien gardées, tout écran qui mène vers l'une d'elles consulte les permissions, et le miroir des rôles existe encore. Le premier test a été vérifié en réintroduisant le défaut : il l'attrape et nomme le fichier.

  Balayage complet des neuf écrans avec un compte en lecture seule : **aucun lien vers une route d'écriture, aucun champ de fichier, aucun champ modifiable** hors recherche, filtres et sélection de documents à télécharger.

- **La lecture seule est désormais une lecture seule à l'écran aussi.** Le serveur refusait déjà toute écriture à ce rôle — un filtre global rejette toute méthode autre que GET, HEAD et OPTIONS, quelle que soit la route. Mais les commandes restaient affichées et cliquables : on remplissait un formulaire entier avant d'apprendre qu'il ne servirait à rien, et la barrière passait pour cosmétique alors qu'elle ne l'est pas.

  **Ce qui disparaît de la page** — pas grisé, pas caché par du style : absent du document, donc rien à réactiver depuis la console. Dépôt d'une facture fournisseur (PDF ou image), import d'un relevé bancaire camt.053, production d'un ordre de paiement pain.001, dépôt et suppression du logo de société, création de facture, d'offre, de contact et d'écriture au journal, modification et comptabilisation d'une facture reçue, rapprochement bancaire.

  **Ce qui reste ouvert** : tout ce qui se consulte et tout ce qui s'exporte — journal, grand livre, balance de vérification, PDF des factures, et **l'archive légale des dix ans** (CO art. 958f). C'est exactement ce qu'une fiduciaire vient chercher : lui fermer ces portes viderait le rôle de son sens.

  **Les formulaires sont neutralisés par `fieldset`**, ce qui désactive nativement chaque champ, liste et bouton qu'ils contiennent — y compris ceux qu'on y ajoutera plus tard. Désactiver champ par champ fonctionne le jour où on l'écrit, puis se périme au premier champ ajouté sans y penser.

  **Trois tests nomment les routes une par une** et vérifient les trois faces de la règle : un lecteur reçoit 403 sans que le gestionnaire tourne, un comptable les atteint toutes, et un lecteur télécharge bien ses exports et son archive. Le filtre global les couvrait déjà par construction ; les nommer dit lesquelles ne pourront jamais faire l'objet d'une exemption.

  Vérifié dans un navigateur avec un vrai compte en lecture seule : aucun champ de fichier dans les pages Achats, Rapports et Paramètres, aucun champ modifiable, et **quatre dépôts forgés à la main — en contournant l'interface avec le jeton de la session — tous refusés en 403**.

### Modifié

- **La veille de conformité nomme les articles dont LedgerAlps dépend.** Elle surveillait la nLPD comme un tout : quand la date de consolidation bouge, l'alerte disait « la nLPD a changé » — ce qui n'oriente personne vers ce qu'il faut relire dans un acte qui compte des dizaines d'articles.

  Le registre porte maintenant, pour la nLPD, les **articles 6, 8, 25, 28 et 32** — relevés dans le code et la documentation, pas choisis de mémoire — chacun avec ce qui en dépend concrètement. L'alerte les affiche.

  **L'art. 6** (licéité, proportionnalité, finalité, exactitude ; al. 4 destruction ou anonymisation dès que les données ne sont plus nécessaires) couvre l'anonymisation des contacts, la purge des adresses IP, la minimisation des états du journal d'audit — et la finalité de toute trace d'activité : traçabilité comptable et sécurité, jamais mesure du comportement des personnes.

  Aucun avis n'est ajouté à l'écran : ces obligations sont satisfaites, et un bandeau toujours faux use la confiance dans ceux qui ne le sont pas.

### Écarté

- **Le Gestionnaire d'identification de Windows pour le coffre à secrets.** Le réflexe d'administration est d'y ranger les secrets d'une application ; la question méritait d'être posée, la réponse est non. Il est lui-même protégé par DPAPI et scellé au même compte : aucun gain de protection, la frontière ne bouge pas. Il introduit en revanche un mode de défaillance nouveau et irréversible — une entrée visible dans un panneau du système se supprime par mégarde, et la clé d'une base chiffrée n'existe qu'à un seul endroit. Enfin elle ne suivrait pas les données lors d'une copie du dossier. LedgerAlps garde donc le fichier scellé par DPAPI, posé à côté de la base. La raison est écrite en tête de `internal/core/secretstore`.

### Modifié

- **Le vocabulaire du second facteur parle d'OTP, plus de téléphone.** « Inscrire mon téléphone » devient « Activer le 2FA » ; l'écran d'activation demande une « application d'authentification 2FA/OTP — sur téléphone ou sur ordinateur » ; le secours à la connexion s'appelle « Application indisponible ? » et non « Téléphone perdu ? ». Le code se calcule aussi bien dans KeePassXC sur un poste fixe, et nommer l'appareil plutôt que la fonction laissait croire qu'un téléphone était nécessaire.


### Ajouté

- **La lecture d'une facture déposée remplit maintenant tout ce que le document porte** : montant, fournisseur, IBAN et référence depuis le QR ; **numéro de facture, date, échéance, taux de TVA et numéro IDE** depuis la couche texte du PDF. Vérifié sur une facture réelle — un rappel d'ePost Service AG.

  **Chaque valeur est annoncée avec l'étiquette qui l'a produite** — « n° 538690 (« Numéro de facture ») · échéance 2025-12-31 (« Échu ») ». Un champ pré-rempli dont on voit la provenance se corrige ; un champ pré-rempli anonyme se croit. Comme une facture fournisseur entre dans les livres *et* dans la déclaration de TVA, c'est la différence entre une aide et un piège.

  **Sans mention de TVA, le taux est 0 %** et le montant du QR est aussi le montant hors taxe : l'écriture ne porte alors aucune ligne de TVA déductible, et il n'y a rien à récupérer (LTVA art. 28 al. 1 exige une facture mentionnant l'impôt pour le déduire). Le piège évité : le numéro d'assujetti d'un fournisseur contient les lettres « MWST » ou « TVA » — chercher ces mots ferait croire à de la TVA sur une facture qui n'en porte aucune, et gonflerait l'impôt préalable déclaré. On cherche un **taux**, jamais un mot.

  Le **QR-IBAN est rangé dans le bon champ** de la fiche fournisseur : une référence QR n'est acceptée qu'avec lui (SIX IG v2.4 §4.2.2), et les confondre fait rejeter le virement.

### Corrigé

- **Une QR-facture remplit le QR-IBAN, et laisse l'IBAN vide.** Un IBAN et un QR-IBAN ne sont pas la même chose : une référence QR n'est acceptée qu'avec le second, une Creditor Reference qu'avec le premier (SIX IG v2.4 §4.2.2). La fiche du fournisseur ouverte par le scan porte donc **deux cases distinctes**, et le compte lu sur le bulletin va dans la sienne — décidé sur le code de clearing, positions 5 à 9, plage 30000–31999. Une valeur mise dans la mauvaise case est reclassée avant l'enregistrement, et l'écran le dit plutôt que de la faire refuser après coup.

- **Un QR-IBAN saisi dans le champ « IBAN » de votre entreprise produisait un bulletin invalide.** Les réglages ne proposent qu'une case, et le champ QR-IBAN dédié ne se remplit que par variable d'environnement : qui possède un compte QR le saisissait donc là où il y a de la place. LedgerAlps émettait alors une référence `NON` sur un QR-IBAN — appariement que les banques rejettent. Le compte est maintenant reconnu pour ce qu'il est, la référence QRR est produite, et l'écran annonce ce qui sera imprimé. Quatre tests tiennent la règle, dont le cas d'une facture en euros, où QRR n'existe pas.

- **Un fournisseur payable était annoncé « sans IBAN ».** Le QR-IBAN se range dans son propre champ — une référence QR n'est acceptée qu'avec lui, et le confondre avec un IBAN ordinaire fait rejeter le virement. Mais la liste des fournisseurs ne regardait que l'IBAN ordinaire : tout fournisseur lu depuis une QR-facture se présentait comme impayable, alors que l'ordre de virement partait très bien. L'écran démentait le produit, ce qui coûte plus cher qu'un bouton cassé : on renonce à s'en servir.

- **Le dialogue de comptabilisation annonçait une TVA déductible sur une facture qui n'en porte pas.** « Charge et TVA déductible au débit », « la TVA payée entre dans votre déclaration (chiffre 400) » : sur une facture à 0 %, il décrivait une écriture qui n'allait pas être passée. Un dialogue de confirmation qui dit autre chose que ce qui va se produire ne protège plus de rien — on le lit une fois, puis on l'ignore. Il suit maintenant le montant de TVA de la pièce.

- **Le QR-IBAN était enregistré sans jamais être montré.** La fiche du contact n'affichait que l'IBAN ordinaire. Le compte sur lequel les paiements allaient partir était donc invisible, et ce qui est invisible ne se corrige pas. La fiche porte maintenant les deux champs, avec ce qui les distingue.

- **Deux lectures de travers, invisibles à l'œil, sur un rappel.**

  *La date du rappel était prise pour celle de la facture.* Le document porte deux dates étiquetées « Date » : celle du rappel en tête de page, celle de la facture dans le tableau. On obtenait une pièce dont le numéro et la date ne se rapportaient pas au même document — un défaut qui ne se voit qu'au rapprochement. La lecture porte maintenant sur la **ligne** : le numéro, sa date et son échéance viennent de la même rangée.

  *Les colonnes de nombres, alignées à droite, décalaient toute la ligne d'un cran.* Leur abscisse tombe sous l'en-tête *précédent* : le montant se rangeait sous « Devise », l'échéance sous « Montant ». Chaque valeur restait juste, chaque étiquette était fausse. L'appariement se fait désormais par rang, et un contrôle de cohérence — un nombre sous « Montant », trois lettres sous « Devise » — refuse l'assignation décalée plutôt que de la livrer.


### Corrigé

- **Un chargement qui ne finissait jamais, en bas des Paramètres.** Les réglages réseau affichaient un rond qui tourne tant que le formulaire n'était pas chargé — or une requête refusée (403) ne le charge jamais. Sur un compte comptable, l'écran restait donc indéfiniment sur « chargement », soit le pire des états : il annonce que quelque chose arrive.

- **« Comptes et rôles » s'affichait pour le comptable et la lecture seule**, sous un titre qui promettait le contraire de ce que le serveur répondait. La section **Sécurité & réseau** entière est désormais réservée à l'administrateur — clé de signature, adresse d'écoute, réglages de session, comptes.

- **Le second facteur nommait « administrateur » quel que soit le rôle.** Un comptable lisait une phrase fausse sur son propre écran : « Étant administrateur, vous devrez… ». Le message suit maintenant le rôle. Une phrase fausse dans un produit use la confiance dans tout ce qu'il affirme par ailleurs.

- La note « la console de rejeu ISO 20022 et le mode bac à sable arrivent dans une prochaine version » est retirée : elle n'était plus d'actualité.

- **Le rendu des Paramètres lisait `tab` là où le recalage écrivait `effectiveTab`.** Un onglet interdit atteint par un lien affichait donc son panneau malgré tout, dont chaque appel répondait 403.

### Ajouté

- **Onglet « Mon compte »**, visible de tous les rôles : second facteur et ordinateurs de confiance. Il vivait dans la section Sécurité, réservée à l'administrateur — un comptable, à qui le second facteur est pourtant **exigé**, ne pouvait donc pas l'atteindre. Ce qu'on règle pour soi-même n'est pas de l'administration du logiciel.

- **Le montant d'une facture fournisseur se saisit en TTC ou en hors taxe**, au choix, TTC par défaut. Une facture reçue annonce ce qu'il faut **payer**, et c'est aussi ce que porte le QR : exiger du hors taxe demandait une division à chaque saisie, et empêchait le QR de rien pré-remplir.

  **Une facture sans TVA** — fournisseur non assujetti — se saisit à 0 % : les deux montants sont alors égaux, l'écriture ne porte aucune ligne de TVA déductible, et il n'y a rien à récupérer (LTVA art. 28 al. 1 exige une facture mentionnant l'impôt pour le déduire). L'écran le dit, plutôt que de laisser chercher un taux qui n'existe pas.

- **Le QR remplit maintenant le montant et propose le fournisseur.** Le montant TTC part directement dans le champ ; si le créancier n'est pas encore enregistré, sa fiche s'ouvre **pré-remplie** avec son nom et son IBAN — et un QR-IBAN est rangé dans `qr_iban`, un IBAN ordinaire dans `iban`, parce que les confondre fait rejeter le virement.


### Corrigé

- **La liste des factures fournisseurs répondait « n'ont pas pu être lues ».** Deux colonnes avaient été ajoutées à la requête sans l'être au balayage des résultats — dix-huit colonnes lues pour seize destinations. L'écran Achats restait donc vide, et le bouton « Comptabiliser », qui n'apparaît que sur un brouillon, avec lui. Le message d'erreur disait « database error », ce qui rend un décalage de colonnes indiscernable d'une base injoignable ; il nomme désormais la cause.

- **L'interface cassait entièrement sur l'écran Paramètres** (React #310, « Rendered more hooks than during the previous render »). Un `useAuthStore` était appelé **après** un `if (isLoading) return null` : au premier rendu la page sortait avant lui, au second elle l'appelait — un hook de plus que la fois précédente, ce que React refuse. La trace minifiée ne désignait rien d'exploitable.

- **Les factures n'étaient visibles que par leur auteur.** Un comptable ne voyait aucune facture émise par l'administrateur, et le total du tableau de bord contredisait la liste. C'est le même filtre `created_by_id` que celui retiré du journal en v1.4.8 — troisième fois que ce motif produit un défaut. Les factures sont les pièces de l'entreprise, pas la boîte de réception de qui les a saisies ; ce qui borne l'accès est le rôle.

- **Neuf gardes lisaient le drapeau administrateur DU JETON**, pas le rôle en base : attestation d'intégrité, journal d'audit (trois routes), anonymisation, exercices comptables (trois routes), contacts désactivés. Le défaut que le contrôle des droits avait été construit pour supprimer subsistait dans les handlers — rétrograder quelqu'un le laissait agir jusqu'à l'expiration de son jeton. La permission est désormais déclarée sur la route ; deux tests vérifient qu'elle y est, sans quoi retirer le middleware ouvrirait ces routes à tout compte connecté.

- **`server.log` répétait à chaque démarrage** « If you're reading this, you're unnecessarily importing github.com/ncruces/go-sqlite3/embed ». Le paquet est déprécié et sa seule action est d'écrire cette ligne ; le binaire WebAssembly de SQLite vient maintenant du module dédié. L'import est retiré — pas le message masqué.

- **Le tableau de bord comptait les clients désactivés** parmi les « clients actifs ». Désactiver un contact n'avait donc aucun effet visible sur le chiffre. Un contact à la fois client et fournisseur compte désormais des deux côtés, au lieu de disparaître des deux.

- Les halos décoratifs de l'écran de connexion sont retirés.

- Le chemin des données dans le README nomme le dossier complet — `C:\Users\<vous>\AppData\Roaming\LedgerAlps\` — plutôt que la seule variable `%APPDATA%`.

### Modifié

- **Le rôle comptable fait désormais tout, sauf la sécurité du logiciel et les comptes utilisateurs.** Il ne pouvait ni clôturer un exercice, ni vérifier la chaîne d'empreintes, ni prendre une sauvegarde, ni répondre à une demande d'effacement, ni régler la fiche entreprise — il devait demander à quelqu'un dont le rôle est de gérer des mots de passe.

  La frontière est maintenant explicite : **`manage` administre la comptabilité, `admin` administre le logiciel et qui y accède.** Restaurer une sauvegarde, la politique de chiffrement, le réseau, la clé de signature, le journal de sécurité et les comptes restent à l'administrateur. Inventaire complet dans [docs/DROITS.md](docs/DROITS.md).

- **Le second facteur est exigé du comptable, plus seulement de l'administrateur.** Un mot de passe volé sur un compte qui écrit dans les livres permet de fabriquer une comptabilité. La **lecture seule en est dispensée** : elle ne peut rien modifier, et c'est le rôle qu'on donne à sa fiduciaire — à qui l'on ne dicte pas son équipement.

### Ajouté

- **« Se souvenir de cet ordinateur », trente jours.** Redemander un code chaque jour sur le poste habituel n'ajoute presque rien à la protection — quelqu'un qui a déjà la main sur la machine n'attend pas la prochaine connexion — et une protection vécue comme une brimade finit désactivée.

  La case est **décochée par défaut** : lever une protection doit être un geste conscient. La date d'expiration est **absolue** — se connecter ne la prolonge pas, sans quoi un poste utilisé chaque semaine ne redemanderait plus jamais de code. Le jeton est haché en base, le navigateur en garde l'unique copie dans un cookie HttpOnly `SameSite=Strict`. Changer de mot de passe, retirer ou réinscrire son second facteur oublie tous les postes, et un écran permet de les oublier depuis un autre ordinateur.

- **[docs/DROITS.md](docs/DROITS.md)** — inventaire de tout ce qui est lisible, modifiable et cliquable, par section et par rôle.

- **Roadmap 13b : lecture automatique des factures fournisseurs.** Trois voies étudiées, toutes locales et libres. Recommandation : décoder le **QR-facture** en premier (`gozxing`, Apache 2.0) — il porte déjà créancier, IBAN, montant et référence, sans aucune reconnaissance de caractères et sans dépendance native. Tout service d'extraction en ligne est écarté : une facture fournisseur contient l'IBAN et l'adresse d'un tiers.

### Ajouté

- **Lire le QR d'une facture fournisseur déposée** (roadmap 13b, voie 1). On dépose le PDF, LedgerAlps décode le QR-facture et pré-remplit le fournisseur, la référence de paiement et le numéro de la pièce.

  **Le QR plutôt que la reconnaissance de caractères** : le code contient déjà, en clair et sans ambiguïté, ce que les Implementation Guidelines SIX définissent champ par champ. Rien n'est deviné — et sur un montant, une décimale devinée de travers entre dans les livres *et* dans la déclaration de TVA. Le contrôle de cohérence référence QR ⇔ QR-IBAN (IG v2.4 §4.2.2, champs 4 et 28) est appliqué à la lecture : un bulletin incohérent est signalé avant de servir à un paiement, plutôt que rejeté par la banque.

  **Rien n'est enregistré.** La route lit et rend ; c'est l'utilisateur qui confirme. Un champ pré-rempli qu'on relit vaut mieux qu'un champ juste qu'on n'a pas vu. Le fournisseur déjà connu est reconnu **par son IBAN** — un nom se saisit de dix façons, un compte non.

  **Rien ne sort de la machine.** Le fichier est lu en mémoire, ses images extraites dans un dossier temporaire effacé aussitôt. Aucun service d'extraction en ligne : une facture fournisseur porte le nom, l'adresse et l'IBAN d'un tiers. Deux dépendances, toutes deux en Go pur, `CGO_ENABLED=0` conservé — `gozxing` (Apache 2.0) et `pdfcpu` (Apache 2.0).

  **Ce qui n'est pas couvert, et le dit** : une facture sans QR, ou dont le QR est tracé en vecteurs plutôt qu'en image, répond « aucun QR trouvé » avec l'explication. Un scan photographié demanderait un rendu de page, donc une dépendance native — ce qui casserait le binaire unique. La saisie manuelle reste le chemin normal, jamais un échec silencieux.

- **Une facture fournisseur au brouillon se modifie.** L'écran permettait de saisir et de comptabiliser, mais pas de corriger : une faute de frappe obligeait à supprimer et à tout ressaisir. Une facture **comptabilisée** reste immuable — son écriture est scellée (CO art. 957a), et la modifier ferait mentir le journal ; le refus le dit et renvoie vers l'écriture de correction. La condition « brouillon » est répétée dans l'écriture elle-même, pour qu'une comptabilisation survenue entre-temps n'écrase pas une pièce déjà scellée.

### Modifié

- **La désactivation manuelle d'un contact est retirée.** L'interrupteur n'apportait rien qu'on ne fasse mieux autrement : un contact qu'on ne veut plus voir s'**anonymise** (nLPD art. 6 al. 4), ce qui l'écarte des listes *et* efface ses données personnelles — un geste qui dit ce qu'il fait. Le serveur refuse aussi la demande, avec le message qui oriente : masquer le bouton sans fermer la route n'aurait rien fermé.

- `PATCH /invoices/:id` et les routes d'achat déclarent désormais leur permission, en plus du filtre global.

## [1.4.9] — 2026-08-06

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

  Le passage au code de secours est désormais un **bouton** — « Application indisponible ? Utiliser un code de secours » — et le champ change réellement de nature : longueur, clavier, casse, espacement, exemple affiché. Le message d'échec correspond à ce qui a été tenté, au lieu de parler de l'horloge d'un appareil à quelqu'un qui recopie un papier.

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

  **Dix codes de secours, montrés une seule fois, hachés en base.** Sans eux, la perte de l'application d'authentification enfermerait définitivement le dernier administrateur — plus personne ne pourrait créer de compte, restaurer une sauvegarde ni rendre le droit de le faire : le second facteur créerait la panne qu'il est censé prévenir. Ils sont hachés comme des mots de passe, ne servent qu'une fois, et leur usage est tracé.

  *À la première connexion après cette mise à jour, l'administrateur devra activer le 2FA avec une application d'authentification OTP avant de pouvoir travailler.* C'est voulu : une protection qu'on peut remettre à plus tard n'est jamais activée. Les autres rôles peuvent l'activer s'ils le souhaitent, sans y être contraints — un comptable écrit dans un journal chaîné et tracé, et n'a pas les clés de l'installation.

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
