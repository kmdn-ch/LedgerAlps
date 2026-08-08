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
} as const
