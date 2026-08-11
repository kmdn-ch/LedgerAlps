// LedgerAlps — Documents d'un client, filtrables et téléchargeables
//
// Depuis une fiche client, on veut souvent repartir avec « toutes ses factures
// 2025 » : pour la fiduciaire, pour répondre à une demande d'accès, ou pour
// archiver. Les télécharger une par une est possible mais absurde au-delà de
// dix.
//
// Sur la légalité de cet export : ces documents sont vos pièces comptables, que
// le CO art. 958f vous impose de conserver dix ans. Les exporter ne pose aucune
// question de principe — c'est même le moyen de répondre à une demande d'accès
// (nLPD art. 25) ou de remise des données (art. 28), qui doit être gratuite et
// traitée dans les trente jours. Ce qui engage commence après : le fichier
// contient des données personnelles et quitte la machine.
//
// Les filtres portent côté serveur, pas dans le navigateur : la liste est
// paginée, donc un filtre local ne verrait que la page chargée et donnerait un
// résultat différent selon l'endroit où l'on se trouve.

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Download, Loader2, FileDown, Info } from 'lucide-react'
import { invoicesApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner, EmptyState, StatusBadge } from '@/components/ui'
import { formatCHF, formatDate, isOverdue } from '@/utils'
import type { Invoice, DocumentType } from '@/types'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

const DOC_LABEL: Record<string, Cle> = {
  invoice:     'doc.facture',
  quote:       'doc.offre',
  credit_note: 'doc.noteDeCredit',
}

const STATUSES: { value: string; cle: Cle }[] = [
  { value: '',          cle: 'cd.tousStatuts' },
  { value: 'draft',     cle: 'statut.brouillon' },
  { value: 'sent',      cle: 'statut.envoyee' },
  { value: 'overdue',   cle: 'statut.enRetard' },
  { value: 'paid',      cle: 'statut.payee' },
  { value: 'cancelled', cle: 'statut.annulee' },
  { value: 'archived',  cle: 'statut.archivee' },
]

const TYPES: { value: string; cle: Cle }[] = [
  { value: '',            cle: 'cd.tousTypes' },
  { value: 'invoice',     cle: 'cd.typesFactures' },
  { value: 'quote',       cle: 'cd.typesOffres' },
  { value: 'credit_note', cle: 'cd.typesNotesCredit' },
]

export function ContactDocuments({ contactId, contactName }: {
  contactId: string
  contactName: string
}) {
  const t = useT()
  const [status, setStatus] = useState('')
  const [docType, setDocType] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const docs = useQuery<Invoice[]>({
    queryKey: ['contact-docs', contactId, status, docType, from, to],
    queryFn: () => invoicesApi.list({
      contact_id: contactId,
      page_size: 100,
      status:        status  || undefined,
      document_type: docType || undefined,
      from:          from    || undefined,
      to:            to      || undefined,
    }).then(r => r.data.items ?? []),
  })

  const rows = docs.data ?? []
  // La sélection est remise à zéro quand les filtres changent : garder une
  // ligne cochée qui n'est plus affichée ferait télécharger un document que
  // l'utilisateur ne voit pas.
  const visibleIds = rows.map(r => r.id)
  const effective = [...selected].filter(id => visibleIds.includes(id))
  const allSelected = rows.length > 0 && effective.length === rows.length

  function toggle(id: string) {
    setSelected(prev => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })
  }

  function toggleAll() {
    setSelected(allSelected ? new Set() : new Set(visibleIds))
  }

  async function download() {
    if (effective.length === 0) return
    setBusy(true); setError(null)
    try {
      const res = await invoicesApi.bulkPDF(effective)
      const disposition: string = res.headers['content-disposition'] ?? ''
      const match = /filename="([^"]+)"/.exec(disposition)
      const url = URL.createObjectURL(res.data)
      const a = document.createElement('a')
      a.href = url
      a.download = match ? match[1] : 'documents.zip'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)

      // Le serveur signale les documents disparus entre l'affichage et le clic.
      // Une archive plus courte que la sélection, sans un mot, se remarque trop
      // tard — souvent une fois transmise.
      const missing = Number(res.headers['x-ledgeralps-missing'] ?? 0)
      if (missing > 0) {
        setError(t('cd.documentsDisparus', { n: missing }))
      }
    } catch {
      setError(t('cd.erreurTelechargement'))
    } finally {
      setBusy(false)
    }
  }

  const resetFilters = () => {
    setStatus(''); setDocType(''); setFrom(''); setTo(''); setSelected(new Set())
  }
  const filtered = status || docType || from || to

  return (
    <div>
      <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
        <SectionTitle>{t('cd.documents', { n: rows.length })}</SectionTitle>
        <button
          onClick={download}
          disabled={effective.length === 0 || busy}
          className="btn-primary btn-sm flex items-center gap-1.5 disabled:opacity-50"
          title={effective.length > 1
            ? t('cd.infoBulleZIP', { n: effective.length })
            : t('cd.infoBullePDF')}
        >
          {busy
            ? <><Loader2 size={13} className="animate-spin" /> {t('sv.preparationEnCours')}</>
            : effective.length > 1
              ? <><FileDown size={13} /> {t('cd.telechargerZIP', { n: effective.length })}</>
              : <><Download size={13} /> {effective.length === 1
                    ? t('cd.telechargerLePDF') : t('action.telecharger')}</>}
        </button>
      </div>

      {/* ── Filtres ─────────────────────────────────────────────────────────── */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        <select className="input text-sm py-1.5" value={docType}
                onChange={e => { setDocType(e.target.value); setSelected(new Set()) }}
                aria-label={t('cd.ariaTypeDocument')}>
          {TYPES.map(o => <option key={o.value} value={o.value}>{t(o.cle)}</option>)}
        </select>
        <select className="input text-sm py-1.5" value={status}
                onChange={e => { setStatus(e.target.value); setSelected(new Set()) }}
                aria-label={t('cd.ariaStatut')}>
          {STATUSES.map(s => <option key={s.value} value={s.value}>{t(s.cle)}</option>)}
        </select>
        <input type="date" className="input text-sm py-1.5" value={from}
               onChange={e => { setFrom(e.target.value); setSelected(new Set()) }}
               aria-label={t('cd.ariaEmisDepuis')} />
        <input type="date" className="input text-sm py-1.5" value={to}
               onChange={e => { setTo(e.target.value); setSelected(new Set()) }}
               aria-label={t('cd.ariaEmisJusqu')} />
        {filtered && (
          <button onClick={resetFilters} className="btn-ghost btn-sm text-xs">
            {t('cd.reinitialiser')}
          </button>
        )}
      </div>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}
      {docs.isLoading && <LoadingSpinner />}
      {docs.isError && <ErrorBanner message={t('cd.erreurChargement')} />}

      {docs.data && rows.length === 0 && (
        <EmptyState
          title={t(filtered ? 'cd.aucunCorrespond' : 'cd.aucunDocument')}
          description={filtered
            ? t('cd.elargirPeriode')
            : t('cd.aucuneEtablie', { nom: contactName })}
        />
      )}

      {rows.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th className="w-8">
                    <input
                      type="checkbox"
                      checked={allSelected}
                      onChange={toggleAll}
                      aria-label={t('cd.toutSelectionner')}
                    />
                  </th>
                  <th>{t('fact.colNumero')}</th><th>{t('cd.colType')}</th>
                  <th>{t('fact.colDate')}</th>
                  <th className="text-right">{t('cd.colTotal')}</th>
                  <th>{t('fact.colStatut')}</th>
                </tr>
              </thead>
              <tbody>
                {rows.map(d => (
                  <tr key={d.id} className={selected.has(d.id) ? 'bg-alpine-50' : ''}>
                    <td>
                      <input
                        type="checkbox"
                        checked={selected.has(d.id)}
                        onChange={() => toggle(d.id)}
                        aria-label={t('cd.ariaSelectionner', { numero: d.invoice_number })}
                      />
                    </td>
                    <td>
                      <Link to={`/invoices/${d.id}`}
                            className="font-mono text-accent-700 hover:text-accent-600 font-medium">
                        {d.invoice_number}
                      </Link>
                    </td>
                    <td className="text-alpine-600">
                      {DOC_LABEL[d.document_type as DocumentType]
                        ? t(DOC_LABEL[d.document_type as DocumentType]) : d.document_type}
                    </td>
                    <td className="text-alpine-600">{formatDate(d.issue_date)}</td>
                    <td className="text-right font-mono tabular-nums">{formatCHF(d.total_amount)}</td>
                    <td><StatusBadge status={d.status} overdue={isOverdue(d)} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <p className="mt-3 text-xs text-alpine-500 flex items-start gap-1.5">
            <Info size={12} className="mt-0.5 flex-shrink-0" />
            {t('cd.mentionExport')}
          </p>
        </>
      )}
    </div>
  )
}
