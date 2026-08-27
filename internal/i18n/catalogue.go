package i18n

// Catalogue des textes que LedgerAlps adresse à l'utilisateur.
//
// LA CLÉ EST LA PHRASE FRANÇAISE. Voir i18n.go pour le pourquoi ; en deux
// mots : le français est la langue des sources, il sert déjà aux journaux et
// aux tests, et une phrase absente d'ici ressort en français — ce que
// l'utilisateur voyait avant, jamais une clé nue.
//
// Une phrase à verbes de format (%s, %d, %.2f) doit porter dans ses quatre
// langues EXACTEMENT les mêmes verbes, dans le même ordre : les valeurs sont
// réinjectées par position, et un verbe déplacé afficherait le montant à la
// place du numéro de compte. Le test le vérifie.
//
// Écrit par scratchpad/cat_go.py — modifiable à la main sans dommage.

var catalogue = map[string]map[Lang]string{
	// Le logo est refuse sur le format REELLEMENT detecte dans ses octets,
	// pas sur ce que l'entete annonce. La personne qui envoie un GIF doit
	// comprendre ce qui est attendu, dans sa langue.
	"format d'image non accepté : %s (PNG ou JPEG attendu)": {
		DE: "Bildformat nicht akzeptiert: %s (PNG oder JPEG erwartet)",
		IT: "formato d'immagine non accettato: %s (atteso PNG o JPEG)",
		EN: "image format not accepted: %s (PNG or JPEG expected)",
	},
	// Le carnet du lait refuse de sortir quand ses totaux ne concordent pas
	// avec le mouvement net de trésorerie. C'est un message d'exploitation,
	// mais il atteint la personne qui a cliqué : elle doit le lire dans sa
	// langue pour savoir que le document N'A PAS été produit.
	"incohérence du carnet : le résultat (%.2f) ne correspond pas au mouvement net de liquidités (%.2f), écart de %.2f — le document n'est pas établi": {
		DE: "Inkonsistenz im Kassabuch: Das Ergebnis (%.2f) stimmt nicht mit der Nettoveränderung der flüssigen Mittel (%.2f) überein, Abweichung von %.2f — das Dokument wird nicht erstellt",
		IT: "Incoerenza del libro cassa: il risultato (%.2f) non corrisponde al movimento netto di liquidità (%.2f), scarto di %.2f — il documento non viene emesso",
		EN: "Cash book inconsistency: the result (%.2f) does not match the net cash movement (%.2f), a discrepancy of %.2f — the document is not produced",
	},

	"\nNote sur les données personnelles (nLPD) : les IBAN des contacts sont\n": {
		DE: "\nHinweis zu den Personendaten (DSG): die IBAN der Kontakte sind\n",
		IT: "\nNota sui dati personali (LPD): gli IBAN dei contatti sono\n",
		EN: "\nNote on personal data (FADP): the contacts’ IBANs are\n",
	},
	"  - Cellule vide = valeur absente, à distinguer d'un zéro.\n\n": {
		DE: "  - Leere Zelle = fehlender Wert, von einer Null zu unterscheiden.\n\n",
		IT: "  - Cella vuota = valore assente, da distinguere da uno zero.\n\n",
		EN: "  - Empty cell = missing value, to be distinguished from a zero.\n\n",
	},
	"  - Décimales  : point (.), comme dans le JSON.\n": {
		DE: "  - Dezimaltrennzeichen: Punkt (.), wie im JSON.\n",
		IT: "  - Decimali   : punto (.), come nel JSON.\n",
		EN: "  - Decimals   : dot (.), as in the JSON.\n",
	},
	"  - Encodage   : UTF-8 avec BOM, pour qu'Excel affiche les accents.\n": {
		DE: "  - Kodierung  : UTF-8 mit BOM, damit Excel die Akzente anzeigt.\n",
		IT: "  - Codifica   : UTF-8 con BOM, affinché Excel mostri gli accenti.\n",
		EN: "  - Encoding   : UTF-8 with BOM, so Excel shows the accents.\n",
	},
	"  - Séparateur : point-virgule (;) — attendu par Excel en Suisse.\n": {
		DE: "  - Trennzeichen: Semikolon (;) — von Excel in der Schweiz erwartet.\n",
		IT: "  - Separatore : punto e virgola (;) — atteso da Excel in Svizzera.\n",
		EN: "  - Separator : semicolon (;) — expected by Excel in Switzerland.\n",
	},
	"%d facture(s) sur %d ne sont plus payables : elles ont pu être réglées ou annulées entre-temps. Rechargez la liste": {
		DE: "%d von %d Rechnung(en) sind nicht mehr zahlbar: sie können zwischenzeitlich beglichen oder storniert worden sein. Laden Sie die Liste neu",
		IT: "%d fattura/e su %d non sono più pagabili: possono essere state saldate o annullate nel frattempo. Ricaricate l’elenco",
		EN: "%d of %d invoice(s) are no longer payable: they may have been settled or cancelled meanwhile. Reload the list",
	},
	"1. Le SCEAU, sans aucun logiciel : retirez la ligne « self_hash » de ce fichier, calculez l'empreinte SHA-256 de ce qui reste, et comparez-la à la valeur retirée. Sous Windows : certutil -hashfile fichier.json SHA256. Sous macOS ou Linux : shasum -a 256 fichier.json.": {
		DE: "1. Das SIEGEL, ohne jede Software: entfernen Sie die Zeile «self_hash» aus dieser Datei, berechnen Sie den SHA-256-Hashwert des Rests und vergleichen Sie ihn mit dem entfernten Wert. Unter Windows: certutil -hashfile datei.json SHA256. Unter macOS oder Linux: shasum -a 256 datei.json.",
		IT: "1. Il SIGILLO, senza alcun software: togliete la riga «self_hash» da questo file, calcolate l’impronta SHA-256 di ciò che resta e confrontatela con il valore tolto. Su Windows: certutil -hashfile file.json SHA256. Su macOS o Linux: shasum -a 256 file.json.",
		EN: "1. The SEAL, with no software at all: remove the “self_hash” line from this file, compute the SHA-256 hash of what remains, and compare it with the value you removed. On Windows: certutil -hashfile file.json SHA256. On macOS or Linux: shasum -a 256 file.json.",
	},
	"2. La CORRESPONDANCE, chez votre client : Paramètres → Maintenance → Conformité → « Vérifier une attestation », et déposez ce fichier. LedgerAlps compare l'empreinte de tête ci-dessus à celle que portent les livres au même numéro de séquence.": {
		DE: "2. Die ÜBEREINSTIMMUNG, bei Ihrem Kunden: Einstellungen → Wartung → Konformität → «Eine Bescheinigung prüfen», und legen Sie diese Datei ab. LedgerAlps vergleicht den obigen Kopf-Hashwert mit dem, den die Bücher an derselben Sequenznummer tragen.",
		IT: "2. La CORRISPONDENZA, presso il vostro cliente: Impostazioni → Manutenzione → Conformità → «Verificare un’attestazione», e depositate questo file. LedgerAlps confronta l’impronta di testa qui sopra con quella che i libri recano allo stesso numero di sequenza.",
		EN: "2. The MATCH, at your client’s: Settings → Maintenance → Compliance → “Check an attestation”, and drop this file. LedgerAlps compares the head hash above with the one the books carry at the same sequence number.",
	},
	"3. Ce que la comparaison prouve : conservez ce fichier. S'il correspond encore dans six mois, aucune des écritures qu'il couvre n'a été réécrite entre-temps. C'est la copie que VOUS détenez qui donne sa valeur au contrôle — le sceau seul ne protège que du fichier retouché à la main.": {
		DE: "3. Was der Vergleich beweist: bewahren Sie diese Datei auf. Stimmt sie in sechs Monaten noch, wurde keine der abgedeckten Buchungen zwischenzeitlich umgeschrieben. Den Wert der Prüfung macht die Kopie aus, die SIE besitzen — das Siegel allein schützt nur vor der von Hand retuschierten Datei.",
		IT: "3. Ciò che il confronto prova: conservate questo file. Se corrisponde ancora fra sei mesi, nessuna delle registrazioni che copre è stata riscritta nel frattempo. È la copia che VOI detenete a dare valore al controllo — il sigillo da solo protegge soltanto dal file ritoccato a mano.",
		EN: "3. What the comparison proves: keep this file. If it still matches in six months, none of the entries it covers has been rewritten meanwhile. What gives the check its value is the copy YOU hold — the seal alone only protects against a file retouched by hand.",
	},
	"Accès réinitialisé. Les sessions ouvertes de ce compte sont fermées, et la personne devra choisir son propre mot de passe avant de pouvoir faire quoi que ce soit.": {
		DE: "Zugang zurückgesetzt. Die offenen Sitzungen dieses Kontos sind beendet, und die Person muss ein eigenes Kennwort wählen, bevor sie irgendetwas tun kann.",
		IT: "Accesso reimpostato. Le sessioni aperte di questo account sono chiuse e la persona dovrà scegliere una propria password prima di poter fare qualsiasi cosa.",
		EN: "Access reset. The open sessions of this account are closed, and the person will have to choose their own password before doing anything.",
	},
	"Admin user created. This endpoint is now disabled.": {
		DE: "Administratorkonto erstellt. Dieser Endpunkt ist nun deaktiviert.",
		IT: "Account amministratore creato. Questo endpoint è ora disattivato.",
		EN: "Administrator account created. This endpoint is now disabled.",
	},
	"Adresse :": {
		DE: "Adresse: ",
		IT: "Indirizzo: ",
		EN: "Address: ",
	},
	"Annule la facture:": {
		DE: "Storniert die Rechnung:",
		IT: "Annulla la fattura:",
		EN: "Cancels invoice:",
	},
	"Attestation d'intégrité de la comptabilité": {
		DE: "Integritätsbescheinigung der Buchhaltung",
		IT: "Attestazione d’integrità della contabilità",
		EN: "Accounting integrity attestation",
	},
	"Attestation vérifiée": {
		DE: "Bescheinigung geprüft",
		IT: "Attestazione verificata",
		EN: "Attestation verified",
	},
	"Attestation vérifiée — aucune écriture couverte": {
		DE: "Bescheinigung geprüft — keine Buchung abgedeckt",
		IT: "Attestazione verificata — nessuna registrazione coperta",
		EN: "Attestation verified — no entry covered",
	},
	"Attestation vérifiée, mais les livres présentent une rupture": {
		DE: "Bescheinigung geprüft, die Bücher weisen jedoch einen Bruch auf",
		IT: "Attestazione verificata, ma i libri presentano una rottura",
		EN: "Attestation verified, but the books show a break",
	},
	"Au %s, la chaîne d'empreintes couvrant %d écriture(s) comptabilisée(s) est intacte.": {
		DE: "Am %s ist die Hash-Kette über %d verbuchte Buchung(en) unversehrt.",
		IT: "Al %s, la catena di impronte che copre %d registrazione/i contabilizzate è intatta.",
		EN: "As at %s, the hash chain covering %d posted entry/entries is intact.",
	},
	"Au %s, la chaîne d'empreintes présente %d anomalie(s) sur %d écriture(s).": {
		DE: "Am %s weist die Hash-Kette %d Auffälligkeit(en) bei %d Buchung(en) auf.",
		IT: "Al %s, la catena di impronte presenta %d anomalia/e su %d registrazione/i.",
		EN: "As at %s, the hash chain shows %d anomaly/anomalies over %d entry/entries.",
	},
	"BIC": {
		DE: "BIC",
		IT: "BIC",
		EN: "BIC",
	},
	"BIC/SWIFT :": {
		DE: "BIC/SWIFT: ",
		IT: "BIC/SWIFT: ",
		EN: "BIC/SWIFT: ",
	},
	"Banque": {
		DE: "Bank",
		IT: "Banca",
		EN: "Bank",
	},
	"Banque :": {
		DE: "Bank: ",
		IT: "Banca: ",
		EN: "Bank: ",
	},
	"Bénéficiaire :": {
		DE: "Zahlungsempfänger:",
		IT: "Beneficiario:",
		EN: "Beneficiary:",
	},
	"CO art. 957a al. 2 ch. 5 — traçabilité des écritures": {
		DE: "OR Art. 957a Abs. 2 Ziff. 5 — Nachvollziehbarkeit der Buchungen",
		IT: "CO art. 957a cpv. 2 n. 5 — tracciabilità delle registrazioni",
		EN: "CO art. 957a para. 2 no. 5 — traceability of entries",
	},
	"Ce document a été modifié après son émission": {
		DE: "Dieses Dokument wurde nach seiner Ausstellung verändert",
		IT: "Questo documento è stato modificato dopo la sua emissione",
		EN: "This document was altered after it was issued",
	},
	"Ces fichiers CSV contiennent les mêmes données que les fichiers JSON\n": {
		DE: "Diese CSV-Dateien enthalten dieselben Daten wie die JSON-Dateien\n",
		IT: "Questi file CSV contengono gli stessi dati dei file JSON\n",
		EN: "These CSV files contain the same data as the JSON files\n",
	},
	"Cette attestation est produite par le logiciel lui-même. Elle documente l'état d'un mécanisme technique ; elle ne remplace ni un contrôle de révision, ni l'avis d'une fiduciaire.": {
		DE: "Diese Bescheinigung wird von der Software selbst erstellt. Sie dokumentiert den Zustand eines technischen Verfahrens; sie ersetzt weder eine Revision noch die Beurteilung einer Treuhandstelle.",
		IT: "Questa attestazione è prodotta dal software stesso. Documenta lo stato di un meccanismo tecnico; non sostituisce né un controllo di revisione, né il parere di un fiduciario.",
		EN: "This attestation is produced by the software itself. It documents the state of a technical mechanism; it replaces neither an audit nor a trustee’s opinion.",
	},
	"Cette attestation ne certifie donc PAS l'intégrité de la comptabilité. Elle constate et documente une rupture.": {
		DE: "Diese Bescheinigung bestätigt die Integrität der Buchhaltung also NICHT. Sie stellt einen Bruch fest und dokumentiert ihn.",
		IT: "Questa attestazione NON certifica dunque l’integrità della contabilità. Constata e documenta una rottura.",
		EN: "This attestation therefore does NOT certify the integrity of the accounts. It records and documents a break.",
	},
	"Chaque entrée dérive par SHA-256 de la précédente. Aucune entrée n'a été modifiée, retirée ou réordonnée entre la première et la dernière.": {
		DE: "Jeder Eintrag leitet sich per SHA-256 aus dem vorherigen ab. Zwischen dem ersten und dem letzten wurde kein Eintrag verändert, entfernt oder umgestellt.",
		IT: "Ogni voce deriva per SHA-256 dalla precedente. Nessuna voce è stata modificata, rimossa o riordinata tra la prima e l’ultima.",
		EN: "Each entry derives by SHA-256 from the previous one. No entry was altered, removed or reordered between the first and the last.",
	},
	"Clé créée. La base sera chiffrée au prochain démarrage de LedgerAlps — la conversion remplace le fichier que le serveur a ouvert, elle ne peut pas se faire maintenant.": {
		DE: "Schlüssel erstellt. Die Datenbank wird beim nächsten Start von LedgerAlps verschlüsselt — die Umwandlung ersetzt die vom Server geöffnete Datei und kann jetzt nicht erfolgen.",
		IT: "Chiave creata. La base sarà cifrata al prossimo avvio di LedgerAlps — la conversione sostituisce il file che il server ha aperto e non può avvenire adesso.",
		EN: "Key created. The database will be encrypted at the next start of LedgerAlps — the conversion replaces the file the server has open and cannot happen now.",
	},
	"Clé retrouvée et rescellée à ce compte. Redémarrez LedgerAlps.": {
		DE: "Schlüssel wiedergefunden und diesem Konto neu versiegelt. Starten Sie LedgerAlps neu.",
		IT: "Chiave ritrovata e risigillata su questo account. Riavviate LedgerAlps.",
		EN: "Key recovered and resealed to this account. Restart LedgerAlps.",
	},
	"Compte": {
		DE: "Konto",
		IT: "Conto",
		EN: "Account",
	},
	"Compte / Payable à": {
		DE: "Konto / Zahlbar an",
		IT: "Conto / Pagabile a",
		EN: "Account / Payable to",
	},
	"Compte créé. Ce mot de passe est TEMPORAIRE : la personne devra en choisir un autre à sa première connexion, et ne pourra rien faire avant.": {
		DE: "Konto erstellt. Dieses Kennwort ist TEMPORÄR: die Person muss bei der ersten Anmeldung ein anderes wählen und kann vorher nichts tun.",
		IT: "Account creato. Questa password è TEMPORANEA: la persona dovrà sceglierne un’altra al primo accesso e non potrà fare nulla prima.",
		EN: "Account created. This password is TEMPORARY: the person must choose another at their first login, and can do nothing before that.",
	},
	"Conditions": {
		DE: "Bedingungen",
		IT: "Condizioni",
		EN: "Terms",
	},
	"Coordonnées bancaires": {
		DE: "Bankverbindung",
		IT: "Coordinate bancarie",
		EN: "Bank details",
	},
	"Date": {
		DE: "Datum",
		IT: "Data",
		EN: "Date",
	},
	"Date:": {
		DE: "Datum:",
		IT: "Data:",
		EN: "Date:",
	},
	"Description": {
		DE: "Bezeichnung",
		IT: "Descrizione",
		EN: "Description",
	},
	"Devise:": {
		DE: "Währung:",
		IT: "Valuta:",
		EN: "Currency:",
	},
	"Entrées écrites avant la v1.4.6, dont l'empreinte propre n'est pas recalculable : ": {
		DE: "Vor v1.4.6 geschriebene Einträge, deren eigener Hashwert nicht neu berechenbar ist: ",
		IT: "Voci scritte prima della v1.4.6, la cui impronta propria non è ricalcolabile: ",
		EN: "Entries written before v1.4.6, whose own hash cannot be recomputed: ",
	},
	"Export de réversibilité — LedgerAlps\n": {
		DE: "Reversibilitätsexport — LedgerAlps\n",
		IT: "Esportazione di reversibilità — LedgerAlps\n",
		EN: "Reversibility export — LedgerAlps\n",
	},
	"FACTURE": {
		DE: "RECHNUNG",
		IT: "FATTURA",
		EN: "INVOICE",
	},
	"IBAN :": {
		DE: "IBAN: ",
		IT: "IBAN: ",
		EN: "IBAN: ",
	},
	// Un client (ou un contact « les deux ») devient le débiteur d'une
	// facture QR : sans adresse structurée complète, le bulletin de
	// versement suisse ne peut pas s'imprimer (SPC 0200 §4.2.2).
	"adresse incomplète pour un client (nécessaire à la facture QR) : %s manquant(e)": {
		DE: "unvollständige Adresse für einen Kunden (für die QR-Rechnung erforderlich): %s fehlt",
		IT: "indirizzo incompleto per un cliente (necessario per la fattura QR): %s mancante",
		EN: "incomplete address for a customer (required for the QR-bill): %s missing",
	},
	"IDE / N° TVA : ": {
		DE: "UID / MWST-Nr.: ",
		IT: "IDI / N. IVA: ",
		EN: "UID / VAT no.: ",
	},
	"IDE : ": {
		DE: "UID: ",
		IT: "IDI: ",
		EN: "UID: ",
	},
	"Informations supplémentaires": {
		DE: "Zusätzliche Informationen",
		IT: "Informazioni supplementari",
		EN: "Additional information",
	},
	"L'attestation couvre des écritures que cette comptabilité ne contient pas. Ce sont deux comptabilités différentes, ou les écritures ont été supprimées.": {
		DE: "Die Bescheinigung deckt Buchungen ab, die diese Buchhaltung nicht enthält. Es sind zwei verschiedene Buchhaltungen, oder die Buchungen wurden gelöscht.",
		IT: "L’attestazione copre registrazioni che questa contabilità non contiene. Sono due contabilità diverse, oppure le registrazioni sono state eliminate.",
		EN: "The attestation covers entries these accounts do not contain. Either these are two different sets of books, or the entries were deleted.",
	},
	"L'empreinte de tête ci-dessus résume l'état de la chaîne : la conserver permet d'établir ultérieurement qu'aucune des écritures couvertes n'a bougé depuis l'émission de cette attestation.": {
		DE: "Der obige Kopf-Hashwert fasst den Zustand der Kette zusammen: ihn aufzubewahren erlaubt später den Nachweis, dass sich keine der erfassten Buchungen seit Ausstellung dieser Bescheinigung verändert hat.",
		IT: "L’impronta di testa qui sopra riassume lo stato della catena: conservarla permette di stabilire in seguito che nessuna delle registrazioni coperte si è mossa dall’emissione di questa attestazione.",
		EN: "The head hash above summarises the state of the chain: keeping it lets you establish later that none of the covered entries has moved since this attestation was issued.",
	},
	"L'empreinte enregistrée au même numéro de séquence ne correspond plus. Une écriture couverte par cette attestation a été réécrite. Une sauvegarde antérieure est nécessaire pour établir ce qui a bougé.": {
		DE: "Der bei derselben Sequenznummer gespeicherte Hashwert stimmt nicht mehr. Eine von dieser Bescheinigung abgedeckte Buchung wurde umgeschrieben. Eine ältere Sicherung ist nötig, um festzustellen, was sich verändert hat.",
		IT: "L’impronta registrata allo stesso numero di sequenza non corrisponde più. Una registrazione coperta da questa attestazione è stata riscritta. Un backup anteriore è necessario per stabilire cosa si è mosso.",
		EN: "The hash stored at the same sequence number no longer matches. An entry covered by this attestation was rewritten. An earlier backup is needed to establish what moved.",
	},
	"L'horodatage provient de l'horloge du poste, non d'une autorité d'horodatage tierce. La chaîne établit l'ORDRE des enregistrements et leur cohérence, pas une date opposable au sens d'un horodatage qualifié (RFC 3161).": {
		DE: "Der Zeitstempel stammt von der Uhr des Rechners, nicht von einer dritten Zeitstempelstelle. Die Kette belegt die REIHENFOLGE der Aufzeichnungen und ihre Stimmigkeit, nicht ein einwendbares Datum im Sinne eines qualifizierten Zeitstempels (RFC 3161).",
		IT: "La marcatura temporale proviene dall’orologio della postazione, non da un’autorità di marcatura temporale terza. La catena stabilisce l’ORDINE delle registrazioni e la loro coerenza, non una data opponibile ai sensi di una marcatura temporale qualificata (RFC 3161).",
		EN: "The timestamp comes from the machine’s clock, not from a third-party timestamping authority. The chain establishes the ORDER of the records and their consistency, not a date enforceable in the sense of a qualified timestamp (RFC 3161).",
	},
	"La base sera réécrite en clair au prochain démarrage de LedgerAlps.": {
		DE: "Die Datenbank wird beim nächsten Start von LedgerAlps unverschlüsselt neu geschrieben.",
		IT: "La base sarà riscritta in chiaro al prossimo avvio di LedgerAlps.",
		EN: "The database will be rewritten unencrypted at the next start of LedgerAlps.",
	},
	"Le document est intact et les livres portent toujours la même empreinte au numéro de séquence attesté. Aucune écriture couverte n'a été modifiée depuis l'émission.": {
		DE: "Das Dokument ist unversehrt, und die Bücher tragen bei der bescheinigten Sequenznummer weiterhin denselben Hashwert. Keine abgedeckte Buchung wurde seit der Ausstellung verändert.",
		IT: "Il documento è intatto e i libri recano sempre la stessa impronta al numero di sequenza attestato. Nessuna registrazione coperta è stata modificata dall’emissione.",
		EN: "The document is intact and the books still carry the same hash at the attested sequence number. No covered entry has been altered since it was issued.",
	},
	"Le document est intact. Il a été émis alors qu'aucune écriture n'était encore comptabilisée : il n'y a donc rien à comparer. Une attestation ne devient probante qu'une fois les livres alimentés.": {
		DE: "Das Dokument ist unversehrt. Es wurde ausgestellt, als noch keine Buchung verbucht war: es gibt also nichts zu vergleichen. Eine Bescheinigung wird erst beweiskräftig, wenn die Bücher gefüllt sind.",
		IT: "Il documento è intatto. È stato emesso quando nessuna registrazione era ancora contabilizzata: non c’è dunque nulla da confrontare. Un’attestazione diventa probante solo una volta alimentati i libri.",
		EN: "The document is intact. It was issued when no entry had yet been posted: there is therefore nothing to compare. An attestation only becomes probative once the books are filled.",
	},
	"Le détail des ruptures figure ci-dessus. Une sauvegarde antérieure à la rupture est nécessaire pour rétablir les livres.": {
		DE: "Die Einzelheiten der Brüche stehen oben. Eine Sicherung von vor dem Bruch ist nötig, um die Bücher wiederherzustellen.",
		IT: "Il dettaglio delle rotture figura sopra. Un backup anteriore alla rottura è necessario per ripristinare i libri.",
		EN: "The details of the breaks appear above. A backup from before the break is needed to restore the books.",
	},
	"LedgerAlps redémarre pour appliquer les changements. Cette page va se recharger automatiquement.": {
		DE: "LedgerAlps startet neu, um die Änderungen zu übernehmen. Diese Seite lädt sich automatisch neu.",
		IT: "LedgerAlps si riavvia per applicare le modifiche. Questa pagina si ricaricherà automaticamente.",
		EN: "LedgerAlps is restarting to apply the changes. This page will reload automatically.",
	},
	"Les livres ne portent pas la séquence attestée": {
		DE: "Die Bücher tragen die bescheinigte Sequenz nicht",
		IT: "I libri non recano la sequenza attestata",
		EN: "The books do not carry the attested sequence",
	},
	"Les livres ont changé depuis cette attestation": {
		DE: "Die Bücher haben sich seit dieser Bescheinigung verändert",
		IT: "I libri sono cambiati da questa attestazione",
		EN: "The books have changed since this attestation",
	},
	"Les prochaines sauvegardes automatiques seront chiffrées.": {
		DE: "Die nächsten automatischen Sicherungen werden verschlüsselt.",
		IT: "I prossimi backup automatici saranno cifrati.",
		EN: "The next automatic backups will be encrypted.",
	},
	"Les écritures couvertes par cette attestation n'ont pas bougé. La chaîne a en revanche rompu plus loin : voyez Paramètres → Maintenance → Piste d'audit.": {
		DE: "Die von dieser Bescheinigung abgedeckten Buchungen haben sich nicht verändert. Die Kette ist hingegen weiter hinten gebrochen: siehe Einstellungen → Wartung → Prüfpfad.",
		IT: "Le registrazioni coperte da questa attestazione non si sono mosse. La catena si è invece rotta più avanti: vedete Impostazioni → Manutenzione → Pista di controllo.",
		EN: "The entries covered by this attestation have not moved. The chain has, however, broken further on: see Settings → Maintenance → Audit trail.",
	},
	"Message": {
		DE: "Mitteilung",
		IT: "Messaggio",
		EN: "Message",
	},
	"Monnaie": {
		DE: "Währung",
		IT: "Valuta",
		EN: "Currency",
	},
	"Montant": {
		DE: "Betrag",
		IT: "Importo",
		EN: "Amount",
	},
	"Mot de passe changé. Les autres sessions de ce compte ont été fermées.": {
		DE: "Kennwort geändert. Die anderen Sitzungen dieses Kontos wurden beendet.",
		IT: "Password cambiata. Le altre sessioni di questo account sono state chiuse.",
		EN: "Password changed. The other sessions of this account have been closed.",
	},
	"NOTE DE CRÉDIT": {
		DE: "GUTSCHRIFT",
		IT: "NOTA DI CREDITO",
		EN: "CREDIT NOTE",
	},
	"Nouvelle phrase de récupération enregistrée. L'ancienne ne fonctionne plus. Notez celle-ci ailleurs que sur cet ordinateur.": {
		DE: "Neue Wiederherstellungs-Passphrase gespeichert. Die alte funktioniert nicht mehr. Notieren Sie diese ausserhalb dieses Rechners.",
		IT: "Nuova passphrase di recupero registrata. La vecchia non funziona più. Annotate questa altrove che su questo computer.",
		EN: "New recovery passphrase saved. The old one no longer works. Write this one down somewhere other than this computer.",
	},
	"N° TVA : ": {
		DE: "MWST-Nr.: ",
		IT: "N. IVA: ",
		EN: "VAT no.: ",
	},
	"N° facture:": {
		DE: "Rechnungsnr.:",
		IT: "N. fattura:",
		EN: "Invoice no.:",
	},
	"N° note de crédit:": {
		DE: "Gutschriftsnr.:",
		IT: "N. nota di credito:",
		EN: "Credit note no.:",
	},
	"N° offre:": {
		DE: "Offertennr.:",
		IT: "N. offerta:",
		EN: "Quotation no.:",
	},
	"OFFRE DE PRIX": {
		DE: "OFFERTE",
		IT: "OFFERTA",
		EN: "QUOTATION",
	},
	"Page %d/{nb}": {
		DE: "Seite %d/{nb}",
		IT: "Pagina %d/{nb}",
		EN: "Page %d/{nb}",
	},
	"Paiement par virement bancaire": {
		DE: "Zahlung per Banküberweisung",
		IT: "Pagamento tramite bonifico bancario",
		EN: "Payment by bank transfer",
	},
	"Partie paiement": {
		DE: "Zahlteil",
		IT: "Sezione pagamento",
		EN: "Payment part",
	},
	"Payable par": {
		DE: "Zahlbar durch",
		IT: "Pagabile da",
		EN: "Payable by",
	},
	"Payable par (nom/adresse)": {
		DE: "Zahlbar durch (Name/Adresse)",
		IT: "Pagabile da (nome/indirizzo)",
		EN: "Payable by (name/address)",
	},
	"Phrase de passe retirée. Les prochaines sauvegardes seront écrites en clair. Les copies déjà chiffrées le restent et exigent toujours l'ancienne phrase.": {
		DE: "Passphrase entfernt. Die nächsten Sicherungen werden unverschlüsselt geschrieben. Die bereits verschlüsselten Kopien bleiben es und verlangen weiterhin die alte Passphrase.",
		IT: "Passphrase rimossa. I prossimi backup saranno scritti in chiaro. Le copie già cifrate restano tali e richiedono sempre la vecchia passphrase.",
		EN: "Passphrase removed. The next backups will be written unencrypted. The copies already encrypted stay so and still require the old passphrase.",
	},
	"Point de dépôt": {
		DE: "Annahmestelle",
		IT: "Punto di accettazione",
		EN: "Acceptance point",
	},
	"Prix": {
		DE: "Preis",
		IT: "Prezzo",
		EN: "Price",
	},
	"Prix unit.": {
		DE: "Einzelpreis",
		IT: "Prezzo un.",
		EN: "Unit price",
	},
	"QR-IBAN IID %d is not in the QR-IID range (30000–31999)": {
		DE: "Die QR-IID %d liegt nicht im QR-IID-Bereich (30000–31999)",
		IT: "L’IID %d del QR-IBAN non rientra nell’intervallo QR-IID (30000–31999)",
		EN: "QR-IBAN IID %d is not in the QR-IID range (30000–31999)",
	},
	"QR-IBAN IID is not numeric": {
		DE: "Die QR-IID ist nicht numerisch",
		IT: "L’IID del QR-IBAN non è numerico",
		EN: "The QR-IBAN IID is not numeric",
	},
	"QR-IBAN cannot be used for %s invoices (QRR is CHF-only) and no regular IBAN is configured": {
		DE: "Eine QR-IBAN lässt sich für Rechnungen in %s nicht verwenden (QRR gilt nur für CHF), und es ist keine gewöhnliche IBAN hinterlegt",
		IT: "Un QR-IBAN non può essere usato per fatture in %s (QRR vale solo per CHF) e nessun IBAN ordinario è configurato",
		EN: "A QR-IBAN cannot be used for %s invoices (QRR is CHF-only) and no regular IBAN is configured",
	},
	"QR-IBAN must be a Swiss IBAN (CH prefix)": {
		DE: "Eine QR-IBAN muss eine Schweizer IBAN sein (Präfix CH)",
		IT: "Un QR-IBAN dev’essere un IBAN svizzero (prefisso CH)",
		EN: "A QR-IBAN must be a Swiss IBAN (CH prefix)",
	},
	"QR-IBAN too short": {
		DE: "QR-IBAN zu kurz",
		IT: "QR-IBAN troppo corto",
		EN: "QR-IBAN too short",
	},
	"QRR digits must be numeric, got %q": {
		DE: "Die QRR-Ziffern müssen numerisch sein, erhalten %q",
		IT: "Le cifre QRR devono essere numeriche, ricevuto %q",
		EN: "The QRR digits must be numeric, got %q",
	},
	"QRR reference check digit invalid (expected %d, got %d)": {
		DE: "Prüfziffer der QRR-Referenz ungültig (erwartet %d, erhalten %d)",
		IT: "Cifra di controllo del riferimento QRR non valida (attesa %d, ricevuta %d)",
		EN: "QRR reference check digit invalid (expected %d, got %d)",
	},
	"QRR reference must be 27 digits, got %d": {
		DE: "Die QRR-Referenz muss 27 Ziffern zählen, erhalten %d",
		IT: "Il riferimento QRR deve contare 27 cifre, ricevute %d",
		EN: "The QRR reference must be 27 digits, got %d",
	},
	"QRR reference must be numeric": {
		DE: "Die QRR-Referenz muss numerisch sein",
		IT: "Il riferimento QRR dev’essere numerico",
		EN: "The QRR reference must be numeric",
	},
	"Qté": {
		DE: "Menge",
		IT: "Qtà",
		EN: "Qty",
	},
	"Rabais": {
		DE: "Rabatt",
		IT: "Sconto",
		EN: "Discount",
	},
	"Remarques": {
		DE: "Bemerkungen",
		IT: "Osservazioni",
		EN: "Notes",
	},
	"Restauration préparée et vérifiée. Votre comptabilité actuelle a été sauvegardée. La restauration sera appliquée au redémarrage de LedgerAlps.": {
		DE: "Wiederherstellung vorbereitet und geprüft. Ihre aktuelle Buchhaltung wurde gesichert. Die Wiederherstellung wird beim Neustart von LedgerAlps ausgeführt.",
		IT: "Ripristino preparato e verificato. La vostra contabilità attuale è stata salvata. Il ripristino sarà applicato al riavvio di LedgerAlps.",
		EN: "Restore prepared and verified. Your current accounts have been backed up. The restore will be applied when LedgerAlps restarts.",
	},
	"Récépissé": {
		DE: "Empfangsschein",
		IT: "Ricevuta",
		EN: "Receipt",
	},
	"Référence": {
		DE: "Referenz",
		IT: "Riferimento",
		EN: "Reference",
	},
	"Référence QR": {
		DE: "QR-Referenz",
		IT: "Riferimento QR",
		EN: "QR reference",
	},
	"Réglages enregistrés. Ils prennent effet au redémarrage de LedgerAlps : l'adresse d'écoute et le chiffrement sont choisis une seule fois, au démarrage.": {
		DE: "Einstellungen gespeichert. Sie werden beim Neustart von LedgerAlps wirksam: Lauschadresse und Verschlüsselung werden einmalig beim Start gewählt.",
		IT: "Impostazioni registrate. Hanno effetto al riavvio di LedgerAlps: l’indirizzo di ascolto e la cifratura si scelgono una sola volta, all’avvio.",
		EN: "Settings saved. They take effect when LedgerAlps restarts: the listening address and encryption are chosen once, at start-up.",
	},
	"Rôle mis à jour. Il s'applique immédiatement : les droits sont relus à chaque requête, sans attendre l'expiration d'une session.": {
		DE: "Rolle aktualisiert. Sie gilt sofort: die Rechte werden bei jeder Anfrage neu gelesen, ohne das Ende einer Sitzung abzuwarten.",
		IT: "Ruolo aggiornato. Si applica immediatamente: i diritti sono riletti a ogni richiesta, senza attendere la scadenza di una sessione.",
		EN: "Role updated. It applies immediately: rights are re-read on every request, without waiting for a session to expire.",
	},
	"SCOR reference check digits invalid (modulo 97-10)": {
		DE: "Prüfziffern der SCOR-Referenz ungültig (Modulo 97-10)",
		IT: "Cifre di controllo del riferimento SCOR non valide (modulo 97-10)",
		EN: "SCOR reference check digits invalid (modulo 97-10)",
	},
	"SCOR reference must be 5–25 characters, got %d": {
		DE: "Die SCOR-Referenz muss 5–25 Zeichen zählen, erhalten %d",
		IT: "Il riferimento SCOR deve contare 5–25 caratteri, ricevuti %d",
		EN: "The SCOR reference must be 5–25 characters, got %d",
	},
	"SCOR reference must be alphanumeric, got %q": {
		DE: "Die SCOR-Referenz muss alphanumerisch sein, erhalten %q",
		IT: "Il riferimento SCOR dev’essere alfanumerico, ricevuto %q",
		EN: "The SCOR reference must be alphanumeric, got %q",
	},
	"SCOR reference must start with %q": {
		DE: "Die SCOR-Referenz muss mit %q beginnen",
		IT: "Il riferimento SCOR deve iniziare con %q",
		EN: "The SCOR reference must start with %q",
	},
	"SCOR reference type requires a reference value": {
		DE: "Der Referenztyp SCOR verlangt einen Referenzwert",
		IT: "Il tipo di riferimento SCOR richiede un valore di riferimento",
		EN: "Reference type SCOR requires a reference value",
	},
	"Second facteur retiré.": {
		DE: "Zweiter Faktor entfernt.",
		IT: "Secondo fattore rimosso.",
		EN: "Second factor removed.",
	},
	"Second facteur retiré. Si ce compte est administrateur, il devra en inscrire un nouveau avant de pouvoir travailler.": {
		DE: "Zweiter Faktor entfernt. Ist dieses Konto ein Administrator, muss es einen neuen einrichten, bevor es arbeiten kann.",
		IT: "Secondo fattore rimosso. Se questo account è amministratore, dovrà attivarne uno nuovo prima di poter lavorare.",
		EN: "Second factor removed. If this account is an administrator, it must set up a new one before it can work.",
	},
	"Secret de signature régénéré. Il prend effet au redémarrage de LedgerAlps, après quoi toutes les sessions devront se reconnecter. Vos mots de passe, vos données et vos sauvegardes ne sont pas affectés.": {
		DE: "Signaturschlüssel erneuert. Er wird beim Neustart von LedgerAlps wirksam, danach müssen sich alle Sitzungen neu anmelden. Ihre Kennwörter, Ihre Daten und Ihre Sicherungen sind nicht betroffen.",
		IT: "Segreto di firma rigenerato. Ha effetto al riavvio di LedgerAlps, dopo di che tutte le sessioni dovranno riconnettersi. Le vostre password, i vostri dati e i vostri backup non sono interessati.",
		EN: "Signing secret regenerated. It takes effect when LedgerAlps restarts, after which every session must sign in again. Your passwords, your data and your backups are unaffected.",
	},
	"Section paiement": {
		DE: "Zahlteil",
		IT: "Sezione pagamento",
		EN: "Payment part",
	},
	"Solde CHF": {
		DE: "Saldo CHF",
		IT: "Saldo CHF",
		EN: "Balance CHF",
	},
	"Solde cumule CHF": {
		DE: "Kumulierter Saldo CHF",
		IT: "Saldo cumulato CHF",
		EN: "Running balance CHF",
	},
	"Son sceau ne correspond plus à son contenu. Demandez une attestation neuve : celle-ci ne prouve rien.": {
		DE: "Sein Siegel stimmt nicht mehr mit seinem Inhalt überein. Verlangen Sie eine neue Bescheinigung: diese beweist nichts.",
		IT: "Il suo sigillo non corrisponde più al suo contenuto. Chiedete un’attestazione nuova: questa non prova nulla.",
		EN: "Its seal no longer matches its content. Ask for a fresh attestation: this one proves nothing.",
	},
	"Sous-total:": {
		DE: "Zwischentotal:",
		IT: "Subtotale:",
		EN: "Subtotal:",
	},
	"TOTAL ": {
		DE: "TOTAL ",
		IT: "TOTALE ",
		EN: "TOTAL ",
	},
	"TVA": {
		DE: "MWST",
		IT: "IVA",
		EN: "VAT",
	},
	"TVA %.1f%%:": {
		DE: "MWST %.1f%%:",
		IT: "IVA %.1f%%:",
		EN: "VAT %.1f%%:",
	},
	"TVA%": {
		DE: "MWST%",
		IT: "IVA%",
		EN: "VAT%",
	},
	"Total": {
		DE: "Total",
		IT: "Totale",
		EN: "Total",
	},
	"Tous les ordinateurs de confiance ont été oubliés. Un code sera redemandé à la prochaine connexion, sur chacun d'eux.": {
		DE: "Alle vertrauenswürdigen Rechner wurden vergessen. Bei der nächsten Anmeldung wird auf jedem von ihnen wieder ein Code verlangt.",
		IT: "Tutti i computer attendibili sono stati dimenticati. Un codice sarà richiesto di nuovo al prossimo accesso, su ciascuno di essi.",
		EN: "All trusted computers have been forgotten. A code will be asked again at the next login, on each of them.",
	},
	"Une troncature en FIN de chaîne n'est pas détectable : rien ne distingue des écritures effacées après la dernière d'écritures jamais passées. Seule la comparaison avec une sauvegarde répond à cette question.": {
		DE: "Ein Abschneiden am ENDE der Kette ist nicht erkennbar: nichts unterscheidet nach der letzten gelöschte Buchungen von nie erfassten. Nur der Vergleich mit einer Sicherung beantwortet diese Frage.",
		IT: "Un troncamento alla FINE della catena non è rilevabile: nulla distingue registrazioni cancellate dopo l’ultima da registrazioni mai effettuate. Solo il confronto con un backup risponde a questa domanda.",
		EN: "A truncation at the END of the chain is not detectable: nothing tells entries erased after the last one from entries never made. Only comparison with a backup answers that question.",
	},
	"Valable jusqu'au:": {
		DE: "Gültig bis:",
		IT: "Valida fino al:",
		EN: "Valid until:",
	},
	"account 5900 (compte de résultat) not found in chart of accounts": {
		DE: "Konto 5900 (Erfolgsrechnung) fehlt im Kontenplan",
		IT: "il conto 5900 (conto economico) è assente dal piano dei conti",
		EN: "account 5900 (income statement) is missing from the chart of accounts",
	},
	"action inconnue: %q": {
		DE: "Unbekannte Aktion: %q",
		IT: "Azione sconosciuta: %q",
		EN: "Unknown action: %q",
	},
	"as_of doit être au format AAAA-MM-JJ": {
		DE: "as_of muss im Format JJJJ-MM-TT vorliegen",
		IT: "as_of dev’essere nel formato AAAA-MM-GG",
		EN: "as_of must be in YYYY-MM-DD format",
	},
	"aucun IBAN n'est configuré : le bulletin de versement ne peut pas être produit": {
		DE: "Es ist keine IBAN hinterlegt: der Zahlteil kann nicht erzeugt werden",
		IT: "Nessun IBAN è configurato: la polizza di versamento non può essere prodotta",
		EN: "No IBAN is configured: the payment part cannot be produced",
	},
	"aucun QR-facture n'a été trouvé dans ce document": {
		DE: "In diesem Dokument wurde keine QR-Rechnung gefunden",
		IT: "In questo documento non è stata trovata alcuna fattura QR",
		EN: "No QR-bill was found in this document",
	},
	"aucun des documents demandés n'existe": {
		DE: "Keines der angeforderten Dokumente existiert",
		IT: "Nessuno dei documenti richiesti esiste",
		EN: "None of the requested documents exists",
	},
	"aucun document sélectionné": {
		DE: "Kein Dokument ausgewählt",
		IT: "Nessun documento selezionato",
		EN: "No document selected",
	},
	"aucun fichier reçu (champ « file » attendu)": {
		DE: "Keine Datei empfangen (Feld «file» erwartet)",
		IT: "Nessun file ricevuto (campo «file» atteso)",
		EN: "No file received (field “file” expected)",
	},
	"aucun jeton de rafraîchissement fourni": {
		DE: "Kein Aktualisierungstoken übermittelt",
		IT: "Nessun token di aggiornamento fornito",
		EN: "No refresh token provided",
	},
	"aucun maillon d'audit pour l'écriture comptabilisée %s": {
		DE: "Kein Prüfglied für die verbuchte Buchung %s",
		IT: "Nessun anello di controllo per la registrazione contabilizzata %s",
		EN: "No audit link for posted entry %s",
	},
	"aucun numéro de TVA n'est enregistré : vous ne pouvez pas facturer de TVA. Si vous êtes assujetti, saisissez-le dans Paramètres → Identité ; sinon, passez les lignes à 0 % — la LTVA art. 27 al. 1 interdit de faire figurer l'impôt, et l'al. 2 vous en rend redevable": {
		DE: "Es ist keine MWST-Nummer hinterlegt: Sie dürfen keine MWST fakturieren. Sind Sie steuerpflichtig, erfassen Sie sie unter Einstellungen → Identität; andernfalls setzen Sie die Positionen auf 0 % — MWSTG Art. 27 Abs. 1 verbietet den Ausweis der Steuer, und Abs. 2 macht Sie dafür zahlungspflichtig",
		IT: "Nessun numero IVA è registrato: non potete fatturare l’IVA. Se siete assoggettati, inseritelo in Impostazioni → Identità; altrimenti portate le righe a 0 % — la LIVA art. 27 cpv. 1 vieta di indicare l’imposta e il cpv. 2 ve ne rende debitori",
		EN: "No VAT number is registered: you may not charge VAT. If you are liable, enter it under Settings → Identity; otherwise set the lines to 0 % — VAT Act art. 27 para. 1 forbids showing the tax, and para. 2 makes you liable for it",
	},
	"aucun second facteur n'est actif sur ce compte": {
		DE: "Auf diesem Konto ist kein zweiter Faktor aktiv",
		IT: "Nessun secondo fattore è attivo su questo account",
		EN: "No second factor is active on this account",
	},
	"aucune conversion en attente": {
		DE: "Keine ausstehende Umwandlung",
		IT: "Nessuna conversione in attesa",
		EN: "No pending conversion",
	},
	"aucune facture sélectionnée": {
		DE: "Keine Rechnung ausgewählt",
		IT: "Nessuna fattura selezionata",
		EN: "No invoice selected",
	},
	"aucune phrase de passe: rien à chiffrer": {
		DE: "Keine Passphrase: nichts zu verschlüsseln",
		IT: "Nessuna passphrase: nulla da cifrare",
		EN: "No passphrase: nothing to encrypt",
	},
	"aucune phrase de récupération n'a été enregistrée pour cette base": {
		DE: "Für diese Datenbank wurde keine Wiederherstellungs-Passphrase hinterlegt",
		IT: "Nessuna passphrase di recupero è stata registrata per questa base",
		EN: "No recovery passphrase has been stored for this database",
	},
	"aucune restauration en attente": {
		DE: "Keine ausstehende Wiederherstellung",
		IT: "Nessun ripristino in attesa",
		EN: "No pending restore",
	},
	"c'est le dernier administrateur : le retirer rendrait cette installation inadministrable, sans possibilité de créer un compte, de restaurer une sauvegarde ou de rendre le droit de le faire": {
		DE: "Das ist der letzte Administrator: ihn zu entfernen machte diese Installation unverwaltbar, ohne Möglichkeit, ein Konto anzulegen, eine Sicherung wiederherzustellen oder das Recht dazu zurückzugeben",
		IT: "È l’ultimo amministratore: rimuoverlo renderebbe questa installazione ingestibile, senza possibilità di creare un account, ripristinare un backup o restituire il diritto di farlo",
		EN: "This is the last administrator: removing them would make this installation unmanageable, with no way to create an account, restore a backup or grant the right to do so",
	},
	"cannot close fiscal year %q: %d draft journal entries must be posted or deleted first": {
		DE: "Das Geschäftsjahr %q lässt sich nicht abschliessen: %d Buchungsentwürfe müssen zuerst verbucht oder gelöscht werden",
		IT: "Impossibile chiudere l’esercizio %q: %d bozze di registrazione devono prima essere contabilizzate o eliminate",
		EN: "Cannot close financial year %q: %d draft entries must be posted or deleted first",
	},
	"caractère non autorisé dans un IBAN : %q": {
		DE: "Unzulässiges Zeichen in einer IBAN: %q",
		IT: "Carattere non autorizzato in un IBAN: %q",
		EN: "Character not allowed in an IBAN: %q",
	},
	"ce bulletin porte une référence QR mais l'IBAN %s n'est pas un QR-IBAN : le document est incohérent": {
		DE: "Dieser Zahlteil trägt eine QR-Referenz, aber die IBAN %s ist keine QR-IBAN: das Dokument ist widersprüchlich",
		IT: "Questa polizza reca un riferimento QR ma l’IBAN %s non è un QR-IBAN: il documento è incoerente",
		EN: "This payment part carries a QR reference but IBAN %s is not a QR-IBAN: the document is inconsistent",
	},
	"ce bulletin porte une référence créancière mais l'IBAN %s est un QR-IBAN, qui n'accepte qu'une référence QR": {
		DE: "Dieser Zahlteil trägt eine Creditor Reference, aber die IBAN %s ist eine QR-IBAN, die nur eine QR-Referenz akzeptiert",
		IT: "Questa polizza reca un riferimento creditore ma l’IBAN %s è un QR-IBAN, che accetta soltanto un riferimento QR",
		EN: "This payment part carries a creditor reference but IBAN %s is a QR-IBAN, which accepts only a QR reference",
	},
	"ce code ne correspond pas. Vérifiez l'heure de l'appareil qui porte votre application : un décalage de plus d'une minute suffit à décaler tous les codes.": {
		DE: "Dieser Code stimmt nicht. Prüfen Sie die Uhrzeit des Geräts mit Ihrer App: eine Abweichung von mehr als einer Minute genügt, um alle Codes zu verschieben.",
		IT: "Questo codice non corrisponde. Verificate l’ora del dispositivo che ospita la vostra app: uno scarto di più di un minuto basta a sfasare tutti i codici.",
		EN: "This code does not match. Check the clock of the device holding your app: a drift of more than a minute is enough to shift every code.",
	},
	"ce compte est désactivé": {
		DE: "Dieses Konto ist deaktiviert",
		IT: "Questo account è disattivato",
		EN: "This account is deactivated",
	},
	"ce compte est désactivé : réactivez-le d'abord si vous voulez lui rendre l'accès": {
		DE: "Dieses Konto ist deaktiviert: reaktivieren Sie es zuerst, wenn Sie ihm den Zugang zurückgeben wollen",
		IT: "Questo account è disattivato: riattivatelo prima se volete restituirgli l’accesso",
		EN: "This account is deactivated: reactivate it first if you want to give it access back",
	},
	"ce compte n'est plus actif": {
		DE: "Dieses Konto ist nicht mehr aktiv",
		IT: "Questo account non è più attivo",
		EN: "This account is no longer active",
	},
	"ce compte n'est plus disponible": {
		DE: "Dieses Konto ist nicht mehr verfügbar",
		IT: "Questo account non è più disponibile",
		EN: "This account is no longer available",
	},
	"ce contact a déjà été anonymisé le ": {
		DE: "Dieser Kontakt wurde bereits anonymisiert am ",
		IT: "Questo contatto è già stato anonimizzato il ",
		EN: "This contact was already anonymised on ",
	},
	"ce fichier n'est pas une attestation LedgerAlps": {
		DE: "Diese Datei ist keine LedgerAlps-Bescheinigung",
		IT: "Questo file non è un’attestazione LedgerAlps",
		EN: "This file is not a LedgerAlps attestation",
	},
	"ce fichier n'est pas une image lisible (PNG ou JPEG attendu)": {
		DE: "Diese Datei ist kein lesbares Bild (PNG oder JPEG erwartet)",
		IT: "Questo file non è un’immagine leggibile (atteso PNG o JPEG)",
		EN: "This file is not a readable image (PNG or JPEG expected)",
	},
	"ce fichier n'est pas une sauvegarde chiffrée": {
		DE: "Diese Datei ist keine verschlüsselte Sicherung",
		IT: "Questo file non è un backup cifrato",
		EN: "This file is not an encrypted backup",
	},
	"ce fournisseur a déjà une facture portant cette référence": {
		DE: "Dieser Lieferant hat bereits eine Rechnung mit dieser Referenz",
		IT: "Questo fornitore ha già una fattura con questo riferimento",
		EN: "This supplier already has an invoice with this reference",
	},
	"ce jeton de rafraîchissement a expiré": {
		DE: "Dieses Aktualisierungstoken ist abgelaufen",
		IT: "Questo token di aggiornamento è scaduto",
		EN: "This refresh token has expired",
	},
	"ce jeton de rafraîchissement a été révoqué": {
		DE: "Dieses Aktualisierungstoken wurde widerrufen",
		IT: "Questo token di aggiornamento è stato revocato",
		EN: "This refresh token was revoked",
	},
	"cette adresse e-mail est déjà enregistrée": {
		DE: "Diese E-Mail-Adresse ist bereits erfasst",
		IT: "Questo indirizzo e-mail è già registrato",
		EN: "This e-mail address is already registered",
	},
	"cette adresse e-mail est déjà utilisée": {
		DE: "Diese E-Mail-Adresse wird bereits verwendet",
		IT: "Questo indirizzo e-mail è già utilizzato",
		EN: "This e-mail address is already in use",
	},
	"cette base a déjà une clé": {
		DE: "Diese Datenbank hat bereits einen Schlüssel",
		IT: "Questa base ha già una chiave",
		EN: "This database already has a key",
	},
	"cette base n'est pas chiffrée": {
		DE: "Diese Datenbank ist nicht verschlüsselt",
		IT: "Questa base non è cifrata",
		EN: "This database is not encrypted",
	},
	"cette facture est déjà comptabilisée : son écriture est scellée (CO art. 957a) et la modifier ferait mentir le journal. Passez par une écriture de correction.": {
		DE: "Diese Rechnung ist bereits verbucht: ihre Buchung ist versiegelt (OR Art. 957a), und sie zu ändern würde das Journal lügen lassen. Nehmen Sie eine Korrekturbuchung vor.",
		IT: "Questa fattura è già contabilizzata: la sua registrazione è sigillata (CO art. 957a) e modificarla farebbe mentire il giornale. Effettuate una registrazione di correzione.",
		EN: "This invoice is already posted: its entry is sealed (CO art. 957a) and changing it would make the journal lie. Use a correcting entry instead.",
	},
	"cette facture fournisseur est déjà enregistrée (même fournisseur, même référence)": {
		DE: "Diese Lieferantenrechnung ist bereits erfasst (gleicher Lieferant, gleiche Referenz)",
		IT: "Questa fattura fornitore è già registrata (stesso fornitore, stesso riferimento)",
		EN: "This supplier invoice is already recorded (same supplier, same reference)",
	},
	"cette offre a déjà été convertie en facture": {
		DE: "Diese Offerte wurde bereits in eine Rechnung umgewandelt",
		IT: "Questa offerta è già stata convertita in fattura",
		EN: "This quote has already been turned into an invoice",
	},
	"cette plateforme ne peut pas sceller un secret au compte": {
		DE: "Diese Plattform kann ein Geheimnis nicht an das Konto versiegeln",
		IT: "Questa piattaforma non può sigillare un segreto sull’account",
		EN: "This platform cannot seal a secret to the account",
	},
	"cette sauvegarde est chiffrée: indiquez la phrase de passe (--passphrase)": {
		DE: "Diese Sicherung ist verschlüsselt: geben Sie die Passphrase an (--passphrase)",
		IT: "Questo backup è cifrato: indicate la passphrase (--passphrase)",
		EN: "This backup is encrypted: give the passphrase (--passphrase)",
	},
	"cette sauvegarde est chiffrée: la phrase de passe est requise": {
		DE: "Diese Sicherung ist verschlüsselt: die Passphrase ist erforderlich",
		IT: "Questo backup è cifrato: la passphrase è obbligatoria",
		EN: "This backup is encrypted: the passphrase is required",
	},
	"cette sauvegarde n'est pas chiffrée: aucune phrase de passe n'est attendue": {
		DE: "Diese Sicherung ist nicht verschlüsselt: es wird keine Passphrase erwartet",
		IT: "Questo backup non è cifrato: nessuna passphrase è attesa",
		EN: "This backup is not encrypted: no passphrase is expected",
	},
	"cette sauvegarde n'est pas chiffrée: retirez --passphrase": {
		DE: "Diese Sicherung ist nicht verschlüsselt: entfernen Sie --passphrase",
		IT: "Questo backup non è cifrato: rimuovete --passphrase",
		EN: "This backup is not encrypted: drop --passphrase",
	},
	"cette étape a expiré : recommencez la connexion": {
		DE: "Dieser Schritt ist abgelaufen: beginnen Sie die Anmeldung erneut",
		IT: "Questa fase è scaduta: ricominciate l’accesso",
		EN: "This step has expired: start the login again",
	},
	"cette étape suit une saisie de mot de passe": {
		DE: "Dieser Schritt folgt auf eine Kennworteingabe",
		IT: "Questa fase segue un’immissione di password",
		EN: "This step follows a password entry",
	},
	"cette étape suit une saisie de mot de passe : recommencez la connexion": {
		DE: "Dieser Schritt folgt auf eine Kennworteingabe: beginnen Sie die Anmeldung erneut",
		IT: "Questa fase segue un’immissione di password: ricominciate l’accesso",
		EN: "This step follows a password entry: start the login again",
	},
	"changez d'abord votre mot de passe temporaire": {
		DE: "Ändern Sie zuerst Ihr temporäres Kennwort",
		IT: "Cambiate prima la vostra password temporanea",
		EN: "Change your temporary password first",
	},
	"clé de %d octets, attendu %d": {
		DE: "Schlüssel von %d Byte, erwartet %d",
		IT: "Chiave di %d byte, attesi %d",
		EN: "Key of %d bytes, expected %d",
	},
	"clé de base de taille %d, attendu %d": {
		DE: "Datenbankschlüssel der Grösse %d, erwartet %d",
		IT: "Chiave di base di dimensione %d, attesi %d",
		EN: "Database key of size %d, expected %d",
	},
	"clé récupérée de taille %d, attendu %d": {
		DE: "Wiederhergestellter Schlüssel der Grösse %d, erwartet %d",
		IT: "Chiave recuperata di dimensione %d, attesi %d",
		EN: "Recovered key of size %d, expected %d",
	},
	"code incorrect": {
		DE: "Falscher Code",
		IT: "Codice errato",
		EN: "Wrong code",
	},
	"commencez par l'étape précédente : aucun secret n'a été préparé": {
		DE: "Beginnen Sie mit dem vorherigen Schritt: es wurde kein Geheimnis vorbereitet",
		IT: "Cominciate dalla fase precedente: nessun segreto è stato preparato",
		EN: "Start with the previous step: no secret has been prepared",
	},
	"compte introuvable": {
		DE: "Konto nicht gefunden",
		IT: "Conto non trovato",
		EN: "Account not found",
	},
	"confirmation requise: la base sera réécrite en clair sur ce disque": {
		DE: "Bestätigung erforderlich: die Datenbank wird auf dieser Platte unverschlüsselt neu geschrieben",
		IT: "Conferma richiesta: la base sarà riscritta in chiaro su questo disco",
		EN: "Confirmation required: the database will be rewritten unencrypted on this disk",
	},
	"confirmation requise: la restauration remplacera la comptabilité actuelle": {
		DE: "Bestätigung erforderlich: die Wiederherstellung ersetzt die aktuelle Buchhaltung",
		IT: "Conferma richiesta: il ripristino sostituirà la contabilità attuale",
		EN: "Confirmation required: the restore will replace the current accounts",
	},
	"confirmation requise: les prochaines sauvegardes seront écrites en clair": {
		DE: "Bestätigung erforderlich: die nächsten Sicherungen werden unverschlüsselt geschrieben",
		IT: "Conferma richiesta: i prossimi backup saranno scritti in chiaro",
		EN: "Confirmation required: the next backups will be written unencrypted",
	},
	"confirmez d'abord avoir noté cette phrase de passe ailleurs que sur cet ordinateur : sans elle, personne ne peut ouvrir vos sauvegardes, vous non plus": {
		DE: "Bestätigen Sie zuerst, diese Passphrase ausserhalb dieses Rechners notiert zu haben: ohne sie kann niemand Ihre Sicherungen öffnen, Sie auch nicht",
		IT: "Confermate innanzitutto di aver annotato questa passphrase altrove che su questo computer: senza di essa nessuno può aprire i vostri backup, nemmeno voi",
		EN: "First confirm that you have written this passphrase down somewhere other than this computer: without it nobody can open your backups, not even you",
	},
	"confirmez d'abord avoir noté la phrase de récupération ailleurs que sur cet ordinateur : elle est le seul moyen de rouvrir cette base depuis une autre machine": {
		DE: "Bestätigen Sie zuerst, die Wiederherstellungs-Passphrase ausserhalb dieses Rechners notiert zu haben: sie ist der einzige Weg, diese Datenbank von einer anderen Maschine aus zu öffnen",
		IT: "Confermate innanzitutto di aver annotato la passphrase di recupero altrove che su questo computer: è l’unico modo di riaprire questa base da un’altra macchina",
		EN: "First confirm that you have written the recovery passphrase down somewhere other than this computer: it is the only way to reopen this database from another machine",
	},
	"connexion incomplète : le second facteur n'a pas été validé": {
		DE: "Anmeldung unvollständig: der zweite Faktor wurde nicht bestätigt",
		IT: "Accesso incompleto: il secondo fattore non è stato convalidato",
		EN: "Login incomplete: the second factor was not validated",
	},
	"contact introuvable": {
		DE: "Kontakt nicht gefunden",
		IT: "Contatto non trovato",
		EN: "Contact not found",
	},
	"contrôle d'intégrité en échec : l'empreinte enregistrée ne correspond pas à celle recalculée (CO art. 957a)": {
		DE: "Integritätsprüfung fehlgeschlagen: der gespeicherte Hashwert stimmt nicht mit dem neu berechneten überein (OR Art. 957a)",
		IT: "Controllo d’integrità fallito: l’impronta registrata non corrisponde a quella ricalcolata (CO art. 957a)",
		EN: "Integrity check failed: the stored hash does not match the recomputed one (CO art. 957a)",
	},
	"conversion annulée": {
		DE: "Umwandlung abgebrochen",
		IT: "conversione annullata",
		EN: "conversion cancelled",
	},
	"creditor IBAN is required": {
		DE: "Die IBAN des Zahlungsempfängers ist erforderlich",
		IT: "L’IBAN del creditore è obbligatorio",
		EN: "The creditor IBAN is required",
	},
	"creditor IBAN must be 21 characters, got %d": {
		DE: "Die IBAN des Zahlungsempfängers muss 21 Zeichen zählen, erhalten %d",
		IT: "L’IBAN del creditore deve contare 21 caratteri, ricevuti %d",
		EN: "The creditor IBAN must be 21 characters, got %d",
	},
	"creditor IBAN must be Swiss or Liechtenstein (CH/LI), got %q": {
		DE: "Die IBAN des Zahlungsempfängers muss schweizerisch oder liechtensteinisch sein (CH/LI), erhalten %q",
		IT: "L’IBAN del creditore dev’essere svizzero o del Liechtenstein (CH/LI), ricevuto %q",
		EN: "The creditor IBAN must be Swiss or Liechtenstein (CH/LI), got %q",
	},
	"creditor country (ISO 3166-1 alpha-2) is required": {
		DE: "Das Land des Zahlungsempfängers (ISO 3166-1 alpha-2) ist erforderlich",
		IT: "Il paese del creditore (ISO 3166-1 alpha-2) è obbligatorio",
		EN: "The creditor country (ISO 3166-1 alpha-2) is required",
	},
	"creditor name exceeds 70 chars": {
		DE: "Der Name des Zahlungsempfängers überschreitet 70 Zeichen",
		IT: "Il nome del creditore supera 70 caratteri",
		EN: "The creditor name exceeds 70 characters",
	},
	"creditor name is required": {
		DE: "Der Name des Zahlungsempfängers ist erforderlich",
		IT: "Il nome del creditore è obbligatorio",
		EN: "The creditor name is required",
	},
	"creditor postal code is required (address type S)": {
		DE: "Die PLZ des Zahlungsempfängers ist erforderlich (Adresstyp S)",
		IT: "Il NPA del creditore è obbligatorio (indirizzo di tipo S)",
		EN: "The creditor postcode is required (address type S)",
	},
	"creditor town is required (address type S)": {
		DE: "Der Ort des Zahlungsempfängers ist erforderlich (Adresstyp S)",
		IT: "La località del creditore è obbligatoria (indirizzo di tipo S)",
		EN: "The creditor town is required (address type S)",
	},
	"currency must be CHF or EUR, got %q": {
		DE: "Die Währung muss CHF oder EUR sein, erhalten %q",
		IT: "La valuta dev’essere CHF o EUR, ricevuto %q",
		EN: "The currency must be CHF or EUR, got %q",
	},
	"dans un autre logiciel comptable. Le JSON reste la référence : il porte\n": {
		DE: "in einer anderen Buchhaltungssoftware. Das JSON bleibt massgebend: es trägt\n",
		IT: "in un altro software contabile. Il JSON resta il riferimento: porta\n",
		EN: "in another accounting package. The JSON remains authoritative: it carries\n",
	},
	"date_from doit être au format AAAA-MM-JJ": {
		DE: "date_from muss im Format JJJJ-MM-TT vorliegen",
		IT: "date_from dev’essere nel formato AAAA-MM-GG",
		EN: "date_from must be in YYYY-MM-DD format",
	},
	"date_to doit être au format AAAA-MM-JJ": {
		DE: "date_to muss im Format JJJJ-MM-TT vorliegen",
		IT: "date_to dev’essere nel formato AAAA-MM-GG",
		EN: "date_to must be in YYYY-MM-DD format",
	},
	"de cette archive, dans un format ouvrable par un tableur ou importable\n": {
		DE: "dieses Archivs, in einem Format, das sich in einer Tabellenkalkulation öffnen oder importieren lässt\n",
		IT: "di questo archivio, in un formato apribile con un foglio di calcolo o importabile\n",
		EN: "of this archive, in a format that opens in a spreadsheet or imports\n",
	},
	"debtor country (ISO 3166-1 alpha-2) is required when debtor is identified": {
		DE: "Das Land des Zahlungspflichtigen (ISO 3166-1 alpha-2) ist erforderlich, wenn dieser genannt wird",
		IT: "Il paese del debitore (ISO 3166-1 alpha-2) è obbligatorio quando il debitore è identificato",
		EN: "The debtor country (ISO 3166-1 alpha-2) is required when the debtor is identified",
	},
	"debtor postal code is required when debtor is identified": {
		DE: "Die PLZ des Zahlungspflichtigen ist erforderlich, wenn dieser genannt wird",
		IT: "Il NPA del debitore è obbligatorio quando il debitore è identificato",
		EN: "The debtor postcode is required when the debtor is identified",
	},
	"debtor town is required when debtor is identified": {
		DE: "Der Ort des Zahlungspflichtigen ist erforderlich, wenn dieser genannt wird",
		IT: "La località del debitore è obbligatoria quando il debitore è identificato",
		EN: "The debtor town is required when the debtor is identified",
	},
	"document introuvable": {
		DE: "Beleg nicht gefunden",
		IT: "Documento non trovato",
		EN: "Document not found",
	},
	"donc une suppression y reste détectable ; seule la question « le contenu de cette ligne a-t-il changé ? » est sans réponse.": {
		DE: "eine Löschung bleibt darin also erkennbar; nur die Frage «hat sich der Inhalt dieser Zeile geändert?» bleibt unbeantwortet.",
		IT: "quindi un’eliminazione vi resta rilevabile; solo la domanda «il contenuto di questa riga è cambiato?» resta senza risposta.",
		EN: "so a deletion stays detectable there; only the question “has the content of this line changed?” goes unanswered.",
	},
	"données base64 invalides": {
		DE: "Ungültige base64-Daten",
		IT: "Dati base64 non validi",
		EN: "Invalid base64 data",
	},
	"la date d'échéance doit être au format AAAA-MM-JJ": {
		DE: "Das Fälligkeitsdatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data di scadenza dev’essere nel formato AAAA-MM-GG",
		EN: "The due date must be in YYYY-MM-DD format",
	},
	"délai hors bornes: %d minutes (0 pour désactiver, 1440 au plus)": {
		DE: "Frist ausserhalb der Grenzen: %d Minuten (0 zum Deaktivieren, höchstens 1440)",
		IT: "Termine fuori limiti: %d minuti (0 per disattivare, 1440 al massimo)",
		EN: "Delay out of bounds: %d minutes (0 to disable, 1440 at most)",
	},
	"délai trop court: %d minute — en dessous de deux minutes, la déconnexion tombe pendant la lecture d'un document et la saisie en cours est perdue": {
		DE: "Frist zu kurz: %d Minute — unter zwei Minuten fällt die Abmeldung mitten in das Lesen eines Dokuments, und die laufende Erfassung geht verloren",
		IT: "Termine troppo breve: %d minuto — sotto i due minuti la disconnessione cade durante la lettura di un documento e l’inserimento in corso è perduto",
		EN: "Delay too short: %d minute — below two minutes the logout falls while a document is being read, and work in progress is lost",
	},
	"elle portait alors sur des valeurs qui n'étaient pas enregistrées. Leur chaînage est vérifié comme les autres, ": {
		DE: "er bezog sich damals auf Werte, die nicht gespeichert wurden. Ihre Verkettung wird wie bei den anderen geprüft, ",
		IT: "verteva allora su valori che non erano registrati. La loro concatenazione è verificata come le altre, ",
		EN: "it then covered values that were not stored. Their chaining is checked like the others, ",
	},
	"empty file": {
		DE: "Leere Datei",
		IT: "File vuoto",
		EN: "Empty file",
	},
	"la date de fin doit être au format AAAA-MM-JJ": {
		DE: "Das Enddatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data di fine dev’essere nel formato AAAA-MM-GG",
		EN: "The end date must be in YYYY-MM-DD format",
	},
	"envoyez le XML dans le corps de la requête (Content-Type: application/xml) ou dans le champ « file » d'un formulaire": {
		DE: "Senden Sie das XML im Anfragekörper (Content-Type: application/xml) oder im Feld «file» eines Formulars",
		IT: "Inviate l’XML nel corpo della richiesta (Content-Type: application/xml) o nel campo «file» di un modulo",
		EN: "Send the XML in the request body (Content-Type: application/xml) or in a form’s “file” field",
	},
	"erreur de base de données": {
		DE: "Datenbankfehler",
		IT: "Errore di base di dati",
		EN: "Database error",
	},
	"facture %s (%s) : %s": {
		DE: "Rechnung %s (%s): %s",
		IT: "Fattura %s (%s): %s",
		EN: "Invoice %s (%s): %s",
	},
	"facture fournisseur introuvable": {
		DE: "Lieferantenrechnung nicht gefunden",
		IT: "Fattura fornitore non trovata",
		EN: "Supplier invoice not found",
	},
	"facture introuvable": {
		DE: "Rechnung nicht gefunden",
		IT: "Fattura non trovata",
		EN: "Invoice not found",
	},
	"fichier introuvable: ": {
		DE: "Datei nicht gefunden: ",
		IT: "File non trovato: ",
		EN: "File not found: ",
	},
	"fiscal year %q is already closed": {
		DE: "Das Geschäftsjahr %q ist bereits abgeschlossen",
		IT: "L’esercizio %q è già chiuso",
		EN: "Financial year %q is already closed",
	},
	"fiscal year %q not found": {
		DE: "Geschäftsjahr %q nicht gefunden",
		IT: "Esercizio %q non trovato",
		EN: "Financial year %q not found",
	},
	"format IDE invalide": {
		DE: "Ungültiges UID-Format",
		IT: "Formato IDI non valido",
		EN: "Invalid UID format",
	},
	"format IDE invalide — attendu CHE-XXX.XXX.XXX": {
		DE: "Ungültiges UID-Format — erwartet CHE-XXX.XXX.XXX",
		IT: "Formato IDI non valido — atteso CHE-XXX.XXX.XXX",
		EN: "Invalid UID format — expected CHE-XXX.XXX.XXX",
	},
	"fournisseur introuvable": {
		DE: "Lieferant nicht gefunden",
		IT: "Fornitore non trovato",
		EN: "Supplier not found",
	},
	"group_by doit valoir year, month ou contact": {
		DE: "group_by muss year, month oder contact sein",
		IT: "group_by dev’essere year, month o contact",
		EN: "group_by must be year, month or contact",
	},
	"génération du QR: ": {
		DE: "QR-Erzeugung: ",
		IT: "Generazione del QR: ",
		EN: "QR generation: ",
	},
	"identifiants incorrects": {
		DE: "Falsche Anmeldedaten",
		IT: "Credenziali errate",
		EN: "Incorrect credentials",
	},
	"il manque : %s": {
		DE: "Es fehlt: %s",
		IT: "Manca: %s",
		EN: "Missing: %s",
	},
	"impossible de lire le dossier de sauvegarde": {
		DE: "Der Sicherungsordner kann nicht gelesen werden",
		IT: "Impossibile leggere la cartella dei backup",
		EN: "The backup folder cannot be read",
	},
	"impossible de modifier une facture avec un paiement enregistré": {
		DE: "Eine Rechnung mit erfasster Zahlung lässt sich nicht ändern",
		IT: "Impossibile modificare una fattura con un pagamento registrato",
		EN: "An invoice with a recorded payment cannot be changed",
	},
	"impossible de sauvegarder la comptabilité actuelle avant la restauration: ": {
		DE: "Die aktuelle Buchhaltung kann vor der Wiederherstellung nicht gesichert werden: ",
		IT: "Impossibile salvare la contabilità attuale prima del ripristino: ",
		EN: "Cannot back up the current accounts before the restore: ",
	},
	"invalid data URL": {
		DE: "Ungültige Daten-URL",
		IT: "URL di dati non valido",
		EN: "Invalid data URL",
	},
	"invalid or expired token": {
		DE: "Token ungültig oder abgelaufen",
		IT: "Token non valido o scaduto",
		EN: "Invalid or expired token",
	},
	"invalid status transition": {
		DE: "Ungültiger Statuswechsel",
		IT: "Transizione di stato non valida",
		EN: "Invalid status transition",
	},
	"invalid token claims": {
		DE: "Ungültige Token-Angaben",
		IT: "Attestazioni del token non valide",
		EN: "Invalid token claims",
	},
	"invoice ID too long: max 26 digits, got %d": {
		DE: "Rechnungsnummer zu lang: höchstens 26 Ziffern, erhalten %d",
		IT: "Numero di fattura troppo lungo: 26 cifre al massimo, ricevute %d",
		EN: "Invoice number too long: 26 digits at most, got %d",
	},
	"invoice must have at least one line": {
		DE: "Eine Rechnung muss mindestens eine Position haben",
		IT: "Una fattura deve avere almeno una riga",
		EN: "An invoice must have at least one line",
	},
	"issue attendue: 'refused' ou 'expired' — une offre est acceptée en la convertissant en facture": {
		DE: "Erwarteter Ausgang: «refused» oder «expired» — eine Offerte wird angenommen, indem man sie in eine Rechnung umwandelt",
		IT: "Esito atteso: «refused» o «expired» — un’offerta si accetta convertendola in fattura",
		EN: "Expected outcome: “refused” or “expired” — a quote is accepted by turning it into an invoice",
	},
	"issue d'offre inconnue": {
		DE: "Unbekannter Offertenausgang",
		IT: "Esito dell’offerta sconosciuto",
		EN: "Unknown quote outcome",
	},
	// L'image est refusée sur ses DIMENSIONS, pas sur son poids : un fichier
	// minuscule peut demander plusieurs gigaoctets à la décompression. Le
	// message nomme donc les pixels, seule grandeur que l'utilisateur puisse
	// corriger.
	"image trop grande": {
		DE: "Bild zu gross",
		IT: "Immagine troppo grande",
		EN: "Image too large",
	},
	"le PDF n'a pas pu être produit": {
		DE: "Das PDF konnte nicht erstellt werden",
		IT: "Il PDF non ha potuto essere prodotto",
		EN: "The PDF could not be produced",
	},
	"« du » doit être au format AAAA-MM-JJ": {
		DE: "«von» muss im Format JJJJ-MM-TT vorliegen",
		IT: "«dal» dev’essere nel formato AAAA-MM-GG",
		EN: "“from” must be in YYYY-MM-DD format",
	},
	"« au » doit être au format AAAA-MM-JJ": {
		DE: "«bis» muss im Format JJJJ-MM-TT vorliegen",
		IT: "«al» dev’essere nel formato AAAA-MM-GG",
		EN: "“to” must be in YYYY-MM-DD format",
	},
	"la date de fin précède la date de début": {
		DE: "Das Enddatum liegt vor dem Anfangsdatum",
		IT: "La data di fine precede la data di inizio",
		EN: "The end date comes before the start date",
	},
	"la date d'émission doit être au format AAAA-MM-JJ": {
		DE: "Das Ausstellungsdatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data di emissione dev’essere nel formato AAAA-MM-GG",
		EN: "The issue date must be in YYYY-MM-DD format",
	},
	"jeton de rafraîchissement introuvable": {
		DE: "Aktualisierungstoken nicht gefunden",
		IT: "Token di aggiornamento non trovato",
		EN: "Refresh token not found",
	},
	"jeton de rafraîchissement invalide ou expiré": {
		DE: "Aktualisierungstoken ungültig oder abgelaufen",
		IT: "Token di aggiornamento non valido o scaduto",
		EN: "Refresh token invalid or expired",
	},
	"journal entry is already posted": {
		DE: "Die Buchung ist bereits verbucht",
		IT: "La registrazione è già contabilizzata",
		EN: "The journal entry is already posted",
	},
	"journal entry not found": {
		DE: "Buchung nicht gefunden",
		IT: "Registrazione non trovata",
		EN: "Journal entry not found",
	},
	"l'IBAN de votre entreprise n'est pas renseigné (Paramètres → Entreprise) : aucun ordre de paiement ne peut etre produit sans compte a debiter": {
		DE: "Die IBAN Ihres Unternehmens ist nicht hinterlegt (Einstellungen → Unternehmen): ohne zu belastendes Konto lässt sich kein Zahlungsauftrag erzeugen",
		IT: "L’IBAN della vostra azienda non è indicato (Impostazioni → Azienda): nessun ordine di pagamento può essere prodotto senza conto da addebitare",
		EN: "Your company’s IBAN is not filled in (Settings → Company): no payment order can be produced without an account to debit",
	},
	"l'IBAN est vide": {
		DE: "Die IBAN ist leer",
		IT: "L’IBAN è vuoto",
		EN: "The IBAN is empty",
	},
	"l'activation d'un contact ne se change pas à la main : pour retirer un contact des listes, anonymisez-le": {
		DE: "Die Aktivierung eines Kontakts wird nicht von Hand geändert: um einen Kontakt aus den Listen zu nehmen, anonymisieren Sie ihn",
		IT: "L’attivazione di un contatto non si cambia a mano: per togliere un contatto dagli elenchi, anonimizzatelo",
		EN: "A contact’s activation is not changed by hand: to take a contact out of the lists, anonymise it",
	},
	"l'adresse de données du logo est invalide": {
		DE: "Die Daten-URL des Logos ist ungültig",
		IT: "L’URL di dati del logo non è valido",
		EN: "The logo data URL is invalid",
	},
	"l'archive ZIP n'a pas pu être finalisée": {
		DE: "Das ZIP-Archiv konnte nicht abgeschlossen werden",
		IT: "L’archivio ZIP non ha potuto essere finalizzato",
		EN: "The ZIP archive could not be finalised",
	},
	"l'identifiant de l'exercice est requis": {
		DE: "Die Kennung des Geschäftsjahres ist erforderlich",
		IT: "L’identificativo dell’esercizio è obbligatorio",
		EN: "The financial year identifier is required",
	},
	"l'installation est déjà initialisée — passez par la création de compte ou le panneau d'administration": {
		DE: "Die Installation ist bereits eingerichtet — nutzen Sie die Kontoerstellung oder das Verwaltungsfenster",
		IT: "L’installazione è già inizializzata — passate dalla creazione di account o dal pannello di amministrazione",
		EN: "The installation is already initialised — use account creation or the administration panel",
	},
	"l'offre ne contient aucune ligne à facturer": {
		DE: "Die Offerte enthält keine zu fakturierende Position",
		IT: "L’offerta non contiene alcuna riga da fatturare",
		EN: "The quote contains no line to invoice",
	},
	"l'ordre de paiement est indisponible": {
		DE: "Der Zahlungsauftrag ist nicht verfügbar",
		IT: "L’ordine di pagamento non è disponibile",
		EN: "The payment order is unavailable",
	},
	"l'échéance doit être au format AAAA-MM-JJ": {
		DE: "Die Fälligkeit muss im Format JJJJ-MM-TT vorliegen",
		IT: "La scadenza dev’essere nel formato AAAA-MM-GG",
		EN: "The due date must be in YYYY-MM-DD format",
	},
	"l'écriture d'origine %s ne porte aucune ligne": {
		DE: "Die ursprüngliche Buchung %s trägt keine Zeile",
		IT: "La registrazione originale %s non porta alcuna riga",
		EN: "The original entry %s carries no line",
	},
	"la clé de contrôle de l'IBAN est fausse — vérifiez la saisie, un chiffre a probablement été inversé": {
		DE: "Die Prüfziffer der IBAN ist falsch — prüfen Sie die Eingabe, wahrscheinlich wurde eine Ziffer vertauscht",
		IT: "La chiave di controllo dell’IBAN è errata — verificate l’inserimento, probabilmente una cifra è stata invertita",
		EN: "The IBAN check digits are wrong — check what you typed, a digit was probably transposed",
	},
	"la clé de la base est illisible sur ce compte : la phrase de récupération est nécessaire": {
		DE: "Der Datenbankschlüssel ist unter diesem Konto nicht lesbar: die Wiederherstellungs-Passphrase ist nötig",
		IT: "La chiave della base non è leggibile su questo account: la passphrase di recupero è necessaria",
		EN: "The database key is unreadable under this account: the recovery passphrase is needed",
	},
	"la clé est illisible sur ce compte : récupérez-la d'abord avec la phrase de récupération": {
		DE: "Der Schlüssel ist unter diesem Konto nicht lesbar: stellen Sie ihn zuerst mit der Wiederherstellungs-Passphrase wieder her",
		IT: "La chiave non è leggibile su questo account: recuperatela prima con la passphrase di recupero",
		EN: "The key is unreadable under this account: recover it first with the recovery passphrase",
	},
	"la date d'exécution doit être au format AAAA-MM-JJ": {
		DE: "Das Ausführungsdatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data di esecuzione dev’essere nel formato AAAA-MM-GG",
		EN: "The execution date must be in YYYY-MM-DD format",
	},
	"la date de fin doit suivre la date de début": {
		DE: "Das Enddatum muss nach dem Startdatum liegen",
		IT: "La data di fine deve seguire la data di inizio",
		EN: "The end date must follow the start date",
	},
	"la date de la facture doit être au format AAAA-MM-JJ": {
		DE: "Das Rechnungsdatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data della fattura dev’essere nel formato AAAA-MM-GG",
		EN: "The invoice date must be in YYYY-MM-DD format",
	},
	"la date doit être au format AAAA-MM-JJ": {
		DE: "Das Datum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data dev’essere nel formato AAAA-MM-GG",
		EN: "The date must be in YYYY-MM-DD format",
	},
	"la facture a été comptabilisée entre-temps : rechargez la page": {
		DE: "Die Rechnung wurde zwischenzeitlich verbucht: laden Sie die Seite neu",
		IT: "La fattura è stata contabilizzata nel frattempo: ricaricate la pagina",
		EN: "The invoice was posted meanwhile: reload the page",
	},
	"la facture ne contient aucune ligne à créditer": {
		DE: "Die Rechnung enthält keine gutzuschreibende Position",
		IT: "La fattura non contiene alcuna riga da accreditare",
		EN: "The invoice contains no line to credit",
	},
	"la fin de période doit être au format AAAA-MM-JJ": {
		DE: "Das Periodenende muss im Format JJJJ-MM-TT vorliegen",
		IT: "La fine del periodo dev’essere nel formato AAAA-MM-GG",
		EN: "The period end must be in YYYY-MM-DD format",
	},
	"la fin de période doit être égale ou postérieure à son début": {
		DE: "Das Periodenende muss gleich oder später als sein Beginn sein",
		IT: "La fine del periodo dev’essere uguale o posteriore al suo inizio",
		EN: "The period end must be equal to or later than its start",
	},
	"la phrase de passe doit compter au moins %d caractères (%d saisis)": {
		DE: "Die Passphrase muss mindestens %d Zeichen zählen (%d eingegeben)",
		IT: "La passphrase deve contare almeno %d caratteri (%d inseriti)",
		EN: "The passphrase must be at least %d characters long (%d entered)",
	},
	"la phrase de passe doit contenir %s": {
		DE: "Die Passphrase muss %s enthalten",
		IT: "La passphrase deve contenere %s",
		EN: "The passphrase must contain %s",
	},
	"la période chevauche l'exercice « ": {
		DE: "Der Zeitraum überschneidet das Geschäftsjahr «",
		IT: "Il periodo si sovrappone all’esercizio «",
		EN: "The period overlaps financial year “",
	},
	"la sérialisation JSON a échoué pour ": {
		DE: "Die JSON-Serialisierung ist fehlgeschlagen für ",
		IT: "La serializzazione JSON è fallita per ",
		EN: "JSON serialisation failed for ",
	},
	"le certificat et la clé doivent être fournis ensemble": {
		DE: "Zertifikat und Schlüssel müssen zusammen angegeben werden",
		IT: "Il certificato e la chiave devono essere forniti insieme",
		EN: "The certificate and the key must be supplied together",
	},
	"le code lu n'est pas un QR-facture suisse (l'en-tête SPC est absent)": {
		DE: "Der gelesene Code ist keine Schweizer QR-Rechnung (der SPC-Kopf fehlt)",
		IT: "Il codice letto non è una fattura QR svizzera (l’intestazione SPC è assente)",
		EN: "The code read is not a Swiss QR-bill (the SPC header is missing)",
	},
	"le compte %s est absent du plan comptable : l'achat ne peut pas être comptabilisé": {
		DE: "Konto %s fehlt im Kontenplan: der Einkauf kann nicht verbucht werden",
		IT: "Il conto %s è assente dal piano dei conti: l’acquisto non può essere contabilizzato",
		EN: "Account %s is missing from the chart of accounts: the purchase cannot be posted",
	},
	"le compte %s est désactivé": {
		DE: "Konto %s ist deaktiviert",
		IT: "Il conto %s è disattivato",
		EN: "Account %s is deactivated",
	},
	"le compte %s n'a pas pu être lu": {
		DE: "Konto %s konnte nicht gelesen werden",
		IT: "Il conto %s non ha potuto essere letto",
		EN: "Account %s could not be read",
	},
	"le compte %s n'existe pas dans le plan comptable (Plan comptable → la liste des numéros disponibles)": {
		DE: "Konto %s existiert nicht im Kontenplan (Kontenplan → die Liste der verfügbaren Nummern)",
		IT: "Il conto %s non esiste nel piano dei conti (Piano dei conti → l’elenco dei numeri disponibili)",
		EN: "Account %s does not exist in the chart of accounts (Chart of accounts → the list of available numbers)",
	},
	"le compte bancaire indiqué est introuvable ou n'est pas un compte d'actif actif": {
		DE: "Das angegebene Bankkonto ist nicht auffindbar oder kein aktives Aktivkonto",
		IT: "Il conto bancario indicato è introvabile o non è un conto attivo dell’attivo",
		EN: "The bank account given cannot be found or is not an active asset account",
	},
	"le compte bancaire par défaut (1020) est absent du plan comptable : indiquez explicitement le compte à débiter": {
		DE: "Das Standard-Bankkonto (1020) fehlt im Kontenplan: geben Sie das zu belastende Konto ausdrücklich an",
		IT: "Il conto bancario predefinito (1020) è assente dal piano dei conti: indicate esplicitamente il conto da addebitare",
		EN: "The default bank account (1020) is missing from the chart of accounts: state the account to debit explicitly",
	},
	"le compte clients (1100) est absent du plan comptable": {
		DE: "Das Debitorenkonto (1100) fehlt im Kontenplan",
		IT: "Il conto clienti (1100) è assente dal piano dei conti",
		EN: "The receivables account (1100) is missing from the chart of accounts",
	},
	"le contrôle d'intégrité n'a pas pu s'exécuter: ": {
		DE: "Die Integritätsprüfung konnte nicht ausgeführt werden: ",
		IT: "Il controllo d’integrità non ha potuto essere eseguito: ",
		EN: "The integrity check could not run: ",
	},
	"le corps de la requête n'a pas pu être lu": {
		DE: "Der Anfragekörper konnte nicht gelesen werden",
		IT: "Il corpo della richiesta non ha potuto essere letto",
		EN: "The request body could not be read",
	},
	"le début de période doit être au format AAAA-MM-JJ": {
		DE: "Der Periodenbeginn muss im Format JJJJ-MM-TT vorliegen",
		IT: "L’inizio del periodo dev’essere nel formato AAAA-MM-GG",
		EN: "The period start must be in YYYY-MM-DD format",
	},
	"le fichier CSV n'a pas pu être produit : ": {
		DE: "Die CSV-Datei konnte nicht erzeugt werden: ",
		IT: "Il file CSV non ha potuto essere prodotto: ",
		EN: "The CSV file could not be produced: ",
	},
	"le fichier d'attestation n'a pas pu être lu": {
		DE: "Die Bescheinigungsdatei konnte nicht gelesen werden",
		IT: "Il file di attestazione non ha potuto essere letto",
		EN: "The attestation file could not be read",
	},
	"le fichier dépasse %d Mo": {
		DE: "Die Datei überschreitet %d MB",
		IT: "Il file supera %d MB",
		EN: "The file exceeds %d MB",
	},
	"le fichier dépasse 10 Mo": {
		DE: "Die Datei überschreitet 10 MB",
		IT: "Il file supera 10 MB",
		EN: "The file exceeds 10 MB",
	},
	"le fichier déposé n'a pas pu être lu": {
		DE: "Die abgelegte Datei konnte nicht gelesen werden",
		IT: "Il file caricato non ha potuto essere letto",
		EN: "The uploaded file could not be read",
	},
	"le fichier n'a pas pu être lu": {
		DE: "Die Datei konnte nicht gelesen werden",
		IT: "Il file non ha potuto essere letto",
		EN: "The file could not be read",
	},
	"le flux de conformité est indisponible": {
		DE: "Der Konformitäts-Feed ist nicht verfügbar",
		IT: "Il flusso di conformità non è disponibile",
		EN: "The compliance feed is unavailable",
	},
	"le jeton d'accès n'a pas pu être produit": {
		DE: "Das Zugangstoken konnte nicht erzeugt werden",
		IT: "Il token di accesso non ha potuto essere prodotto",
		EN: "The access token could not be produced",
	},
	"le jeton de rafraîchissement n'a pas pu être produit": {
		DE: "Das Aktualisierungstoken konnte nicht erzeugt werden",
		IT: "Il token di aggiornamento non ha potuto essere prodotto",
		EN: "The refresh token could not be produced",
	},
	"le jeton n'a pas pu être produit": {
		DE: "Das Token konnte nicht erzeugt werden",
		IT: "Il token non ha potuto essere prodotto",
		EN: "The token could not be produced",
	},
	"le logo doit être au format PNG ou JPEG": {
		DE: "Das Logo muss im Format PNG oder JPEG vorliegen",
		IT: "Il logo dev’essere in formato PNG o JPEG",
		EN: "The logo must be in PNG or JPEG format",
	},
	"le manifeste n'a pas pu être produit": {
		DE: "Das Manifest konnte nicht erzeugt werden",
		IT: "Il manifesto non ha potuto essere prodotto",
		EN: "The manifest could not be produced",
	},
	"le montant doit valoir au moins 0.01, reçu %.2f": {
		DE: "Der Betrag muss mindestens 0.01 betragen, erhalten %.2f",
		IT: "L’importo deve valere almeno 0.01, ricevuto %.2f",
		EN: "The amount must be at least 0.01, received %.2f",
	},
	"le montant ne peut pas dépasser 999999999.99, reçu %.2f": {
		DE: "Der Betrag darf 999999999.99 nicht übersteigen, erhalten %.2f",
		IT: "L’importo non può superare 999999999.99, ricevuto %.2f",
		EN: "The amount cannot exceed 999999999.99, received %.2f",
	},
	"le mot de passe actuel est incorrect": {
		DE: "Das aktuelle Kennwort ist falsch",
		IT: "La password attuale è errata",
		EN: "The current password is wrong",
	},
	"le mot de passe doit contenir au moins 8 caractères": {
		DE: "Das Kennwort muss mindestens 8 Zeichen enthalten",
		IT: "La password deve contenere almeno 8 caratteri",
		EN: "The password must contain at least 8 characters",
	},
	"le mot de passe n'a pas pu être haché": {
		DE: "Das Kennwort konnte nicht gehasht werden",
		IT: "La password non ha potuto essere sottoposta ad hash",
		EN: "The password could not be hashed",
	},
	"le nouveau mot de passe doit être différent de l'actuel : c'est justement celui que quelqu'un d'autre connaît": {
		DE: "Das neue Kennwort muss sich vom aktuellen unterscheiden: genau dieses kennt jemand anderes",
		IT: "La nuova password dev’essere diversa da quella attuale: è proprio quella che qualcun altro conosce",
		EN: "The new password must differ from the current one: that is precisely the one somebody else knows",
	},
	"le paiement n'a pas pu être enregistré": {
		DE: "Die Zahlung konnte nicht erfasst werden",
		IT: "Il pagamento non ha potuto essere registrato",
		EN: "The payment could not be recorded",
	},
	"le paramètre account_code est requis": {
		DE: "Der Parameter account_code ist erforderlich",
		IT: "Il parametro account_code è obbligatorio",
		EN: "The account_code parameter is required",
	},
	"le payload QR n'a pas pu être produit: ": {
		DE: "Die QR-Nutzdaten konnten nicht erzeugt werden: ",
		IT: "Il payload QR non ha potuto essere prodotto: ",
		EN: "The QR payload could not be produced: ",
	},
	"le second facteur est déjà actif sur ce compte": {
		DE: "Der zweite Faktor ist auf diesem Konto bereits aktiv",
		IT: "Il secondo fattore è già attivo su questo account",
		EN: "The second factor is already active on this account",
	},
	"le total des notes de crédit dépasserait le montant de la facture": {
		DE: "Die Summe der Gutschriften würde den Rechnungsbetrag übersteigen",
		IT: "Il totale delle note di credito supererebbe l’importo della fattura",
		EN: "The total of the credit notes would exceed the invoice amount",
	},
	"le type d'un document ne peut pas être changé après sa création": {
		DE: "Die Art eines Belegs lässt sich nach seiner Erstellung nicht mehr ändern",
		IT: "Il tipo di un documento non può essere cambiato dopo la sua creazione",
		EN: "A document’s type cannot be changed after it is created",
	},
	"lecture des factures fournisseurs: ": {
		DE: "Lesen der Lieferantenrechnungen: ",
		IT: "Lettura delle fatture fornitori: ",
		EN: "Reading the supplier invoices: ",
	},
	"les caractères 3 et 4 doivent être la clé de contrôle (deux chiffres), pas %q": {
		DE: "Die Zeichen 3 und 4 müssen die Prüfziffer sein (zwei Ziffern), nicht %q",
		IT: "I caratteri 3 e 4 devono essere la chiave di controllo (due cifre), non %q",
		EN: "Characters 3 and 4 must be the check digits (two digits), not %q",
	},
	"les comptes n'ont pas pu être lus : ": {
		DE: "Die Konten konnten nicht gelesen werden: ",
		IT: "I conti non hanno potuto essere letti: ",
		EN: "The accounts could not be read: ",
	},
	"les contacts n'ont pas pu être lus : ": {
		DE: "Die Kontakte konnten nicht gelesen werden: ",
		IT: "I contatti non hanno potuto essere letti: ",
		EN: "The contacts could not be read: ",
	},
	"les dates doivent être au format AAAA-MM-JJ": {
		DE: "Die Daten müssen im Format JJJJ-MM-TT vorliegen",
		IT: "Le date devono essere nel formato AAAA-MM-GG",
		EN: "The dates must be in YYYY-MM-DD format",
	},
	"les deux premiers caractères doivent être le code du pays (deux lettres), pas %q": {
		DE: "Die ersten beiden Zeichen müssen der Ländercode sein (zwei Buchstaben), nicht %q",
		IT: "I primi due caratteri devono essere il codice del paese (due lettere), non %q",
		EN: "The first two characters must be the country code (two letters), not %q",
	},
	"les exercices n'ont pas pu être lus : ": {
		DE: "Die Geschäftsjahre konnten nicht gelesen werden: ",
		IT: "Gli esercizi non hanno potuto essere letti: ",
		EN: "The financial years could not be read: ",
	},
	"les factures n'ont pas pu être lues : ": {
		DE: "Die Rechnungen konnten nicht gelesen werden: ",
		IT: "Le fatture non hanno potuto essere lette: ",
		EN: "The invoices could not be read: ",
	},
	"les paramètres « from » et « to » sont requis (AAAA-MM-JJ)": {
		DE: "Die Parameter «from» und «to» sind erforderlich (JJJJ-MM-TT)",
		IT: "I parametri «from» e «to» sono obbligatori (AAAA-MM-GG)",
		EN: "The “from” and “to” parameters are required (YYYY-MM-DD)",
	},
	"les types exacts, là où le CSV est du texte.\n\n": {
		DE: "die genauen Typen, wo die CSV nur Text ist.\n\n",
		IT: "i tipi esatti, mentre il CSV è testo.\n\n",
		EN: "the exact types, where the CSV is text.\n\n",
	},
	"les écritures n'ont pas pu être lues : ": {
		DE: "Die Buchungen konnten nicht gelesen werden: ",
		IT: "Le registrazioni non hanno potuto essere lette: ",
		EN: "The entries could not be read: ",
	},
	"limit doit être compris entre 1 et 1000": {
		DE: "limit muss zwischen 1 und 1000 liegen",
		IT: "limit dev’essere compreso tra 1 e 1000",
		EN: "limit must be between 1 and 1000",
	},
	"logo trop volumineux (2 Mo au maximum)": {
		DE: "Logo zu gross (höchstens 2 MB)",
		IT: "Logo troppo grande (2 MB al massimo)",
		EN: "Logo too large (2 MB maximum)",
	},
	"logo_data est requis (adresse de données base64)": {
		DE: "logo_data ist erforderlich (base64-Daten-URL)",
		IT: "logo_data è obbligatorio (URL di dati base64)",
		EN: "logo_data is required (base64 data URL)",
	},
	"maillon d'audit introuvable": {
		DE: "Prüfglied nicht gefunden",
		IT: "Anello di controllo non trovato",
		EN: "Audit link not found",
	},
	"masqués dans cet export, comme dans le JSON. L'export complet d'un IBAN\n": {
		DE: "in diesem Export maskiert, wie im JSON. Der vollständige Export einer IBAN\n",
		IT: "mascherati in questa esportazione, come nel JSON. L’esportazione completa di un IBAN\n",
		EN: "masked in this export, as in the JSON. The full export of an IBAN\n",
	},
	"missing or malformed Authorization header": {
		DE: "Authorization-Kopfzeile fehlt oder ist fehlerhaft",
		IT: "Intestazione Authorization assente o malformata",
		EN: "Missing or malformed Authorization header",
	},
	"mot de passe incorrect": {
		DE: "Falsches Kennwort",
		IT: "Password errata",
		EN: "Wrong password",
	},
	"mot de passe trop faible — ": {
		DE: "Kennwort zu schwach — ",
		IT: "Password troppo debole — ",
		EN: "Password too weak — ",
	},
	"numéro IDE introuvable au registre du commerce": {
		DE: "UID-Nummer im Handelsregister nicht gefunden",
		IT: "Numero IDI non trovato nel registro di commercio",
		EN: "UID number not found in the commercial register",
	},
	"paiement introuvable": {
		DE: "Zahlung nicht gefunden",
		IT: "Pagamento non trovato",
		EN: "Payment not found",
	},
	"pain.001: at least one transaction required": {
		DE: "pain.001: mindestens eine Transaktion erforderlich",
		IT: "pain.001: almeno una transazione è obbligatoria",
		EN: "pain.001: at least one transaction is required",
	},
	"pain.001: debtor IBAN required": {
		DE: "pain.001: IBAN des Zahlungspflichtigen erforderlich",
		IT: "pain.001: IBAN del debitore obbligatorio",
		EN: "pain.001: debtor IBAN required",
	},
	"paramètre 'che' requis": {
		DE: "Parameter «che» erforderlich",
		IT: "Parametro «che» obbligatorio",
		EN: "Parameter “che” required",
	},
	"passphrase incorrecte, ou fichier de sauvegarde altéré": {
		DE: "Falsche Passphrase oder beschädigte Sicherungsdatei",
		IT: "Passphrase errata, o file di backup alterato",
		EN: "Wrong passphrase, or corrupted backup file",
	},
	"passphrase vide": {
		DE: "Leere Passphrase",
		IT: "Passphrase vuota",
		EN: "Empty passphrase",
	},
	"la date de paiement doit être au format AAAA-MM-JJ": {
		DE: "Das Zahlungsdatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data di pagamento dev’essere nel formato AAAA-MM-GG",
		EN: "The payment date must be in YYYY-MM-DD format",
	},
	"pdf generation failed": {
		DE: "PDF-Erzeugung fehlgeschlagen",
		IT: "Generazione del PDF fallita",
		EN: "PDF generation failed",
	},
	"périodicité hors bornes: %d jours (0 pour désactiver, 365 au plus)": {
		DE: "Turnus ausserhalb der Grenzen: %d Tage (0 zum Deaktivieren, höchstens 365)",
		IT: "Periodicità fuori limiti: %d giorni (0 per disattivare, 365 al massimo)",
		EN: "Interval out of bounds: %d days (0 to disable, 365 at most)",
	},
	"rapprochement annulé": {
		DE: "Abstimmung rückgängig gemacht",
		IT: "riconciliazione annullata",
		EN: "reconciliation undone",
	},
	"reference must be empty for reference type NON, got %q": {
		DE: "Die Referenz muss beim Typ NON leer sein, erhalten %q",
		IT: "Il riferimento dev’essere vuoto per il tipo NON, ricevuto %q",
		EN: "The reference must be empty for type NON, got %q",
	},
	"reference type NON cannot be used with QR-IBAN %s; a QR-IBAN requires the QRR reference type": {
		DE: "Der Referenztyp NON lässt sich nicht mit der QR-IBAN %s verwenden; eine QR-IBAN verlangt den Typ QRR",
		IT: "Il tipo di riferimento NON non può essere usato con il QR-IBAN %s; un QR-IBAN richiede il tipo QRR",
		EN: "Reference type NON cannot be used with QR-IBAN %s; a QR-IBAN requires the QRR type",
	},
	"reference type QRR may only be used for invoices in CHF, got %s": {
		DE: "Der Referenztyp QRR darf nur für Rechnungen in CHF verwendet werden, erhalten %s",
		IT: "Il tipo di riferimento QRR può essere usato solo per fatture in CHF, ricevuto %s",
		EN: "Reference type QRR may only be used for invoices in CHF, got %s",
	},
	"reference type QRR requires a QR-IBAN (QR-IID 30000–31999); %s is a regular IBAN — use SCOR or NON": {
		DE: "Der Referenztyp QRR verlangt eine QR-IBAN (QR-IID 30000–31999); %s ist eine gewöhnliche IBAN — verwenden Sie SCOR oder NON",
		IT: "Il tipo di riferimento QRR richiede un QR-IBAN (QR-IID 30000–31999); %s è un IBAN ordinario — usate SCOR o NON",
		EN: "Reference type QRR requires a QR-IBAN (QR-IID 30000–31999); %s is a regular IBAN — use SCOR or NON",
	},
	"reference type SCOR cannot be used with QR-IBAN %s; a QR-IBAN requires the QRR reference type": {
		DE: "Der Referenztyp SCOR lässt sich nicht mit der QR-IBAN %s verwenden; eine QR-IBAN verlangt den Typ QRR",
		IT: "Il tipo di riferimento SCOR non può essere usato con il QR-IBAN %s; un QR-IBAN richiede il tipo QRR",
		EN: "Reference type SCOR cannot be used with QR-IBAN %s; a QR-IBAN requires the QRR type",
	},
	"reference type must be QRR, SCOR, or NON; got %q": {
		DE: "Der Referenztyp muss QRR, SCOR oder NON sein; erhalten %q",
		IT: "Il tipo di riferimento dev’essere QRR, SCOR o NON; ricevuto %q",
		EN: "The reference type must be QRR, SCOR or NON; got %q",
	},
	"registre IDE momentanément indisponible": {
		DE: "UID-Register vorübergehend nicht verfügbar",
		IT: "Registro IDI momentaneamente non disponibile",
		EN: "UID register temporarily unavailable",
	},
	"registre IDE momentanément indisponible — saisissez les informations manuellement": {
		DE: "UID-Register vorübergehend nicht verfügbar — erfassen Sie die Angaben von Hand",
		IT: "Registro IDI momentaneamente non disponibile — inserite le informazioni manualmente",
		EN: "UID register temporarily unavailable — enter the details by hand",
	},
	"restauration annulée": {
		DE: "Wiederherstellung abgebrochen",
		IT: "ripristino annullato",
		EN: "restore cancelled",
	},
	"retirez votre propre second facteur depuis votre compte : le mot de passe y est redemandé, ce qui est justement la protection à laquelle vous renoncez": {
		DE: "Entfernen Sie Ihren eigenen zweiten Faktor über Ihr Konto: dort wird das Kennwort erneut verlangt, was genau der Schutz ist, auf den Sie verzichten",
		IT: "Rimuovete il vostro secondo fattore dal vostro account: lì la password è richiesta di nuovo, che è proprio la protezione a cui rinunciate",
		EN: "Remove your own second factor from your account: the password is asked again there, which is precisely the protection you are giving up",
	},
	"rien à appliquer : le redémarrage n'est proposé que pour une restauration préparée ou un changement de réglages réseau": {
		DE: "Nichts anzuwenden: der Neustart wird nur für eine vorbereitete Wiederherstellung oder eine Änderung der Netzwerkeinstellungen angeboten",
		IT: "Nulla da applicare: il riavvio è proposto solo per un ripristino preparato o una modifica delle impostazioni di rete",
		EN: "Nothing to apply: the restart is offered only for a prepared restore or a change of network settings",
	},
	"réinitialisez plutôt votre propre mot de passe depuis votre compte : cette action afficherait un mot de passe temporaire à l'écran sans rien vous apporter": {
		DE: "Setzen Sie Ihr eigenes Kennwort lieber über Ihr Konto zurück: diese Aktion zeigte ein temporäres Kennwort auf dem Bildschirm, ohne Ihnen etwas zu bringen",
		IT: "Reimpostate piuttosto la vostra password dal vostro account: questa azione mostrerebbe una password temporanea a schermo senza portarvi nulla",
		EN: "Reset your own password from your account instead: this action would show a temporary password on screen without helping you",
	},
	"rôle inconnu": {
		DE: "Unbekannte Rolle",
		IT: "Ruolo sconosciuto",
		EN: "Unknown role",
	},
	"sauvegarde introuvable": {
		DE: "Sicherung nicht gefunden",
		IT: "Backup non trovato",
		EN: "Backup not found",
	},
	"se fait depuis la fiche du contact concerné.\n": {
		DE: "erfolgt über das Dossier des betreffenden Kontakts.\n",
		IT: "si effettua dalla scheda del contatto interessato.\n",
		EN: "is done from the relevant contact’s record.\n",
	},
	"secret absent": {
		DE: "Geheimnis fehlt",
		IT: "Segreto assente",
		EN: "Secret missing",
	},
	"secret vide": {
		DE: "Leeres Geheimnis",
		IT: "Segreto vuoto",
		EN: "Empty secret",
	},
	"service comptable indisponible": {
		DE: "Buchhaltungsdienst nicht verfügbar",
		IT: "Servizio contabile non disponibile",
		EN: "Accounting service unavailable",
	},
	"session invalide": {
		DE: "Ungültige Sitzung",
		IT: "Sessione non valida",
		EN: "Invalid session",
	},
	"seule une facture fournisseur au brouillon peut être supprimée. Annulez-la plutôt : la pièce doit être conservée (CO art. 958f)": {
		DE: "Nur eine Lieferantenrechnung im Entwurf lässt sich löschen. Stornieren Sie sie stattdessen: der Beleg ist aufzubewahren (OR Art. 958f)",
		IT: "Solo una fattura fornitore in bozza può essere eliminata. Annullatela piuttosto: il documento dev’essere conservato (CO art. 958f)",
		EN: "Only a supplier invoice in draft can be deleted. Cancel it instead: the record must be kept (CO art. 958f)",
	},
	"seule une facture peut faire l'objet d'une note de crédit": {
		DE: "Nur zu einer Rechnung kann eine Gutschrift ausgestellt werden",
		IT: "Solo una fattura può essere oggetto di una nota di credito",
		EN: "Only an invoice can be the subject of a credit note",
	},
	"seuls les offres de prix peuvent être converties en facture": {
		DE: "Nur Offerten lassen sich in eine Rechnung umwandeln",
		IT: "Solo le offerte possono essere convertite in fattura",
		EN: "Only quotes can be turned into an invoice",
	},
	"la date de début doit être au format AAAA-MM-JJ": {
		DE: "Das Startdatum muss im Format JJJJ-MM-TT vorliegen",
		IT: "La data di inizio dev’essere nel formato AAAA-MM-GG",
		EN: "The start date must be in YYYY-MM-DD format",
	},
	"statut TVA inconnu : attendu « assujetti » ou « non assujetti »": {
		DE: "Unbekannter MWST-Status: erwartet «steuerpflichtig» oder «nicht steuerpflichtig»",
		IT: "Stato IVA sconosciuto: atteso «assoggettato» o «non assoggettato»",
		EN: "Unknown VAT status: expected “liable” or “not liable”",
	},
	"statut inconnu ": {
		DE: "Unbekannter Status ",
		IT: "Stato sconosciuto ",
		EN: "Unknown status ",
	},
	"sur PostgreSQL, le chiffrement au repos se règle côté serveur de base de données": {
		DE: "Unter PostgreSQL wird die Verschlüsselung im Ruhezustand auf dem Datenbankserver eingestellt",
		IT: "Su PostgreSQL, la cifratura a riposo si configura sul server di base di dati",
		EN: "On PostgreSQL, encryption at rest is configured on the database server",
	},
	"too many failed attempts, try again later": {
		DE: "Zu viele Fehlversuche, versuchen Sie es später erneut",
		IT: "Troppi tentativi falliti, riprovate più tardi",
		EN: "Too many failed attempts, try again later",
	},
	"trace refusée: aucun auteur pour %s sur %s/%s": {
		DE: "Spur abgelehnt: kein Urheber für %s auf %s/%s",
		IT: "Traccia rifiutata: nessun autore per %s su %s/%s",
		EN: "Trace refused: no author for %s on %s/%s",
	},
	"un IBAN %s compte %d caractères, celui-ci en a %d": {
		DE: "Eine IBAN %s zählt %d Zeichen, diese hat %d",
		IT: "Un IBAN %s conta %d caratteri, questo ne ha %d",
		EN: "A %s IBAN has %d characters, this one has %d",
	},
	"un IBAN compte entre 15 et 34 caractères, celui-ci en a %d": {
		DE: "Eine IBAN zählt zwischen 15 und 34 Zeichen, diese hat %d",
		IT: "Un IBAN conta tra 15 e 34 caratteri, questo ne ha %d",
		EN: "An IBAN has between 15 and 34 characters, this one has %d",
	},
	"un compte administrateur doit être protégé par un second facteur : inscrivez votre application d'authentification pour continuer": {
		DE: "Ein Administratorkonto muss durch einen zweiten Faktor geschützt sein: richten Sie Ihre Authentifizierungs-App ein, um fortzufahren",
		IT: "Un account amministratore dev’essere protetto da un secondo fattore: attivate la vostra app di autenticazione per continuare",
		EN: "An administrator account must be protected by a second factor: set up your authenticator app to continue",
	},
	"un exercice porte déjà ce nom": {
		DE: "Ein Geschäftsjahr trägt diesen Namen bereits",
		IT: "Un esercizio porta già questo nome",
		EN: "A financial year already bears this name",
	},
	"une facture au statut brouillon ou annulé ne peut pas être créditée": {
		DE: "Zu einer Rechnung im Entwurf oder storniert lässt sich keine Gutschrift ausstellen",
		IT: "Una fattura in stato bozza o annullata non può essere accreditata",
		EN: "An invoice in draft or cancelled status cannot be credited",
	},
	"une facture fournisseur annulée ne change plus de statut": {
		DE: "Eine stornierte Lieferantenrechnung wechselt den Status nicht mehr",
		IT: "Una fattura fornitore annullata non cambia più stato",
		EN: "A cancelled supplier invoice no longer changes status",
	},
	"une offre au statut brouillon ou annulé ne peut pas être convertie": {
		DE: "Eine Offerte im Entwurf oder storniert lässt sich nicht umwandeln",
		IT: "Un’offerta in stato bozza o annullata non può essere convertita",
		EN: "A quote in draft or cancelled status cannot be converted",
	},
	"une écriture comporte au moins deux lignes : ce qui est débité et ce qui est crédité (CO art. 957)": {
		DE: "Eine Buchung umfasst mindestens zwei Zeilen: was belastet und was gutgeschrieben wird (OR Art. 957)",
		IT: "Una registrazione comprende almeno due righe: ciò che è addebitato e ciò che è accreditato (CO art. 957)",
		EN: "An entry has at least two lines: what is debited and what is credited (CO art. 957)",
	},
	"une écriture comptable porte au moins deux lignes": {
		DE: "Eine Buchung trägt mindestens zwei Zeilen",
		IT: "Una registrazione contabile porta almeno due righe",
		EN: "An accounting entry carries at least two lines",
	},
	"unknown VAT method %q: must be 'effective' or 'tdfn'": {
		DE: "Unbekannte MWST-Methode %q: muss «effective» oder «tdfn» sein",
		IT: "Metodo IVA sconosciuto %q: dev’essere «effective» o «tdfn»",
		EN: "Unknown VAT method %q: must be “effective” or “tdfn”",
	},
	"votre rôle (": {
		DE: "Ihre Rolle (",
		IT: "Il vostro ruolo (",
		EN: "Your role (",
	},
	"vous avez déclaré ne pas être assujetti à la TVA : la LTVA art. 27 al. 1 vous interdit de la faire figurer sur une facture, et l'al. 2 vous en rendrait redevable même sans l'avoir encaissée. Passez les lignes à 0 %, ou corrigez votre statut dans Paramètres → Banque": {
		DE: "Sie haben erklärt, nicht mehrwertsteuerpflichtig zu sein: MWSTG Art. 27 Abs. 1 untersagt Ihnen, die Steuer auf einer Rechnung auszuweisen, und Abs. 2 würde Sie dafür steuerpflichtig machen, auch ohne sie vereinnahmt zu haben. Setzen Sie die Zeilen auf 0 %, oder berichtigen Sie Ihren Status unter Einstellungen → Bank",
		IT: "Avete dichiarato di non essere assoggettati all’IVA: la LIVA art. 27 cpv. 1 vi vieta di indicarla su una fattura, e il cpv. 2 ve ne renderebbe debitori anche senza averla incassata. Portate le righe a 0 %, oppure correggete il vostro stato in Impostazioni → Banca",
		EN: "You have declared that you are not liable for VAT: VAT Act art. 27 para. 1 forbids you to show it on an invoice, and para. 2 would make you liable for it even if you never collected it. Set the lines to 0 %, or correct your status under Settings → Bank",
	},
	"vous ne pouvez pas changer votre propre rôle : une rétrogradation par mégarde vous couperait l'accès. Demandez à un autre administrateur.": {
		DE: "Sie können Ihre eigene Rolle nicht ändern: eine versehentliche Herabstufung würde Ihnen den Zugang abschneiden. Fragen Sie einen anderen Administrator.",
		IT: "Non potete cambiare il vostro ruolo: un declassamento per errore vi taglierebbe l’accesso. Chiedete a un altro amministratore.",
		EN: "You cannot change your own role: an accidental demotion would cut off your access. Ask another administrator.",
	},
	"vous ne pouvez pas désactiver votre propre compte": {
		DE: "Sie können Ihr eigenes Konto nicht deaktivieren",
		IT: "Non potete disattivare il vostro account",
		EN: "You cannot deactivate your own account",
	},
	"« from » doit être au format AAAA-MM-JJ": {
		DE: "«from» muss im Format JJJJ-MM-TT vorliegen",
		IT: "«from» dev’essere nel formato AAAA-MM-GG",
		EN: "“from” must be in YYYY-MM-DD format",
	},
	"« to » doit être au format AAAA-MM-JJ": {
		DE: "«to» muss im Format JJJJ-MM-TT vorliegen",
		IT: "«to» dev’essere nel formato AAAA-MM-GG",
		EN: "“to” must be in YYYY-MM-DD format",
	},
	"Échéance:": {
		DE: "Fällig am:",
		IT: "Scadenza:",
		EN: "Due:",
	},
	"Écriture rapprochée. L'encaissement reste à enregistrer depuis la facture : rapprocher identifie le versement, il ne solde pas la créance.": {
		DE: "Buchung abgestimmt. Der Zahlungseingang ist noch von der Rechnung aus zu erfassen: Abstimmen erkennt die Zahlung, es gleicht die Forderung nicht aus.",
		IT: "Registrazione riconciliata. L’incasso resta da registrare dalla fattura: riconciliare identifica il versamento, non salda il credito.",
		EN: "Entry reconciled. The payment still has to be recorded from the invoice: reconciling identifies the transfer, it does not settle the receivable.",
	},
	"écriture bancaire introuvable": {
		DE: "Bankbuchung nicht gefunden",
		IT: "registrazione bancaria non trovata",
		EN: "bank entry not found",
	},
	"écriture dans l'archive impossible": {
		DE: "Schreiben ins Archiv nicht möglich",
		IT: "Scrittura nell’archivio impossibile",
		EN: "Cannot write into the archive",
	},
	"écriture dans l'archive impossible : ": {
		DE: "Schreiben ins Archiv nicht möglich: ",
		IT: "Scrittura nell’archivio impossibile: ",
		EN: "Cannot write into the archive: ",
	},
	"écriture du CSV: ": {
		DE: "Schreiben der CSV: ",
		IT: "Scrittura del CSV: ",
		EN: "Writing the CSV: ",
	},
	"écriture du manifeste dans l'archive impossible": {
		DE: "Schreiben des Manifests ins Archiv nicht möglich",
		IT: "Scrittura del manifesto nell’archivio impossibile",
		EN: "Cannot write the manifest into the archive",
	},
	"écriture introuvable": {
		DE: "Buchung nicht gefunden",
		IT: "registrazione non trovata",
		EN: "entry not found",
	},
	"écriture mise à jour": {
		DE: "Buchung aktualisiert",
		IT: "registrazione aggiornata",
		EN: "entry updated",
	},
}
