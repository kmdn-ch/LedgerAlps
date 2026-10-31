// LedgerAlps — Schéma partagé du formulaire facture / offre
//
// `NewInvoicePage` et `EditInvoicePage` portaient chacune leur copie de ce
// schéma, de `computeLineTotals` et d'une mini-fenêtre de création de contact —
// environ 345 lignes en double. `EditInvoicePage` l'annonçait elle-même, en
// tête de fichier : « Schéma identique à NewInvoicePage ». La duplication était
// donc connue et nommée, jamais résolue.
//
// Elle avait déjà dérivé : la copie de `NewInvoicePage` envoyait un champ
// `currency` à la création de contact, l'autre non — et la table `contacts` n'a
// aucune colonne de ce nom, si bien que Gin le jetait en silence. Deux copies
// éditées séparément, un écart que rien ne signalait.
//
// Le dernier changement de règle (adresse client obligatoire, v1.5.9) a dû
// toucher les trois fichiers en parallèle, et en a manqué un quatrième
// (`ContactDetailPage`), ce que l'audit 4 a relevé en C-2. C'est le coût
// concret de cette duplication, pas une gêne théorique.

import { z } from 'zod'

const lineSchema = z.object({
  description:  z.string().min(1, 'val.requis'),
  quantity:     z.coerce.number().positive(),
  unit:         z.string().optional(),
  unit_price:   z.coerce.number().positive('val.prixRequis'),
  discount_pct: z.coerce.number().min(0).max(100).default(0),
  vat_rate:     z.coerce.number().min(0).default(8.1),
})

export const invoiceFormSchema = z.object({
  document_type: z.enum(['invoice', 'quote', 'credit_note']).default('invoice'),
  contact_id:    z.string().min(1, 'val.contactRequis'),
  issue_date:    z.string().min(1, 'val.dateRequise'),
  due_date:      z.string().min(1, 'val.echeanceRequise'),
  notes:         z.string().optional(),
  terms:         z.string().optional(),
  lines:         z.array(lineSchema).min(1, 'val.auMoinsUneLigne'),
})

// zod 4 distingue ce que le formulaire CONTIENT (z.input : un champ coercé est
// « unknown », un défaut est facultatif) de ce que le schéma PRODUIT (z.infer).
// react-hook-form porte les deux : valeurs en entrée, données validées en sortie.
export type InvoiceFormData = z.infer<typeof invoiceFormSchema>
export type InvoiceFormInput = z.input<typeof invoiceFormSchema>

/**
 * roundCents arrondit au centime, les demis à l'écart de zéro : c'est
 * `compliance.RoundCents` du paquet Go `internal/core/compliance`.
 *
 * Le centime se décide sur des millionièmes entiers : 1.005 * 100 vaut
 * 100.49999999999999 en virgule flottante, et Math.round(v * 100) rendrait 1.00
 * pour un demi-centime qui doit monter. Cette duplication-là est structurelle —
 * le même calcul doit exister des deux côtés du réseau pour que l'aperçu à la
 * saisie corresponde au document émis — mais elle n'a pas à être répétée deux
 * fois DANS le frontend.
 */
export function roundCents(v: number): number {
  if (!Number.isFinite(v)) return v
  const centimes = Math.floor((Math.round(Math.abs(v) * 1e6) + 5000) / 10000)
  if (centimes === 0) return 0
  return (v < 0 ? -centimes : centimes) / 100
}

/**
 * computeLineTotals calcule base HT, TVA et total d'une ligne, au centime.
 * Les totaux du DOCUMENT ne sont pas la somme de ces TVA-là : voir
 * `computeDocumentTotals`.
 */
export function computeLineTotals(line: Partial<InvoiceFormInput['lines'][0]>) {
  const qty      = Number(line.quantity     ?? 1)
  const price    = Number(line.unit_price   ?? 0)
  const discount = Number(line.discount_pct ?? 0) / 100
  const vatRate  = Number(line.vat_rate     ?? 8.1) / 100
  const base     = roundCents(qty * price * (1 - discount))
  const vat      = roundCents(base * vatRate)
  return { base, vat, total: roundCents(base + vat) }
}

/**
 * computeDocumentTotals reproduit `computeTotals` du service Go : HT = somme des
 * lignes arrondies, TVA calculée PAR TAUX sur la somme des bases de ce taux puis
 * arrondie, total = HT + TVA exactement. Depuis la 1.6.6 le total n'est plus arrondi
 * à 5 centimes : la facture, son QR et son écriture au journal portent le même
 * montant au centime près.
 */
export function computeDocumentTotals(lines: Partial<InvoiceFormInput['lines'][0]>[]) {
  const parLigne = lines.map(computeLineTotals)
  const tvaParTaux = new Map<number, number>()
  let subtotal = 0
  lines.forEach((l, i) => {
    const taux = Number(l.vat_rate ?? 8.1)
    subtotal += parLigne[i].base
    tvaParTaux.set(taux, (tvaParTaux.get(taux) ?? 0) + (parLigne[i].base * taux) / 100)
  })
  let vat = 0
  tvaParTaux.forEach(v => { vat += roundCents(v) })
  subtotal = roundCents(subtotal)
  vat = roundCents(vat)
  return { lines: parLigne, subtotal, vat, total: roundCents(subtotal + vat) }
}
