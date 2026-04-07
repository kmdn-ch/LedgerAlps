// LedgerAlps — Utilitaires UI

import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { format, parseISO } from 'date-fns'
import { fr } from 'date-fns/locale'
import type { DocumentStatus } from '@/types'

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
const STATUS_LABELS: Record<DocumentStatus, string> = {
  draft:     'Brouillon',
  sent:      'Envoyée',
  paid:      'Payée',
  overdue:   'En retard',
  cancelled: 'Annulée',
  archived:  'Archivée',
}

const STATUS_CLASS: Record<DocumentStatus, string> = {
  draft:     'badge-draft',
  sent:      'badge-sent',
  paid:      'badge-paid',
  overdue:   'badge-overdue',
  cancelled: 'badge-cancelled',
  archived:  'badge-archived',
}

export function statusLabel(s: DocumentStatus): string { return STATUS_LABELS[s] ?? s }
export function statusClass(s: DocumentStatus): string { return STATUS_CLASS[s] ?? 'badge-draft' }

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
