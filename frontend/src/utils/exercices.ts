// Les exercices proposables pour le carnet du lait.
//
// # Pourquoi le carnet ne partage pas la période des autres exports
//
// La plage de l'écran Rapports est commune au journal, au grand livre et à la
// balance : on l'ajuste pour regarder ce qu'on veut. Le carnet, lui, est une
// pièce que l'on tend à l'administration, et ses deux seuils — CO art. 957
// al. 2 ch. 1 et LTVA art. 10 al. 2 let. a — portent en droit sur le chiffre
// d'affaires du DERNIER EXERCICE. Sur une plage partielle, le document ne peut
// donc rien conclure, et la valeur par défaut de l'écran (du 31 décembre au
// jour courant) n'était jamais un exercice.
//
// # D'où vient la liste
//
// Des exercices DÉCLARÉS quand il y en a : c'est la donnée de l'utilisateur,
// et elle porte les décalages, les exercices raccourcis et les clôtures que
// nul calcul ne devine. À défaut, on les dérive du mois de début d'exercice de
// la fiche entreprise — une déduction, annoncée comme telle par l'appelant.

/** Un exercice proposable, tel que l'écran l'affiche. */
export interface Exercice {
  /** Identifiant d'option ; `libre` désigne la période personnalisée. */
  cle: string
  nom: string
  du: string
  au: string
  /** Vrai quand l'exercice vient des exercices déclarés, non d'un calcul. */
  declare: boolean
  /** Vrai quand la date de fin est passée : l'exercice est révolu. */
  revolu: boolean
}

/** Un exercice tel que l'API le rend. */
export interface ExerciceDeclare {
  id: string
  name: string
  start_date: string
  end_date: string
  is_closed: boolean
}

/** CLE_LIBRE désigne l'option « période personnalisée ». */
export const CLE_LIBRE = 'libre'

/** jour rend une date ISO `AAAA-MM-JJ` sans passer par le fuseau local. */
function jour(annee: number, mois: number, jourDuMois: number): string {
  const d = new Date(Date.UTC(annee, mois - 1, jourDuMois))
  return d.toISOString().slice(0, 10)
}

/** aujourdHui rend la date du jour en ISO, en heure locale. */
export function aujourdHui(maintenant: Date = new Date()): string {
  return jour(maintenant.getFullYear(), maintenant.getMonth() + 1, maintenant.getDate())
}

/**
 * derives calcule les exercices à partir du mois de début.
 *
 * Rend l'exercice en cours et les deux précédents — assez pour couvrir la
 * déclaration de l'année écoulée et un rattrapage, sans transformer un menu en
 * archive. Le nom porte les deux millésimes quand l'exercice est à cheval :
 * un « Exercice 2025 » qui court jusqu'en juin 2026 induirait en erreur.
 */
export function derives(moisDebut: number, maintenant: Date = new Date()): Exercice[] {
  const m = Number.isInteger(moisDebut) && moisDebut >= 1 && moisDebut <= 12 ? moisDebut : 1
  const auj = aujourdHui(maintenant)

  // Le millésime de l'exercice EN COURS : si l'on n'a pas encore atteint le
  // mois de début, l'exercice courant a commencé l'année précédente.
  const annee = maintenant.getFullYear()
  const debutCourant = maintenant.getMonth() + 1 >= m ? annee : annee - 1

  const out: Exercice[] = []
  for (let i = 0; i < 3; i++) {
    const a = debutCourant - i
    const du = jour(a, m, 1)
    const au = finDExercice(a, m)
    out.push({
      cle: `derive-${du}`,
      nom: m === 1 ? `${a}` : `${a}/${a + 1}`,
      du,
      au,
      declare: false,
      revolu: au < auj,
    })
  }
  return out
}

/** finDExercice rend la veille du premier jour de l'exercice suivant. */
function finDExercice(anneeDebut: number, moisDebut: number): string {
  const d = new Date(Date.UTC(anneeDebut + 1, moisDebut - 1, 1))
  d.setUTCDate(d.getUTCDate() - 1)
  return d.toISOString().slice(0, 10)
}

/** depuisDeclares convertit les exercices de l'API en options d'écran. */
export function depuisDeclares(
  liste: ExerciceDeclare[], maintenant: Date = new Date(),
): Exercice[] {
  const auj = aujourdHui(maintenant)
  return liste
    .map(e => ({
      cle: `declare-${e.id}`,
      nom: e.name,
      du: e.start_date.slice(0, 10),
      au: e.end_date.slice(0, 10),
      declare: true,
      revolu: e.end_date.slice(0, 10) < auj,
    }))
    .sort((a, b) => (a.du < b.du ? 1 : a.du > b.du ? -1 : 0))
}

/**
 * choisirParDefaut rend l'exercice à proposer d'emblée.
 *
 * Le dernier exercice RÉVOLU, et non celui en cours : on établit un carnet du
 * lait pour déclarer une année terminée. Proposer l'exercice courant sortirait
 * un document forcément incomplet, que son lecteur prendrait pour un bilan.
 * À défaut d'exercice révolu — une entreprise dans sa première année —, le
 * plus récent, faute de mieux et parce qu'un menu vide ne sert personne.
 */
export function choisirParDefaut(exercices: Exercice[]): Exercice | null {
  if (exercices.length === 0) return null
  return exercices.find(e => e.revolu) ?? exercices[0]
}
