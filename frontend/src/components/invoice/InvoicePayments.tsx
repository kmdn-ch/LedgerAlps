// LedgerAlps — Paiements d'une facture
//
// Jusqu'à la 1.6.4, cet écran n'existait pas : le seul geste était « Marquer
// payée », qui changeait le statut sans rien passer au journal, et la fenêtre de
// confirmation renvoyait vers un « onglet Paiements » qui n'existait nulle part.
// Un client qui payait en deux fois restait donc « à payer » jusqu'au bout.
//
// Ici, chaque versement — partiel ou non — s'enregistre avec sa date et son mode.
// Le serveur le plafonne au solde, le cumule, le passe au journal quand
// l'émission de la facture l'a été, et solde la facture quand le cumul atteint le
// total (voir invoicing.RecordPayment).

import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { invoicePaymentsApi } from '@/api/client'
import { SectionTitle } from '@/components/ui'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'
import { formatCHF, formatDate } from '@/utils'
import { businessRefusal, refusalMessage } from '@/utils/refusal'
import type { Invoice, Payment, PaymentMethod } from '@/types'

const METHODS: { value: PaymentMethod; cle: Cle }[] = [
  { value: 'bank_transfer', cle: 'pm.virement' },
  { value: 'cash',          cle: 'pm.especes' },
  { value: 'card',          cle: 'pm.carte' },
  { value: 'check',         cle: 'pm.cheque' },
  { value: 'other',         cle: 'pm.autre' },
]

const METHOD_LABEL: Record<PaymentMethod, Cle> = {
  bank_transfer: 'pm.virement',
  cash:          'pm.especes',
  card:          'pm.carte',
  check:         'pm.cheque',
  other:         'pm.autre',
}

// Un centime de tolérance : l'écart d'arrondi d'une saisie à deux décimales.
const TOLERANCE = 0.005

const aujourdhui = () => new Date().toISOString().slice(0, 10)

interface Props {
  invoice: Invoice
  /** Le compte peut-il écrire ? Sans cela, la liste reste lisible mais rien ne se saisit. */
  canRecord: boolean
}

export function InvoicePayments({ invoice, canRecord }: Props) {
  const t = useT()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)

  const payments = useQuery<Payment[]>({
    queryKey: ['payments', invoice.id],
    queryFn: () => invoicePaymentsApi.list(invoice.id).then(r => (r.data.items ?? []) as Payment[]),
  })

  const balance = Math.round((invoice.total_amount - invoice.amount_paid) * 100) / 100
  const peutEncaisser = canRecord && invoice.document_type !== 'quote'
    && invoice.status === 'sent' && balance > TOLERANCE
  const liste = payments.data ?? []

  // Rien à montrer et rien à faire : pas de section vide.
  if (liste.length === 0 && !peutEncaisser) return null

  return (
    <div className="card card-pad mb-6">
      <div className="flex items-center justify-between mb-1">
        <SectionTitle>{t('pm.titre')}</SectionTitle>
        {peutEncaisser && (
          <button
            type="button"
            onClick={() => { setNotice(null); setOpen(true) }}
            className="btn-secondary btn-sm flex items-center gap-1.5 -mt-3"
          >
            <Plus size={14} />
            {t('pm.enregistrer')}
          </button>
        )}
      </div>

      {notice && (
        <p className="mb-3 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2 text-sm" role="status">
          {notice}
        </p>
      )}

      {liste.length === 0 ? (
        <p className="text-sm text-alpine-500">{t('pm.aucun')}</p>
      ) : (
        <div className="table-wrapper">
          <table className="table">
            <thead>
              <tr>
                <th>{t('pm.colDate')}</th>
                <th>{t('pm.colMode')}</th>
                <th>{t('pm.colReference')}</th>
                <th className="text-right">{t('pm.colMontant')}</th>
              </tr>
            </thead>
            <tbody>
              {liste.map(p => (
                <tr key={p.id}>
                  <td className="tabular-nums">{formatDate(p.payment_date)}</td>
                  <td>{METHOD_LABEL[p.method] ? t(METHOD_LABEL[p.method]) : p.method}</td>
                  <td className="text-alpine-600">{p.reference ?? '—'}</td>
                  <td className="text-right tabular-nums font-mono">{formatCHF(p.amount)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {open && (
        <PaymentModal
          invoice={invoice}
          balance={balance}
          onClose={() => setOpen(false)}
          onRecorded={(journalPosted) => {
            setOpen(false)
            setNotice(journalPosted ? t('pm.enregistre') : t('pm.horsJournal'))
            qc.invalidateQueries({ queryKey: ['invoice', invoice.id] })
            qc.invalidateQueries({ queryKey: ['invoices'] })
            qc.invalidateQueries({ queryKey: ['payments', invoice.id] })
          }}
        />
      )}
    </div>
  )
}

interface ModalProps {
  invoice: Invoice
  balance: number
  onClose: () => void
  onRecorded: (journalPosted: boolean) => void
}

function PaymentModal({ invoice, balance, onClose, onRecorded }: ModalProps) {
  const t = useT()
  const [amount, setAmount] = useState(balance.toFixed(2))
  const [date, setDate] = useState(aujourdhui())
  const [method, setMethod] = useState<PaymentMethod>('bank_transfer')
  const [reference, setReference] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [tried, setTried] = useState(false)

  const value = Number(amount.replace(',', '.'))
  const amountOk = Number.isFinite(value) && value > 0 && value <= balance + TOLERANCE

  const record = useMutation({
    mutationFn: () => invoicePaymentsApi.record({
      invoice_id:   invoice.id,
      amount:       Math.round(value * 100) / 100,
      payment_date: date,
      method,
      reference:    reference.trim() || undefined,
    }),
    onSuccess: (res) => onRecorded(res.data.journal_posted !== false),
    onError: (e) => setErr(businessRefusal(e) ?? refusalMessage(e, t('pm.erreur'))),
  })

  const submit = () => {
    setTried(true)
    setErr(null)
    if (!amountOk || !date) return
    record.mutate()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div
        className="bg-white rounded-xl shadow-2xl w-full max-w-md mx-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="payment-title"
      >
        <div className="flex items-center justify-between px-5 py-4 border-b border-alpine-200">
          <h3 id="payment-title" className="text-sm font-semibold text-alpine-900">
            {t('pm.titreModal', { numero: invoice.invoice_number })}
          </h3>
          <button type="button" onClick={onClose} className="btn-ghost btn-sm p-1 text-alpine-400">
            <X size={16} />
          </button>
        </div>

        <div className="px-5 py-4 space-y-3">
          {err && <p className="text-xs text-danger-700 bg-danger-100 rounded px-3 py-2" role="alert">{err}</p>}

          <p className="text-sm text-alpine-600">
            {t('pm.soldeRestant', { montant: formatCHF(balance) })}
          </p>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="label" htmlFor="pay-amount">{t('pm.montant')}</label>
              <input
                id="pay-amount"
                inputMode="decimal"
                className={`input text-right tabular-nums ${tried && !amountOk ? 'input-error' : ''}`}
                value={amount}
                onChange={e => setAmount(e.target.value)}
              />
            </div>
            <div>
              <label className="label" htmlFor="pay-date">{t('pm.date')}</label>
              <input
                id="pay-date"
                type="date"
                className={`input ${tried && !date ? 'input-error' : ''}`}
                value={date}
                onChange={e => setDate(e.target.value)}
              />
            </div>
          </div>
          {tried && !amountOk && (
            <p className="text-xs text-danger-700">{t('pm.montantInvalide')}</p>
          )}

          <div>
            <label className="label" htmlFor="pay-method">{t('pm.mode')}</label>
            <select
              id="pay-method"
              className="input"
              value={method}
              onChange={e => setMethod(e.target.value as PaymentMethod)}
            >
              {METHODS.map(m => <option key={m.value} value={m.value}>{t(m.cle)}</option>)}
            </select>
            <p className="mt-1 text-xs text-alpine-500">{t('pm.aideCompte')}</p>
          </div>

          <div>
            <label className="label" htmlFor="pay-ref">{t('pm.reference')}</label>
            <input
              id="pay-ref"
              className="input"
              value={reference}
              onChange={e => setReference(e.target.value)}
            />
          </div>
        </div>

        <div className="flex justify-end gap-2 border-t border-alpine-200 px-5 py-3">
          <button type="button" onClick={onClose} className="btn-ghost btn-sm">
            {t('action.annuler')}
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={record.isPending}
            className="btn-primary btn-sm"
          >
            {record.isPending ? t('pm.enCours') : t('pm.valider')}
          </button>
        </div>
      </div>
    </div>
  )
}
