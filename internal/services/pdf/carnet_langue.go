package pdf

// Les libellés du carnet du lait, dans les quatre langues du produit.
//
// # Pourquoi un jeu de libellés et non le catalogue du serveur
//
// `internal/i18n` traduit les MESSAGES d'erreur, indexés par leur phrase
// française et réécrits à la sortie JSON. Un PDF ne passe pas par là — il sort
// en binaire, et le middleware le laisse traverser sans y toucher, à raison :
// une facture de dix mégaoctets n'a pas à passer par la mémoire pour qu'on y
// cherche un mot.
//
// Les libellés d'un document sont d'ailleurs d'une autre nature qu'un message
// d'erreur : ce sont des termes comptables normalisés, dont la traduction
// engage — « Vermögenslage » et « situazione patrimoniale » sont les termes du
// CO dans leurs versions allemande et italienne, pas des approximations. Ils
// vivent donc ici, structurés, où on les relit d'un regard.
//
// # Les références légales changent de nom, pas seulement de langue
//
// Le Code des obligations est l'OR en allemand et le CO en italien ; la loi
// sur la TVA est la MWSTG et la LIVA. Un document remis à une administration
// cantonale alémanique qui citerait « CO art. 957 » citerait une loi qui n'y
// porte pas ce nom.

import "github.com/kmdn-ch/ledgeralps/internal/i18n"

// libellésCarnet porte tout le texte fixe du document.
type libellésCarnet struct {
	Titre       string
	SousTitre   string
	IDE         string
	Exercice    string
	BaseCaisse  string
	Recettes    string
	Depenses    string
	TotalRec    string
	TotalDep    string
	Resultat    string
	Patrimoine  string
	Avoirs      string
	Engagements string
	TotalAvoirs string
	TotalEngag  string
	Fortune     string
	Compte      string
	Libelle     string
	AucunMouv   string
	Neant       string
	Regime      string
	CA          string
	Admise      string
	Refusee     string
	TVADue      string
	TVALiberee  string

	// PeriodePartielle remplace les DEUX verdicts quand la periode n'est
	// pas un exercice. Les seuils du CO art. 957 et de la LTVA art. 10
	// portent sur le chiffre d'affaires du dernier exercice : mesures sur
	// un trimestre, ils affirmaient en vert une eligibilite que rien ne
	// fondait. Le document dit alors qu'il ne conclut pas.
	PeriodePartielle string
	PiedDePage       string
	Page             string
	SansRaison       string
	NomFichier       string

	// Propres à l'export CSV : il porte les mêmes informations que le PDF,
	// mais sous forme de lignes plutôt que de mise en page.
	Periode     string
	Montant     string
	Oui         string
	Non         string
	LigneAdmise string
	LigneTVA    string
}

// libellés rend le jeu correspondant à la langue, français par défaut.
func libellés(l i18n.Lang) libellésCarnet {
	if v, ok := jeuxCarnet[l]; ok {
		return v
	}
	return jeuxCarnet[i18n.FR]
}

var jeuxCarnet = map[i18n.Lang]libellésCarnet{
	i18n.FR: {
		Titre:     "Comptabilité simplifiée",
		SousTitre: "Recettes, dépenses et état du patrimoine — CO art. 957 al. 2 ch. 1",
		IDE:       "N° IDE : ",
		Exercice:  "Exercice du %s au %s",
		BaseCaisse: "Établi selon le principe des recettes et des dépenses (base de caisse) : " +
			"les montants sont comptés au moment de l'encaissement et du décaissement. " +
			"Les factures émises et non encore encaissées n'y figurent donc pas ; elles " +
			"apparaissent au chiffre d'affaires et à l'état du patrimoine.",
		Recettes:    "RECETTES",
		Depenses:    "DÉPENSES",
		TotalRec:    "Total des recettes",
		TotalDep:    "Total des dépenses",
		Resultat:    "RÉSULTAT DE L'EXERCICE",
		Patrimoine:  "ÉTAT DU PATRIMOINE AU ",
		Avoirs:      "Avoirs",
		Engagements: "Engagements",
		TotalAvoirs: "Total des avoirs",
		TotalEngag:  "Total des engagements",
		Fortune:     "FORTUNE NETTE",
		Compte:      "Compte",
		Libelle:     "Libellé",
		AucunMouv:   "Aucun mouvement sur la période",
		Neant:       "Néant",
		Regime:      "Régime applicable",
		CA:          "Chiffre d'affaires de l'exercice",
		Admise: "Chiffre d'affaires inférieur à 500 000 francs : la comptabilité simplifiée " +
			"est admise (CO art. 957 al. 2 ch. 1).",
		Refusee: "ATTENTION — le chiffre d'affaires atteint ou dépasse 500 000 francs. " +
			"La comptabilité en partie double et les comptes annuels sont obligatoires " +
			"(CO art. 957 al. 1) : ce document ne peut pas être présenté seul.",
		TVADue: "Chiffre d'affaires égal ou supérieur à 100 000 francs : assujettissement à la TVA " +
			"(LTVA art. 10). Le décompte TVA accompagne ce document.",
		TVALiberee: "Chiffre d'affaires inférieur à 100 000 francs : libération de l'assujettissement " +
			"à la TVA (LTVA art. 10 al. 2 let. a).",
		PiedDePage:       "Établi par LedgerAlps — comptabilité en partie double",
		Page:             "Page %d/{nb}",
		SansRaison:       "(raison sociale non renseignée)",
		NomFichier:       "comptabilite-simplifiee",
		PeriodePartielle: "Période inférieure à un exercice : les seuils du CO art. 957 al. 2 ch. 1 et de la LTVA art. 10 al. 2 let. a portent sur le chiffre d'affaires du dernier exercice et ne peuvent pas être appréciés sur ce document.",
		Periode:          "Période",
		Montant:          "Montant",
		Oui:              "oui",
		Non:              "non",
		LigneAdmise:      "Comptabilité simplifiée admise (< 500 000)",
		LigneTVA:         "Assujettissement TVA (>= 100 000)",
	},

	i18n.DE: {
		Titre:     "Vereinfachte Buchhaltung",
		SousTitre: "Einnahmen, Ausgaben und Vermögenslage — OR Art. 957 Abs. 2 Ziff. 1",
		IDE:       "UID-Nr.: ",
		Exercice:  "Geschäftsjahr vom %s bis %s",
		BaseCaisse: "Nach dem Grundsatz der Einnahmen und Ausgaben erstellt (Kassenprinzip): " +
			"Beträge werden im Zeitpunkt des Zahlungseingangs und der Zahlung erfasst. " +
			"Gestellte, aber noch nicht bezahlte Rechnungen erscheinen daher nicht; sie " +
			"zählen zum Umsatz und zur Vermögenslage.",
		Recettes:    "EINNAHMEN",
		Depenses:    "AUSGABEN",
		TotalRec:    "Total Einnahmen",
		TotalDep:    "Total Ausgaben",
		Resultat:    "ERGEBNIS DES GESCHÄFTSJAHRES",
		Patrimoine:  "VERMÖGENSLAGE PER ",
		Avoirs:      "Vermögen",
		Engagements: "Verbindlichkeiten",
		TotalAvoirs: "Total Vermögen",
		TotalEngag:  "Total Verbindlichkeiten",
		Fortune:     "NETTOVERMÖGEN",
		Compte:      "Konto",
		Libelle:     "Bezeichnung",
		AucunMouv:   "Keine Bewegung in dieser Periode",
		Neant:       "Keine",
		Regime:      "Anwendbare Regelung",
		CA:          "Umsatz des Geschäftsjahres",
		Admise: "Umsatz unter 500 000 Franken: die vereinfachte Buchhaltung ist zulässig " +
			"(OR Art. 957 Abs. 2 Ziff. 1).",
		Refusee: "ACHTUNG — der Umsatz erreicht oder übersteigt 500 000 Franken. " +
			"Die doppelte Buchhaltung und die Jahresrechnung sind obligatorisch " +
			"(OR Art. 957 Abs. 1): dieses Dokument genügt allein nicht.",
		TVADue: "Umsatz von 100 000 Franken oder mehr: MWST-Pflicht (MWSTG Art. 10). " +
			"Die MWST-Abrechnung begleitet dieses Dokument.",
		TVALiberee: "Umsatz unter 100 000 Franken: Befreiung von der MWST-Pflicht " +
			"(MWSTG Art. 10 Abs. 2 Bst. a).",
		PiedDePage:       "Erstellt mit LedgerAlps — doppelte Buchhaltung",
		Page:             "Seite %d/{nb}",
		SansRaison:       "(Firmenname nicht erfasst)",
		NomFichier:       "vereinfachte-buchhaltung",
		PeriodePartielle: "Zeitraum kürzer als ein Geschäftsjahr: Die Schwellenwerte von OR Art. 957 Abs. 2 Ziff. 1 und MWSTG Art. 10 Abs. 2 Bst. a beziehen sich auf den Umsatz des letzten Geschäftsjahres und können anhand dieses Dokuments nicht beurteilt werden.",
		Periode:          "Periode",
		Montant:          "Betrag",
		Oui:              "ja",
		Non:              "nein",
		LigneAdmise:      "Vereinfachte Buchhaltung zulässig (< 500 000)",
		LigneTVA:         "MWST-Pflicht (>= 100 000)",
	},

	i18n.IT: {
		Titre:     "Contabilità semplificata",
		SousTitre: "Entrate, uscite e situazione patrimoniale — CO art. 957 cpv. 2 n. 1",
		IDE:       "N. IDI: ",
		Exercice:  "Esercizio dal %s al %s",
		BaseCaisse: "Allestita secondo il principio delle entrate e delle uscite (principio di cassa): " +
			"gli importi sono contati al momento dell'incasso e del pagamento. " +
			"Le fatture emesse e non ancora incassate non vi figurano; esse " +
			"contano nella cifra d'affari e nella situazione patrimoniale.",
		Recettes:    "ENTRATE",
		Depenses:    "USCITE",
		TotalRec:    "Totale entrate",
		TotalDep:    "Totale uscite",
		Resultat:    "RISULTATO D'ESERCIZIO",
		Patrimoine:  "SITUAZIONE PATRIMONIALE AL ",
		Avoirs:      "Averi",
		Engagements: "Impegni",
		TotalAvoirs: "Totale averi",
		TotalEngag:  "Totale impegni",
		Fortune:     "PATRIMONIO NETTO",
		Compte:      "Conto",
		Libelle:     "Descrizione",
		AucunMouv:   "Nessun movimento nel periodo",
		Neant:       "Nessuno",
		Regime:      "Regime applicabile",
		CA:          "Cifra d'affari dell'esercizio",
		Admise: "Cifra d'affari inferiore a 500 000 franchi: la contabilità semplificata " +
			"è ammessa (CO art. 957 cpv. 2 n. 1).",
		Refusee: "ATTENZIONE — la cifra d'affari raggiunge o supera i 500 000 franchi. " +
			"La contabilità in partita doppia e i conti annuali sono obbligatori " +
			"(CO art. 957 cpv. 1): questo documento non può essere presentato da solo.",
		TVADue: "Cifra d'affari pari o superiore a 100 000 franchi: assoggettamento all'IVA " +
			"(LIVA art. 10). Il rendiconto IVA accompagna questo documento.",
		TVALiberee: "Cifra d'affari inferiore a 100 000 franchi: esenzione dall'assoggettamento " +
			"all'IVA (LIVA art. 10 cpv. 2 lett. a).",
		PiedDePage:       "Allestita con LedgerAlps — contabilità in partita doppia",
		Page:             "Pagina %d/{nb}",
		SansRaison:       "(ragione sociale non indicata)",
		NomFichier:       "contabilita-semplificata",
		PeriodePartielle: "Periodo inferiore a un esercizio: le soglie dell'art. 957 cpv. 2 n. 1 CO e dell'art. 10 cpv. 2 lett. a LIVA si riferiscono alla cifra d'affari dell'ultimo esercizio e non possono essere valutate su questo documento.",
		Periode:          "Periodo",
		Montant:          "Importo",
		Oui:              "sì",
		Non:              "no",
		LigneAdmise:      "Contabilità semplificata ammessa (< 500 000)",
		LigneTVA:         "Assoggettamento IVA (>= 100 000)",
	},

	i18n.EN: {
		Titre:     "Simplified accounting",
		SousTitre: "Receipts, expenditure and net worth — CO art. 957 para. 2 no. 1",
		IDE:       "UID no.: ",
		Exercice:  "Financial year from %s to %s",
		BaseCaisse: "Prepared on the receipts-and-expenditure principle (cash basis): " +
			"amounts are counted when money is received and paid. " +
			"Invoices issued but not yet paid therefore do not appear here; they " +
			"count towards turnover and net worth.",
		Recettes:    "RECEIPTS",
		Depenses:    "EXPENDITURE",
		TotalRec:    "Total receipts",
		TotalDep:    "Total expenditure",
		Resultat:    "RESULT FOR THE YEAR",
		Patrimoine:  "NET WORTH AS AT ",
		Avoirs:      "Assets",
		Engagements: "Liabilities",
		TotalAvoirs: "Total assets",
		TotalEngag:  "Total liabilities",
		Fortune:     "NET WORTH",
		Compte:      "Account",
		Libelle:     "Description",
		AucunMouv:   "No movement in the period",
		Neant:       "None",
		Regime:      "Applicable regime",
		CA:          "Turnover for the year",
		Admise: "Turnover below CHF 500,000: simplified accounting is allowed " +
			"(CO art. 957 para. 2 no. 1).",
		Refusee: "WARNING — turnover reaches or exceeds CHF 500,000. " +
			"Double-entry bookkeeping and annual accounts are mandatory " +
			"(CO art. 957 para. 1): this document cannot be submitted on its own.",
		TVADue: "Turnover of CHF 100,000 or more: VAT liability applies (VAT Act art. 10). " +
			"The VAT return accompanies this document.",
		TVALiberee: "Turnover below CHF 100,000: exempt from VAT liability " +
			"(VAT Act art. 10 para. 2 let. a).",
		PiedDePage:       "Prepared with LedgerAlps — double-entry bookkeeping",
		Page:             "Page %d/{nb}",
		SansRaison:       "(business name not set)",
		NomFichier:       "simplified-accounting",
		PeriodePartielle: "Period shorter than a financial year: the thresholds of CO art. 957 para. 2 no. 1 and VAT Act art. 10 para. 2 let. a apply to the turnover of the last financial year and cannot be assessed from this document.",
		Periode:          "Period",
		Montant:          "Amount",
		Oui:              "yes",
		Non:              "no",
		LigneAdmise:      "Simplified accounting allowed (< 500,000)",
		LigneTVA:         "VAT liability (>= 100,000)",
	},
}

// SansRaisonSociale rend la mention affichée quand la fiche entreprise est
// vide. Exportée parce que le gestionnaire la pose avant d'appeler le rendu.
func SansRaisonSociale(l i18n.Lang) string { return libellés(l).SansRaison }

// NomFichierCarnet rend le nom de fichier, dans la langue du document.
//
// Le fichier atterrit dans un dossier de téléchargements parmi cent autres :
// c'est par son nom qu'on le reconnaît, et « comptabilite-simplifiee » ne dit
// rien à quelqu'un qui travaille en allemand.
func NomFichierCarnet(l i18n.Lang) string { return libellés(l).NomFichier }

// LibellesCarnet expose les libellés pour l'export CSV, qui porte les mêmes
// informations que le PDF sous une autre forme.
//
// Le type reste non exporté : l'appelant lit des champs, il n'en construit
// pas — un jeu de libellés fabriqué ailleurs échapperait à la relecture qui
// garantit que les quatre langues disent la même chose.
func LibellesCarnet(l i18n.Lang) libellésCarnet { return libellés(l) }
