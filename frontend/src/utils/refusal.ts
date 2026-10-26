// LedgerAlps — Traduction d'un refus du serveur en message actionnable
//
// Le serveur répond 409 quand la demande est bien formée mais se heurte à une
// règle légale, et joint un `reason` stable. Se contenter d'afficher le message
// brut marcherait, mais l'utilisateur reçoit alors une phrase qui commence par
// des chiffres. Une accroche courte dit d'abord CE QUI est refusé ; le détail
// du serveur, qui porte les montants, vient ensuite.

const LEAD: Record<string, string> = {
  credit_exceeds_invoice:
    "Impossible : le montant dépasse celui de la facture corrigée.",
  vat_without_number:
    "Impossible : vous ne pouvez pas facturer de TVA sans numéro de TVA.",
}

// La forme d'une erreur axios, réduite à ce qu'on en lit.
//
// `any` disait « je ne sais pas », ce qui est faux : on sait exactement
// quels deux champs comptent. Le dire rend la lecture vérifiable par le
// compilateur, et c'est le seul endroit du frontend où cette forme est
// déballée — les autres écrans passent par cette fonction.
type RefusHTTP = { response?: { data?: { reason?: string; error?: string } } }

// Le message d'un refus MÉTIER seulement, ou null.
//
// Un refus métier est celui que le serveur a marqué d'un `reason` : une règle
// légale que l'utilisateur peut corriger (adresse du client incomplète, TVA sans
// numéro). Son message, déjà traduit par le serveur, dit quoi faire — il vaut
// mieux que « erreur lors du changement de statut ». Les autres échecs
// (transition impossible, base indisponible) gardent le message générique de
// l'écran : leur texte brut n'aide personne.
export function businessRefusal(error: unknown): string | null {
  const data = (error as RefusHTTP | null | undefined)?.response?.data
  if (!data?.reason) return null
  const message = refusalMessage(error, '')
  // Les messages du serveur s'écrivent « minuscule en tête », comme toute erreur
  // Go ; seuls dans un bandeau, ils ouvrent une phrase.
  return message ? message.charAt(0).toUpperCase() + message.slice(1) : null
}

export function refusalMessage(error: unknown, fallback: string): string {
  const data = (error as RefusHTTP | null | undefined)?.response?.data
  if (!data) return fallback
  const lead = data.reason ? LEAD[data.reason] : undefined
  const detail: string = data.error ?? ''
  if (lead) return detail ? `${lead} ${detail}` : lead
  return detail || fallback
}
