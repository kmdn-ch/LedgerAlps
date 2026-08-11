// LedgerAlps — Utilitaires UI

import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { format, parseISO } from 'date-fns'
import { de, enGB, fr, it } from 'date-fns/locale'
import type { DisplayStatus } from '@/types'
import { localeIntl, type Cle, type Langue } from '@/i18n'
import { useLangueStore } from '@/i18n/useT'

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

// ─── Ces deux fonctions sont appelées HORS composant ──────────────────────────
//
// `formatCHF` et `formatDate` sont utilisées dans des rendus, des tableaux et
// des chaînes construites à la volée — cent cinquante appels, dont plusieurs
// hors de tout composant React. Les convertir en crochets aurait demandé de
// réécrire chaque appelant ; elles lisent donc la langue directement dans le
// magasin zustand, qui vit hors de React et rend la valeur du moment.
//
// C'est un compromis assumé : elles ne s'abonnent pas au magasin et ne
// provoquent donc aucun rendu à elles seules. En pratique, changer de langue
// re-rend l'écran par les textes qui, eux, passent par `useT()` — et ces
// fonctions sont rappelées dans la foulée avec la nouvelle langue.
function langueCourante(): Langue {
  return useLangueStore.getState().langue
}

// ─── Formatage CHF ────────────────────────────────────────────────────────────
//
// `1'234.50` est suisse, `1,234.50` est britannique. Le séparateur figé en
// `de-CH` affichait une apostrophe suisse sur une interface anglaise.
export function formatCHF(value: string | number, currency = 'CHF'): string {
  const n = typeof value === 'string' ? parseFloat(value) : value
  if (isNaN(n)) return `0.00 ${currency}`
  return (
    n.toLocaleString(localeIntl(langueCourante()),
      { minimumFractionDigits: 2, maximumFractionDigits: 2 }) +
    ' ' + currency
  )
}

// ─── Formatage date ───────────────────────────────────────────────────────────
//
// Le format suit la langue : `11.08.2026` en Suisse, `11/08/2026` pour un
// Britannique. Le figer en `dd.MM.yyyy` avec la locale française donnait des
// noms de mois français dès qu'un appelant demandait `MMMM`.
const LOCALES_DATE = { fr, de, it, en: enGB } as const

const FORMAT_PAR_LANGUE: Record<Langue, string> = {
  fr: 'dd.MM.yyyy', de: 'dd.MM.yyyy', it: 'dd.MM.yyyy', en: 'dd/MM/yyyy',
}

export function formatDate(iso: string | null | undefined, fmt?: string): string {
  if (!iso) return '—'
  const langue = langueCourante()
  // Un format explicite est respecté tel quel — il porte souvent une heure
  // (« dd.MM.yyyy HH:mm ») que l'appelant a choisie sciemment. Seul le format
  // par défaut suit la langue.
  const motif = fmt ?? FORMAT_PAR_LANGUE[langue]
  try { return format(parseISO(iso), motif, { locale: LOCALES_DATE[langue] }) }
  catch { return iso }
}

// ─── Badge status ─────────────────────────────────────────────────────────────
//
// La table porte des CLÉS, pas des mots : c'est une constante de module, et le
// badge s'affiche sur presque tous les écrans. Rester en français ici aurait
// laissé « Envoyée » au milieu de chaque liste allemande — l'endroit le plus
// visible, et le dernier qu'on pense à regarder.
export const STATUS_LABELS: Record<DisplayStatus, Cle> = {
  draft:     'statut.brouillon',
  sent:      'statut.envoyee',
  paid:      'statut.payee',
  overdue:   'statut.enRetard',
  cancelled: 'statut.annulee',
  archived:  'statut.archivee',
}

const STATUS_CLASS: Record<DisplayStatus, string> = {
  draft:     'badge-draft',
  sent:      'badge-sent',
  paid:      'badge-paid',
  overdue:   'badge-overdue',
  cancelled: 'badge-cancelled',
  archived:  'badge-archived',
}

export function statusCle(s: DisplayStatus): Cle | null { return STATUS_LABELS[s] ?? null }
export function statusClass(s: DisplayStatus): string { return STATUS_CLASS[s] ?? 'badge-draft' }

// ─── IBAN formaté ─────────────────────────────────────────────────────────────
export function formatIBAN(iban: string): string {
  const c = iban.replace(/\s/g, '').toUpperCase()
  return c.match(/.{1,4}/g)?.join(' ') ?? iban
}

// ─── Téléchargement blob ──────────────────────────────────────────────────────
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a   = document.createElement('a')
  a.href    = url
  a.download = filename
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// Une facture est en retard parce que la date d'échéance est passée, pas parce
// qu'on l'a décidé. Seule une facture envoyée peut l'être : un brouillon n'a
// jamais été réclamé, une facture payée ou annulée ne doit plus rien.
export function isOverdue(inv: { status: string; due_date: string | null; document_type?: string }): boolean {
  if (inv.status !== 'sent') return false
  if (inv.document_type && inv.document_type !== 'invoice') return false
  if (!inv.due_date) return false
  return inv.due_date < new Date().toISOString().slice(0, 10)
}

// estQRIBAN dit si un compte suisse est un QR-IBAN.
//
// La marque est l'identifiant d'institution — positions 5 à 9 de l'IBAN, plage
// 30000–31999 réservée par SIX aux comptes QR. Ce n'est pas un détail de
// présentation : une référence QR n'est acceptée QU'AVEC un QR-IBAN, et une
// Creditor Reference qu'avec un IBAN ordinaire (SIX IG v2.4 §4.2.2). Ranger
// l'un à la place de l'autre fait rejeter le virement par la banque.
//
// Le serveur applique la même règle (`compliance.IsQRIBAN`) et c'est lui qui
// tranche. Ici, elle sert à montrer tout de suite dans quelle case une valeur
// saisie à la main appartient, plutôt qu'à la faire refuser après coup.
export function estQRIBAN(valeur: string): boolean {
  const v = valeur.replace(/\s/g, '').toUpperCase()
  if (!/^CH\d{7}/.test(v)) return false
  const iid = parseInt(v.slice(4, 9), 10)
  return iid >= 30000 && iid <= 31999
}
