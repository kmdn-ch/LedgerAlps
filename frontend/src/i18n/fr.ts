// Le catalogue français — la SOURCE.
//
// C'est la langue dans laquelle le produit a été écrit et pensé, et celle dont
// la terminologie réglementaire a été arrêtée en premier (docs/GLOSSAIRE.md).
// Les autres catalogues en dérivent par typage : une clé manquante ailleurs est
// une erreur de compilation.
//
// # Comment nommer une clé
//
// `zone.element` — la zone est l'écran ou le domaine, l'élément dit ce que la
// chaîne est, pas ce qu'elle contient. `achats.deposerPDF`, pas
// `achats.lireUnPDF` : le jour où le libellé devient « Importer une facture »,
// la clé reste juste.
//
// # Les repères
//
// `{nom}` entre accolades, jamais positionnels : l'ordre des mots change d'une
// langue à l'autre. Voir `interpole()`.
//
// # Ce catalogue n'est pas encore complet
//
// Il porte le socle — navigation, rôles, statuts, actions communes — soit ce
// qui apparaît sur tous les écrans. Les écrans eux-mêmes viennent ensuite, dans
// l'ordre d'usage. Le sélecteur de langue ne sera montré qu'une fois la
// couverture faite : un sélecteur qui traduit un tiers de l'écran est pire
// qu'une interface unilingue.

export const fr = {
  // ─── Navigation ────────────────────────────────────────────────────────────
  'nav.tableauDeBord': 'Tableau de bord',
  'nav.facturation':   'Facturation',
  'nav.achats':        'Achats',
  'nav.contacts':      'Contacts',
  'nav.journal':       'Journal',
  'nav.planComptable': 'Plan comptable',
  'nav.rapports':      'Rapports',
  'nav.parametres':    'Paramètres',
  'nav.deconnexion':   'Déconnexion',

  // ─── Rôles ─────────────────────────────────────────────────────────────────
  'role.admin':      'Administrateur',
  'role.comptable':  'Comptable',
  'role.lecture':    'Lecture seule',
  'role.lectureSeuleTitre':   'Compte en lecture seule',
  'role.lectureSeuleDetail': 'Vous n’êtes pas autorisé à faire des modifications. Consultation et exports uniquement.',
  'role.lectureSeuleRaison': 'Votre compte est en lecture seule : vous pouvez tout consulter et tout exporter, mais rien modifier.',

  // ─── Statuts ───────────────────────────────────────────────────────────────
  'statut.brouillon':      'Brouillon',
  'statut.envoyee':        'Envoyée',
  'statut.payee':          'Payée',
  'statut.enRetard':       'En retard',
  'statut.comptabilisee':  'Comptabilisée',
  'statut.annulee':        'Annulée',
  'statut.archivee':       'Archivée',

  // ─── Actions communes ──────────────────────────────────────────────────────
  'action.enregistrer':  'Enregistrer',
  'action.annuler':      'Annuler',
  'action.supprimer':    'Supprimer',
  'action.modifier':     'Modifier',
  'action.fermer':       'Fermer',
  'action.retour':       'Retour',
  'action.telecharger':  'Télécharger',
  'action.rechercher':   'Rechercher…',
  'action.confirmer':    'Confirmer',
  'action.continuer':    'Continuer',

  // ─── États génériques ──────────────────────────────────────────────────────
  'etat.chargement':     'Chargement…',
  'etat.enregistrement': 'Enregistrement…',
  'etat.aucunResultat':  'Aucun résultat',
  'etat.erreur':         'Une erreur est survenue.',

  // ─── Comptabilité — termes du glossaire ────────────────────────────────────
  'compta.ecriture':      'Écriture',
  'compta.extourne':      'Extourne',
  'compta.debit':         'Débit',
  'compta.credit':        'Crédit',
  'compta.solde':         'Solde',
  'compta.charge':        'Charge',
  'compta.produit':       'Produit',
  'compta.grandLivre':    'Grand livre',
  'compta.balance':       'Balance de vérification',
  'compta.bilan':         'Bilan',
  'compta.resultat':      'Compte de résultat',
  'compta.exercice':      'Exercice',
  'compta.pieceComptable': 'Pièce comptable',

  // ─── TVA ───────────────────────────────────────────────────────────────────
  'tva.tva':            'TVA',
  'tva.impotPrealable': 'Impôt préalable',
  'tva.taux':           'Taux de TVA',
  'tva.tauxNormal':     '{taux} % — taux normal',
  'tva.tauxReduit':     '{taux} % — taux réduit (alimentation, livres, médicaments)',
  'tva.tauxHebergement': '{taux} % — hébergement',
  'tva.tauxZero':       '0 % — exonéré ou hors du champ',
  'tva.horsTaxe':       'Hors taxe',
  'tva.toutesTaxes':    'TTC',

  // ─── Documents ─────────────────────────────────────────────────────────────
  'doc.facture':           'Facture',
  'doc.factureFournisseur': 'Facture fournisseur',
  'doc.noteDeCredit':      'Note de crédit',
  'doc.offre':             'Offre de prix',
  'doc.echeance':          'Échéance',
  'doc.numero':            'N° de la facture',
  'doc.dateFacture':       'Date de la facture',
  'doc.montant':           'Montant',
  'doc.client':            'Client',
  'doc.fournisseur':       'Fournisseur',

  // ─── Paiement ──────────────────────────────────────────────────────────────
  'paiement.qrFacture':     'QR-facture',
  'paiement.qrIban':        'QR-IBAN',
  'paiement.iban':          'IBAN',
  'paiement.reference':     'Référence de paiement',
  'paiement.ordre':         'Ordre de paiement',
  'paiement.releveBancaire': 'Relevé bancaire',
  'paiement.rapprochement': 'Rapprochement bancaire',

  // ─── Sécurité ──────────────────────────────────────────────────────────────
  'securite.secondFacteur':  'Second facteur',
  'securite.otp':            'Code à usage unique (OTP)',
  'securite.appAuth':        'Application d’authentification 2FA/OTP',
  'securite.codeSecours':    'Code de secours',
  'securite.motDePasse':     'Mot de passe',
  'securite.phraseDePasse':  'Phrase de passe',
  'securite.sauvegarde':     'Sauvegarde',
  'securite.restauration':   'Restauration',
  'securite.chiffrement':    'Chiffrement',
  'securite.tracabilite':    'Traçabilité',
  'securite.attestation':    'Attestation d’intégrité',

  // ─── Connexion ─────────────────────────────────────────────────────────────
  'connexion.titre':        'Connexion',
  'connexion.sousTitre':    'Accédez à votre espace.',
  'connexion.email':        'Adresse e-mail',
  'connexion.motDePasse':   'Mot de passe',
  'connexion.seConnecter':  'Se connecter',
  'connexion.identifiantsIncorrects': 'Identifiants incorrects.',
  'connexion.piedDePage':   'LedgerAlps — Données locales · CO · nLPD',

  'connexion.sessionFermee': 'Session fermée après inactivité',
  'connexion.aideCodeSecours': 'Reprenez la liste notée lors de l’activation et saisissez un code non encore utilisé.',
  'connexion.aideCodeApp': 'Ouvrez votre application d’authentification et recopiez le code affiché.',
  'connexion.utiliserApp': 'Utiliser le code de mon application',
  'connexion.utiliserSecours': 'Application indisponible ? Utiliser un code de secours',
  'connexion.revenir': 'Revenir à la connexion',
  'connexion.verification': 'Vérification',
  'connexion.enCours': 'Connexion…',
  'connexion.saisirCodeSecours': 'Saisissez l’un des codes de secours notés lors de l’activation.',
  'connexion.saisirCodeApp': 'Saisissez le code affiché par votre application d’authentification.',
  // ─── Bandeau de compte (barre latérale) ────────────────────────────────────
  'banniere.adminTitre':   'Compte ADMINISTRATEUR',
  'banniere.adminDetail': 'Ne pas utiliser pour le travail courant. Ce compte peut effacer les sauvegardes, changer les rôles et déchiffrer la base.',
  'banniere.comptableTitre':  'Compte comptable',
  'banniere.comptableDetail': 'Vous tenez les livres. Les sauvegardes, la sécurité et les comptes utilisateurs restent réservés à un administrateur.',

  // ─── Langue ────────────────────────────────────────────────────────────────
  'langue.encoursTitre': 'Traduction en cours',
  'langue.encoursDetail': 'La navigation, la connexion et le vocabulaire comptable sont traduits. Les écrans restent en français ; ils suivent, écran par écran.',
  'langue.titre':       'Langue',
  'langue.description': 'La langue de l’interface. Le changement est immédiat.',

  // ─── Facturation — liste ───
  'fact.onglietFactures': 'Factures',
  'fact.ongletOffres': 'Offres de prix',
  'fact.nouvelleFacture': 'Nouvelle facture',
  'fact.nouvelleOffre': 'Nouvelle offre',
  'fact.tousContacts': 'Tous les contacts',
  'fact.filtrerFactures': 'Filtrer les factures par contact',
  'fact.filtrerOffres': 'Filtrer les offres par contact',
  'fact.rechercherNumero': 'Rechercher un numéro…',
  'fact.filtreToutes': 'Toutes',
  'fact.filtreBrouillons': 'Brouillons',
  'fact.filtreEnvoyees': 'Envoyées',
  'fact.filtrePayees': 'Payées',
  'fact.filtreEnRetard': 'En retard',
  'fact.colNumero': 'Numéro',
  'fact.colDate': 'Date',
  'fact.colContact': 'Contact',
  'fact.colTotal': 'Total CHF',
  'fact.colStatut': 'Statut',
  'fact.contactSupprime': 'contact supprimé',
  'fact.telechargerPDF': 'Télécharger PDF',
  'fact.payer': 'Payer',
  'fact.creer': 'Créer',
  'fact.aucuneFacture': 'Aucune facture',
  'fact.aucuneOffre': 'Aucune offre de prix',
  'fact.premiereFacture': 'Créez votre première facture pour démarrer.',
  'fact.premiereOffre': 'Créez votre première offre de prix.',
  'fact.paiementAuJournal': 'Le paiement est passé au journal (banque / débiteurs).',
  'fact.plusModifiable': 'La facture ne sera plus modifiable.',

  // ─── Contacts ───
  'ct.nouveau': 'Nouveau contact',
  'ct.ajouter': 'Ajouter',
  'ct.aucun': 'Aucun contact',
  'ct.tous': 'Tous',
  'ct.clients': 'Clients',
  'ct.fournisseurs': 'Fournisseurs',
  'ct.ajoutezClientsFournisseurs': 'Ajoutez vos clients et fournisseurs.',

  // ─── Pluriels ───
  'fact.unDocument': '{n} document',
  'fact.desDocuments': '{n} documents',

  // ─── Achats ───
  'ach.sousTitre': 'Factures fournisseurs et ordres de paiement',
  'ach.lirePDF': 'Lire un PDF',
  'ach.lecture': 'Lecture…',
  'ach.saisirFacture': 'Saisir une facture',
  'ach.masquer': 'Masquer',
  'ach.qrLu': 'QR-facture lu',
  'ach.rienALire': 'Rien à lire dans ce document',
  'ach.aucunQR': 'Aucun QR-facture trouvé.',
  'ach.nouveauFournisseur': 'Nouveau fournisseur',
  'ach.nom': 'Nom *',
  'ach.email': 'E-mail',
  'ach.ide': 'N° IDE',
  'ach.ibanAide': 'Pour une facture sans QR, ou avec une référence Creditor Reference.',
  'ach.ibanEstQR': 'Ce compte est un QR-IBAN — il sera enregistré comme tel.',
  'ach.qrIbanAide': 'Rempli par la lecture du QR. Institution 30000 à 31999.',
  'ach.qrIbanPasQR': 'Ce compte n’est pas un QR-IBAN — il sera enregistré comme IBAN ordinaire.',
  'ach.creerSelectionner': 'Créer et sélectionner',
  'ach.titreNouvelle': 'Nouvelle facture fournisseur',
  'ach.titreModifier': 'Modifier la facture {ref}',
  'ach.fournisseurObl': 'Fournisseur *',
  'ach.choisir': 'Choisir…',
  'ach.sansIban': '(sans IBAN)',
  'ach.aucunFournisseur': 'Aucun fournisseur enregistré pour l’instant.',
  'ach.numeroObl': 'N° de la facture *',
  'ach.numeroAide': 'Tel qu’imprimé par le fournisseur.',
  'ach.dateObl': 'Date de la facture *',
  'ach.objet': 'Objet',
  'ach.objetExemple': 'Fournitures, abonnement, sous-traitance…',
  'ach.montantObl': 'Montant *',
  'ach.montantTTCAide': 'Le montant à payer, tel qu’il figure sur la facture.',
  'ach.montantHTAide': 'Le montant hors taxe.',
  'ach.compteCharge': 'Compte de charge',
  'ach.compteParDefaut': '6500 — Charges d’administration (par défaut)',
  'ach.refAide': 'Celle du bulletin de versement, pas le n° de facture. Elle voyagera dans l’ordre de virement pour que le fournisseur reconnaisse votre paiement.',
  'ach.refExemple': '27 chiffres, ou RF…',
  'ach.sansTVA': 'Sans TVA : le montant hors taxe est le montant payé, et il n’y a pas d’impôt préalable à déduire.',
  'ach.enregistrerBrouillon': 'Enregistrer le brouillon',
  'ach.enregistrerModifs': 'Enregistrer les modifications',
  'ach.facturesRecues': 'Factures reçues',
  'ach.numeroCol': 'N° facture',
  'ach.aucuneFacture': 'Aucune facture fournisseur',
  'ach.aucuneFactureAide': 'Saisissez ce que vous devez : la TVA payée à vos fournisseurs se déduit de celle que vous encaissez, et la charge entre dans votre résultat.',
  'ach.comptabiliser': 'Comptabiliser',
  'ach.confirmTitre': 'Comptabiliser la facture {ref} ?',
  'ach.confirmScellee': 'L’écriture est passée et scellée : charge et TVA déductible au débit, créanciers au crédit.',
  'ach.confirmScelleeSansTVA': 'L’écriture est passée et scellée : la charge au débit, créanciers au crédit.',
  'ach.confirmDeclaration': 'La TVA payée entre dans votre déclaration (impôt préalable, chiffre 400).',
  'ach.confirmSansTVA': 'Cette facture ne porte aucune TVA : il n’y a pas d’impôt préalable à récupérer, et rien à reporter au chiffre 400.',
  'ach.confirmPayable': 'La facture devient payable et apparaît dans l’ordre de paiement.',
  'ach.confirmPasPayee': 'Elle ne devient pas « payée » pour autant : c’est le relevé bancaire qui l’établira.',

  // ─── Paiements ───
  'pay.titre': 'Paiements fournisseurs — ISO 20022 pain.001',
  'pay.intro': 'Sélectionnez les factures à régler : LedgerAlps produit un fichier XML que vous déposez dans votre e-banking. Compatible UBS, PostFinance, Raiffeisen et Banques cantonales.',
  'pay.compteADebiter': 'Compte à débiter',
  'pay.aucuneAPayer': 'Aucune facture à payer',
  'pay.aucuneAPayerAide': 'Seules les factures fournisseurs comptabilisées apparaissent ici. Une facture encore au brouillon doit d’abord être comptabilisée : la charge doit exister dans les livres avant que la trésorerie bouge.',
  'pay.toutSelectionner': 'Tout sélectionner',
  'pay.colFacture': 'Facture',
  'pay.colReference': 'Référence',
  'pay.joursRetard': '{n} jours de retard',
  'pay.uneBloquee': '{n} facture ne peut pas être payée',
  'pay.desBloquees': '{n} factures ne peuvent pas être payées',
  'pay.retirerUne': 'Retirer {n} facture de la liste',
  'pay.retirerPlusieurs': 'Retirer {n} factures de la liste',
  'pay.dateExecution': 'Date d’exécution souhaitée',
  'pay.dateExecutionAide': 'La banque exécute à cette date, ou au premier jour ouvrable suivant.',
  'pay.uneSelectionnee': '{n} facture sélectionnée',
  'pay.desSelectionnees': '{n} factures sélectionnées',
  'pay.genererFichier': 'Générer le fichier',
  'pay.fichierProduitUn': 'Fichier produit — {n} virement, {montant}',
  'pay.fichierProduitPlusieurs': 'Fichier produit — {n} virements, {montant}',
  'pay.deposerEbanking': 'Déposez-le dans votre e-banking pour que les virements partent. Les factures restent comptabilisées jusqu’à ce que le débit apparaisse au relevé.',
  'pay.confirmRetirerUne': 'Retirer {n} facture de la liste ?',
  'pay.confirmRetirerPlusieurs': 'Retirer {n} factures de la liste ?',
  'pay.confirmBrouillon': 'Un brouillon est supprimé : rien n’était entré dans les livres.',
  'pay.confirmExtourne': 'Une facture comptabilisée est extournée — une écriture inverse neutralise la charge et la TVA déductible — puis marquée annulée. La pièce et les deux écritures restent lisibles (CO art. 958f).',
  'pay.confirmPayee': 'Une facture déjà réglée est refusée : l’argent est parti.',
  'pay.confirmVerdict': 'Chaque facture reçoit son propre verdict : un lot partiel est normal, et le détail vous est rendu.',
  'pay.retraitBilan': '{n} facture(s) retirée(s) sur {total}.',
  'pay.retraitNonTraitees': 'Non traitée(s) : {details}',
  'pay.motifLibre': 'motif en texte libre',

  // ─── Journal et Rapports ───
  'jr.sousTitre': 'Écritures comptables — CO art. 957a',
  'jr.nouvelle': 'Nouvelle écriture',
  'jr.ecritureManuelle': 'Écriture manuelle',
  'jr.pasAuto': 'Les factures envoyées ne s’inscrivent pas ici automatiquement',
  'jr.description': 'Description *',
  'jr.date': 'Date *',
  'jr.descExemple': 'Vente comptant, achat de fournitures…',
  'jr.lignes': 'Lignes',
  'jr.ligne': 'Ligne',
  'jr.compteDebite': 'Compte débité',
  'jr.compteCredite': 'Compte crédité',
  'jr.montantCHF': 'Montant CHF',
  'jr.libelle': 'Libellé',
  'jr.facultatif': 'Facultatif',
  'jr.equilibree': 'Équilibrée',
  'jr.renseignez': 'Renseignez au moins un compte et un montant.',
  'jr.aideVentilation': 'Un débit et un crédit sur la même ligne forment une écriture simple. Pour une ventilation, laissez un côté vide et ajoutez des lignes.',
  'jr.colRef': 'Réf.',
  'jr.colDescription': 'Description',
  'jr.colAuteur': 'Auteur',
  'jr.precedente': 'Précédente',
  'jr.suivante': 'Suivante',
  'jr.vide': 'Journal vide',
  'jr.videAide': 'Les écritures apparaîtront ici. Les factures émises s’y inscrivent si la comptabilisation automatique est activée.',
  'jr.statutContrepassee': 'Contrepassée',
  'jr.confirmScellee': 'L’écriture est scellée par une empreinte chaînée (CO art. 957a al. 2 ch. 5) et ne peut plus être modifiée.',
  'jr.confirmEntre': 'Elle entre dans la balance, le bilan et le compte de résultat.',
  'jr.confirmCorrection': 'Une correction se fait ensuite par contrepassation, jamais par retouche.',
  'jr.brouillonAide': 'Brouillon : aucune empreinte. Rien ne la scelle et elle ne compte ni à la balance, ni au bilan, ni au compte de résultat.',
  'jr.compte': 'Compte',
  'rp.sousTitre': 'Exports comptables et import bancaire ISO 20022',
  'rp.periodeAide': 'La période s’applique aux trois exports ci-dessous.',
  'rp.documentsComptables': 'Documents comptables',
  'rp.csvAide': 'Fichiers CSV séparés par des point-virgules, lisibles directement dans Excel. Seules les écritures comptabilisées y figurent : un brouillon n’est scellé par rien et pourrait encore changer.',
  'rp.journalGeneral': 'Journal général',
  'rp.journalGeneralAide': 'Toutes les écritures dans l’ordre chronologique, avec leur empreinte.',
  'rp.grandLivreAide': 'Les mêmes mouvements rangés par compte, avec le solde cumulé après chaque ligne.',
  'rp.balanceAide': 'Totaux par compte et contrôle d’équilibre. Le premier document que demande une fiduciaire.',
  'rp.telechargerCSV': 'Télécharger le CSV',
  'rp.preparation': 'Préparation…',
  'rp.importBancaire': 'Import bancaire — ISO 20022 camt.053',
  'rp.releveCamt': 'Relevé bancaire camt.053',
  'rp.releveCamtAide': 'Déposez le fichier XML téléchargé depuis votre e-banking : LedgerAlps y reconnaît vos encaissements et propose les rapprochements.',
  'rp.choisirXML': 'Choisir un fichier XML',
  'rp.exportsOuverts': 'L’import d’un relevé bancaire et la production d’un ordre de paiement demandent un compte autorisé à écrire. Les exports et l’archive légale ci-dessous vous restent ouverts.',
  'rp.archivage': 'Archivage légal — CO art. 958f (10 ans)',
  'rp.archiveZIP': 'Archive légale ZIP',
  'rp.archiveJSON': 'Toute la comptabilité en JSON',
  'rp.archiveAide': 'L’archive couvre l’ensemble des données, sans filtre de période : c’est ce que la conservation légale exige.',
  'rp.telechargerArchive': 'Télécharger l’archive',
  'rp.du': 'du',
  'rp.au': 'au',

  // ─── Journal — dialogue ───
  'jr.confirmTitre': 'Comptabiliser {ref} ?',
  'jr.confirmBrouillon': 'Tant qu’elle reste un brouillon, elle ne compte nulle part.',

  // ─── Rapports — archive ───
  'rp.archiveZIPAide': 'Toute la comptabilité en JSON et en CSV — écritures, factures, contacts, exercices — avec un manifeste SHA-256. Le format est ce qui rend la sortie réelle : une archive qu’on ne peut pas relire ailleurs n’est pas une archive.',

  // ─── Rapports — camt ───
  'rp.uneTransactionLue': '{n} transaction lue — rapprochez-la dans Paramètres → Banque',
  'rp.desTransactionsLues': '{n} transactions lues — rapprochez-les dans Paramètres → Banque',
} as const
