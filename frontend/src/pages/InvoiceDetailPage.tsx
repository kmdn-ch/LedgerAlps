// LedgerAlps — Détail d'une facture

import { useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft, Download, Eye, EyeOff, Send, CheckCircle,
  XCircle, Archive, Loader2,
} from 'lucide-react'
import { invoicesApi, downloadBlob } from '@/api/client'
import {
  PageHeader, StatusBadge, LoadingSpinner, ErrorBanner,
  SectionTitle, PDFPreview,
} from '@/components/ui'
import { formatCHF, formatDate } from '@/utils'
import type { DocumentStatus, Invoice } from '@/types'

// ─── Transitions de statut autorisées ────────────────────────────────────────

const TRANSITIONS: Record<DocumentStatus, { status: DocumentStatus; label: string; icon: typeof Send; className: string }[]> = {
  draft:     [
    { status: 'sent',      label: 'Marquer envoyée',  icon: Send,        className: 'btn-primary' },
    { status: 'cancelled', label: 'Annuler',           icon: XCircle,     className: 'btn-ghost text-danger-600' },
  ],
  sent:      [
    { status: 'paid',      label: 'Marquer payée',    icon: CheckCircle, className: 'btn-primary' },
    { status: 'overdue',   label: 'Marquer en retard',icon: Loader2,     className: 'btn-ghost text-warning-600' },
    { status: 'cancelled', label: 'Annuler',           icon: XCircle,     className: 'btn-ghost text-danger-600' },
  ],
  overdue:   [
    { status: 'paid',      label: 'Marquer payée',    icon: CheckCircle, className: 'btn-primary' },
    { status: 'cancelled', label: 'Annuler',           icon: XCircle,     className: 'btn-ghost text-danger-600' },
  ],
  paid:      [
    { status: 'archived',  label: 'Archiver',          icon: Archive,     className: 'btn-ghost' },
  ],
  cancelled: [
    { status: 'archived',  label: 'Archiver',          icon: Archive,     className: 'btn-ghost' },
  ],
  archived:  [],
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function InvoiceDetailPage() {
  const { invoiceId } = useParams<{ invoiceId: string }>()
  const navigate      = useNavigate()
  const qc            = useQueryClient()
  const [showPDF, setShowPDF] = useState(false)

  const { data: invoice, isLoading, error } = useQuery<Invoice>({
    queryKey: ['invoice', invoiceId],
    queryFn:  () => invoicesApi.get(invoiceId!).then(r => r.data),
    enabled:  !!invoiceId,
  })

  const transition = useMutation({
    mutationFn: ({ status, paymentDate }: { status: DocumentStatus; paymentDate?: string }) =>
      invoicesApi.updateStatus(invoiceId!, status, paymentDate ? { payment_date: paymentDate } : undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['invoice', invoiceId] })
      qc.invalidateQueries({ queryKey: ['invoices'] })
    },
  })

  const handleTransition = (status: DocumentStatus) => {
    const extra = status === 'paid'
      ? { paymentDate: new Date().toISOString().slice(0, 10) }
      : {}
    transition.mutate({ status, ...extra })
  }

  const handleDownload = async () => {
    if (!invoice) return
    const resp = await invoicesApi.downloadPDF(invoiceId!)
    downloadBlob(resp.data, `facture_${invoice.number}.pdf`)
  }

  if (isLoading) return <LoadingSpinner />
  if (error || !invoice) return (
    <ErrorBanner message="Impossible de charger la facture." />
  )

  const actions = TRANSITIONS[invoice.status] ?? []
  const totalRemaining = parseFloat(invoice.total) - parseFloat(invoice.amount_paid)

  return (
    <div className="max-w-4xl mx-auto">
      {/* En-tête */}
      <div className="flex items-center gap-3 mb-6">
        <button onClick={() => navigate(-1)} className="btn-ghost btn-sm">
          <ArrowLeft size={15} />
        </button>
        <PageHeader
          title={invoice.number}
          subtitle={`${invoice.document_type === 'invoice' ? 'Facture' : invoice.document_type} · émise le ${formatDate(invoice.issue_date)}`}
          actions={
            <div className="flex items-center gap-2">
              {actions.map(t => (
                <button
                  key={t.status}
                  onClick={() => handleTransition(t.status)}
                  disabled={transition.isPending}
                  className={`${t.className} btn-sm flex items-center gap-1.5`}
                >
                  <t.icon size={14} />
                  {t.label}
                </button>
              ))}
              <button
                onClick={() => setShowPDF(v => !v)}
                className="btn-ghost btn-sm flex items-center gap-1.5"
                title={showPDF ? 'Masquer l\'aperçu PDF' : 'Aperçu PDF'}
              >
                {showPDF ? <EyeOff size={14} /> : <Eye size={14} />}
                PDF
              </button>
              <button onClick={handleDownload} className="btn-ghost btn-sm" title="Télécharger PDF">
                <Download size={14} />
              </button>
            </div>
          }
        />
      </div>

      {transition.isError && (
        <ErrorBanner message="Erreur lors du changement de statut." />
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-5 mb-6">
        {/* Résumé financier */}
        <div className="lg:col-span-2 card">
          <SectionTitle>Montants</SectionTitle>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between text-alpine-600">
              <span>Sous-total HT</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.subtotal)}</span>
            </div>
            <div className="flex justify-between text-alpine-600">
              <span>TVA</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.vat_amount)}</span>
            </div>
            <div className="flex justify-between text-alpine-900 font-semibold border-t border-alpine-100 pt-2">
              <span>Total TTC</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.total)}</span>
            </div>
            {parseFloat(invoice.amount_paid) > 0 && (
              <>
                <div className="flex justify-between text-success-700">
                  <span>Déjà payé</span>
                  <span className="font-mono tabular-nums">−{formatCHF(invoice.amount_paid)}</span>
                </div>
                <div className="flex justify-between text-alpine-900 font-semibold border-t border-alpine-100 pt-2">
                  <span>Solde restant</span>
                  <span className="font-mono tabular-nums">{formatCHF(totalRemaining)}</span>
                </div>
              </>
            )}
          </div>
        </div>

        {/* Infos facture */}
        <div className="card space-y-3">
          <SectionTitle>Informations</SectionTitle>
          <dl className="text-sm space-y-2">
            <div className="flex justify-between">
              <dt className="text-alpine-500">Statut</dt>
              <dd><StatusBadge status={invoice.status} /></dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-alpine-500">Date d'émission</dt>
              <dd className="text-alpine-800">{formatDate(invoice.issue_date)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-alpine-500">Échéance</dt>
              <dd className={`${invoice.status === 'overdue' ? 'text-danger-600 font-medium' : 'text-alpine-800'}`}>
                {formatDate(invoice.due_date)}
              </dd>
            </div>
            {invoice.qr_iban && (
              <div className="flex justify-between">
                <dt className="text-alpine-500">QR-IBAN</dt>
                <dd className="text-alpine-700 font-mono text-xs truncate max-w-[120px]">{invoice.qr_iban}</dd>
              </div>
            )}
          </dl>
        </div>
      </div>

      {/* Lignes de facture */}
      <div className="card mb-6">
        <SectionTitle>Lignes</SectionTitle>
        <div className="table-wrapper">
          <table className="table">
            <thead>
              <tr>
                <th>#</th>
                <th>Description</th>
                <th className="text-right">Qté</th>
                <th className="text-right">P.U. CHF</th>
                <th className="text-right">Rabais</th>
                <th className="text-right">TVA</th>
                <th className="text-right">Total CHF</th>
              </tr>
            </thead>
            <tbody>
              {invoice.lines.map(line => (
                <tr key={line.id}>
                  <td className="text-alpine-400 w-8">{line.position}</td>
                  <td className="text-alpine-800">{line.description}</td>
                  <td className="text-right tabular-nums">
                    {line.quantity}{line.unit ? ` ${line.unit}` : ''}
                  </td>
                  <td className="text-right tabular-nums font-mono">{formatCHF(line.unit_price)}</td>
                  <td className="text-right tabular-nums text-alpine-500">
                    {parseFloat(line.discount_percent) > 0 ? `${line.discount_percent}%` : '—'}
                  </td>
                  <td className="text-right tabular-nums text-alpine-500">{line.vat_rate}%</td>
                  <td className="text-right tabular-nums font-mono font-medium">{formatCHF(line.line_total)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Notes / conditions */}
      {(invoice.notes || invoice.terms) && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
          {invoice.notes && (
            <div className="card">
              <SectionTitle>Remarques</SectionTitle>
              <p className="text-sm text-alpine-600 whitespace-pre-line">{invoice.notes}</p>
            </div>
          )}
          {invoice.terms && (
            <div className="card">
              <SectionTitle>Conditions</SectionTitle>
              <p className="text-sm text-alpine-600 whitespace-pre-line">{invoice.terms}</p>
            </div>
          )}
        </div>
      )}

      {/* Aperçu PDF inline */}
      {showPDF && (
        <PDFPreview
          fetchPDF={() => invoicesApi.downloadPDF(invoiceId!).then(r => r.data as Blob)}
          filename={`facture_${invoice.number}.pdf`}
          className="mb-6"
          onClose={() => setShowPDF(false)}
        />
      )}
    </div>
  )
}
