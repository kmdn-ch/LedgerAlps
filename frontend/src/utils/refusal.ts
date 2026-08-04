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

export function refusalMessage(error: unknown, fallback: string): string {
  const data = (error as any)?.response?.data
  if (!data) return fallback
  const lead = data.reason ? LEAD[data.reason] : undefined
  const detail: string = data.error ?? ''
  if (lead) return detail ? `${lead} ${detail}` : lead
  return detail || fallback
}
