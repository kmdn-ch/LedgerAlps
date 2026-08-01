// LedgerAlps — Utilitaires UI

import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { format, parseISO } from 'date-fns'
import { fr } from 'date-fns/locale'
import type { DisplayStatus } from '@/types'

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

// ─── Formatage CHF ────────────────────────────────────────────────────────────
export function formatCHF(value: string | number, currency = 'CHF'): string {
  const n = typeof value === 'string' ? parseFloat(value) : value
  if (isNaN(n)) return `0.00 ${currency}`
  return (
    n.toLocaleString('de-CH', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) +
    ' ' + currency
  )
}

// ─── Formatage date ───────────────────────────────────────────────────────────
export function formatDate(iso: string | null | undefined, fmt = 'dd.MM.yyyy'): string {
  if (!iso) return '—'
  try { return format(parseISO(iso), fmt, { locale: fr }) }
  catch { return iso }
}

// ─── Badge status ─────────────────────────────────────────────────────────────
const STATUS_LABELS: Record<DisplayStatus, string> = {
  draft:     'Brouillon',
  sent:      'Envoyée',
  paid:      'Payée',
  overdue:   'En retard',
  cancelled: 'Annulée',
  archived:  'Archivée',
}

const STATUS_CLASS: Record<DisplayStatus, string> = {
  draft:     'badge-draft',
  sent:      'badge-sent',
  paid:      'badge-paid',
  overdue:   'badge-overdue',
  cancelled: 'badge-cancelled',
  archived:  'badge-archived',
}

export function statusLabel(s: DisplayStatus): string { return STATUS_LABELS[s] ?? s }
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
