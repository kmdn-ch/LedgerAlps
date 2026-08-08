// La langue de l'interface.
//
// # Pourquoi pas une bibliothèque
//
// react-i18next fait bien plus que ce qu'il faut ici : détection de langue par
// le navigateur, chargement asynchrone des catalogues, espaces de noms,
// contextes, interpolation avec composants React. LedgerAlps a quatre langues
// connues à la compilation, un catalogue embarqué dans le binaire, et des
// phrases simples. Le coût d'une dépendance se paie en mises à jour, en
// surface d'attaque et en poids — pour un besoin que cent lignes couvrent.
//
// Ce choix a un revers assumé : pas de pluriels complexes ni de formats ICU.
// Les quatre langues visées mettent toutes le pluriel à « un / autre », ce que
// `plural()` couvre. Si une cinquième langue arrivait avec des règles slaves,
// il faudrait revoir cela — et ce serait le bon moment pour prendre la
// bibliothèque.
//
// # Le catalogue est TYPÉ par le français
//
// `Catalogue` dérive du fichier français. Une clé oubliée dans l'allemand est
// une erreur de compilation, pas une chaîne manquante découverte par un
// utilisateur. C'est ce qui rend la promesse « traduction complète » tenable :
// elle est vérifiée par TypeScript, pas par la relecture.

import { fr } from './fr'

/** Les langues proposées. L'ordre est celui du sélecteur. */
export const LANGUES = [
  { code: 'fr', nom: 'Français',  drapeau: 'FR' },
  { code: 'de', nom: 'Deutsch',   drapeau: 'DE' },
  { code: 'it', nom: 'Italiano',  drapeau: 'IT' },
  { code: 'en', nom: 'English',   drapeau: 'EN' },
] as const

export type Langue = (typeof LANGUES)[number]['code']

/**
 * La forme du catalogue, dérivée du français.
 *
 * Le français est la source : c'est la langue dans laquelle le produit a été
 * écrit et pensé, et celle dont la terminologie a été validée en premier
 * (voir docs/GLOSSAIRE.md).
 */
export type Catalogue = Record<keyof typeof fr, string>

/** Une clé de traduction — vérifiée à la compilation. */
export type Cle = keyof Catalogue

// `Record<keyof typeof fr, string>` et non `typeof fr` : `as const` fige chaque
// valeur française en type littéral, si bien qu'un catalogue allemand ne
// pourrait contenir que… le français. On garde les CLÉS strictes — une clé
// oubliée reste une erreur de compilation — et on libère les valeurs.

const LANGUE_PAR_DEFAUT: Langue = 'fr'

/**
 * langueValide ramène n'importe quelle valeur à une langue connue.
 *
 * Une préférence enregistrée par une version future, un réglage bricolé à la
 * main, une valeur nulle : tout retombe sur le français. Afficher des clés
 * brutes serait pire que d'afficher la mauvaise langue.
 */
export function langueValide(v: unknown): Langue {
  return LANGUES.some(l => l.code === v) ? (v as Langue) : LANGUE_PAR_DEFAUT
}

/**
 * interpole remplace les repères {nom} par leurs valeurs.
 *
 * Les repères sont NOMMÉS et non positionnels : l'ordre des mots change d'une
 * langue à l'autre, et « {compte} est absent » devient « Das Konto {compte}
 * fehlt ». Des repères positionnels obligeraient le traducteur à respecter un
 * ordre que sa langue ne veut pas.
 */
export function interpole(modele: string, valeurs?: Record<string, string | number>): string {
  if (!valeurs) return modele
  return modele.replace(/\{(\w+)\}/g, (brut, cle) =>
    cle in valeurs ? String(valeurs[cle]) : brut)
}

/**
 * plural choisit entre singulier et pluriel.
 *
 * Les quatre langues visées se comportent de la même façon : une seule forme
 * de pluriel, et le zéro se dit au pluriel — sauf en français, où « 0 facture »
 * reste au singulier. C'est la seule irrégularité, et elle est ici.
 */
export function plural(n: number, langue: Langue, un: string, plusieurs: string): string {
  if (langue === 'fr') return Math.abs(n) < 2 ? un : plusieurs
  return Math.abs(n) === 1 ? un : plusieurs
}

// ─── Formats ─────────────────────────────────────────────────────────────────

/**
 * localeIntl rend l'étiquette de locale à passer à Intl.
 *
 * Le suffixe compte : `de-CH` sépare les milliers par une apostrophe, `de-DE`
 * par un point — et « 1.234 » se lirait « mille deux cent trente-quatre » ou
 * « un virgule deux » selon le pays. L'anglais vise le Royaume-Uni, donc
 * `en-GB` : dates en jj/mm/aaaa et virgule des milliers.
 */
export function localeIntl(langue: Langue): string {
  switch (langue) {
    case 'de': return 'de-CH'
    case 'it': return 'it-CH'
    case 'en': return 'en-GB'
    default:   return 'fr-CH'
  }
}

/**
 * abrevLoi traduit l'abréviation d'un texte de loi.
 *
 * Le Code des obligations s'appelle OR en allemand : « CO art. 958f » y devient
 * « OR Art. 958f ». Les NUMÉROS d'article ne changent pas — c'est le même
 * texte, publié en trois langues. Voir docs/GLOSSAIRE.md.
 */
export function abrevLoi(abrev: string, langue: Langue): string {
  const table: Record<string, Record<Langue, string>> = {
    CO:    { fr: 'CO',    de: 'OR',    it: 'CO',    en: 'CO' },
    LTVA:  { fr: 'LTVA',  de: 'MWSTG', it: 'LIVA',  en: 'MWSTG' },
    nLPD:  { fr: 'nLPD',  de: 'DSG',   it: 'LPD',   en: 'FADP' },
    LPD:   { fr: 'LPD',   de: 'DSG',   it: 'LPD',   en: 'FADP' },
    Olico: { fr: 'Olico', de: 'GeBüV', it: 'Olic',  en: 'GeBüV' },
    OPDo:  { fr: 'OPDo',  de: 'DSV',   it: 'OPDa',  en: 'DSV' },
  }
  return table[abrev]?.[langue] ?? abrev
}
