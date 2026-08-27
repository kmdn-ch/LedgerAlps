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

export const lineSchema = z.object({
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

export type InvoiceFormData = z.infer<typeof invoiceFormSchema>

/**
 * computeLineTotals calcule base HT, TVA et total d'une ligne.
 *
 * L'arrondi au 5 rappen (`* 20) / 20`) reproduit `RoundTo5Rappen` du paquet Go
 * `internal/core/compliance`. Cette duplication-là est structurelle — le même
 * calcul doit exister des deux côtés du réseau pour que l'aperçu à la saisie
 * corresponde au document émis — mais elle n'a pas à être répétée deux fois
 * DANS le frontend.
 */
export function computeLineTotals(line: Partial<InvoiceFormData['lines'][0]>) {
  const qty      = Number(line.quantity     ?? 1)
  const price    = Number(line.unit_price   ?? 0)
  const discount = Number(line.discount_pct ?? 0) / 100
  const vatRate  = Number(line.vat_rate     ?? 8.1) / 100
  const base     = qty * price * (1 - discount)
  const vat      = Math.round(base * vatRate * 20) / 20  // arrondi 5 rappen
  return {
    base:  Math.round(base * 100) / 100,
    vat:   Math.round(vat  * 100) / 100,
    total: Math.round((base + vat) * 100) / 100,
  }
}
