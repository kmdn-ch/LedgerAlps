// Rapprochement bancaire — Paramètres → Banque.
//
// L'écran montre les écritures d'un relevé qui n'ont pas encore été
// identifiées, avec au plus une suggestion chacune et la raison qui l'a
// désignée. « Même montant » et « référence du bulletin » n'engagent pas la
// même confiance, et l'utilisateur doit pouvoir en tenir compte sans ouvrir la
// facture.
//
// Rapprocher n'encaisse pas, et l'écran le dit. Solder une facture parce qu'un
// montant correspond ferait passer pour réglée une créance que personne n'a
// vérifiée — l'erreur ne se découvre qu'en relançant un client qui a déjà payé,
// ou en ne relançant jamais celui qui n'a pas payé.

import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Upload, Link2, EyeOff, Undo2, Loader2, Info } from 'lucide-react'
import { bankEntriesApi, isoApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner, EmptyState } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { formatDate } from '@/utils'
import { useCanWrite } from '@/hooks/usePermissions'
import type { BankEntry } from '@/types'
import { useT } from '@/i18n/useT'

const CONFIDENCE: Record<string, string> = {
  certaine: 'text-success-700',
  probable: 'text-warning-700',
  possible: 'text-alpine-600',
}

export function ReconciliationPanel() {
  const t = useT()
  const peutEcrire = useCanWrite()
  const qc = useQueryClient()
  const fileRef = useRef<HTMLInputElement>(null)
  const [showAll, setShowAll] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [imported, setImported] = useState<string | null>(null)

  const entries = useQuery<{ items: BankEntry[] }>({
    queryKey: ['bank-entries', showAll],
    queryFn:  () => bankEntriesApi.list(showAll).then(r => r.data),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['bank-entries'] })

  const importStatement = useMutation({
    mutationFn: (file: File) => isoApi.importCamt053(file),
    onSuccess: (r) => {
      setError(null)
      const d = r.data as { imported?: number; duplicate?: number }
      setImported(
        t('rc.ecrituresAjoutees', { n: d.imported ?? 0 })
        + (d.duplicate ? t('rc.dejaConnues', { n: d.duplicate }) : ''),
      )
      invalidate()
    },
    onError: (e) => setError(refusalMessage(e, t('rc.echecReleve'))),
  })

  const match   = useMutation({ mutationFn: (v: { id: string; invoiceId: string }) =>
                                  bankEntriesApi.match(v.id, v.invoiceId), onSuccess: invalidate })
  const unmatch = useMutation({ mutationFn: (id: string) => bankEntriesApi.unmatch(id), onSuccess: invalidate })
  const ignore  = useMutation({ mutationFn: (v: { id: string; ignored: boolean }) =>
                                  bankEntriesApi.ignore(v.id, v.ignored), onSuccess: invalidate })

  const items = entries.data?.items ?? []

  return (
    <div>
      <SectionTitle>{t('rc.titre')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        {t('rc.introduction')}
      </p>

      <div className="rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm mb-4">
        <div className="flex items-start gap-2">
          <Info size={15} className="mt-0.5 flex-shrink-0 text-alpine-500" />
          <p className="text-alpine-700">
            {t('rc.nEncaissePas')}
          </p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3 mb-4">
        {/* Déposer un relevé écrit des écritures bancaires en base. Ni le champ
            de fichier ni le bouton n'existent dans la page pour un compte en
            lecture seule — le filtre de consultation, lui, reste : regarder ce
            qui a déjà été rapproché n'est pas une modification. */}
        {peutEcrire && (
          <>
            <input
              ref={fileRef}
              type="file"
              accept=".xml,text/xml,application/xml"
              className="hidden"
              onChange={e => {
                const f = e.target.files?.[0]
                if (f) importStatement.mutate(f)
                e.target.value = ''
              }}
            />
            <button
              onClick={() => fileRef.current?.click()}
              disabled={importStatement.isPending}
              className="btn-primary btn-sm flex items-center gap-1.5"
            >
              {importStatement.isPending ? <Loader2 size={13} className="animate-spin" /> : <Upload size={13} />}
              {t('rc.importerReleve')}
            </button>
          </>
        )}
        <label className="flex items-center gap-1.5 text-sm text-alpine-700">
          <input type="checkbox" checked={showAll} onChange={e => setShowAll(e.target.checked)} />
          {t('rc.voirTraitees')}
        </label>
      </div>

      {imported && <p className="text-sm text-success-700 mb-3">{imported}</p>}
      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}

      {entries.isLoading && <LoadingSpinner />}
      {entries.isError && <ErrorBanner message={t('rc.echecLecture')} />}

      {entries.data && items.length === 0 && (
        <EmptyState
          icon={<Upload size={28} />}
          title={t(showAll ? 'rc.aucuneEcriture' : 'rc.rienARapprocher')}
          description={t(showAll ? 'rc.importezPourCommencer' : 'rc.toutTraite')}
        />
      )}

      {items.length > 0 && (
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th>{t('fact.colDate')}</th>
                <th className="text-right">{t('rc.colMontant')}</th>
                <th>{t('rc.colContrepartie')}</th>
                <th>{t('rc.colProposition')}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map(e => (
                <tr key={e.id}>
                  <td className="whitespace-nowrap">{formatDate(e.booking_date)}</td>
                  <td className={`text-right tabular-nums whitespace-nowrap ${
                    e.is_credit ? 'text-success-700' : 'text-alpine-700'
                  }`}>
                    {e.is_credit ? '+' : '−'}{e.amount.toFixed(2)} {e.currency}
                  </td>
                  <td className="max-w-[16rem] truncate" title={e.counterparty || e.remittance}>
                    {e.counterparty || e.remittance || <span className="text-alpine-500">—</span>}
                  </td>
                  <td>
                    {e.invoice_id ? (
                      <Link to={`/invoices/${e.invoice_id}`} className="text-accent-700 hover:text-accent-600 font-medium">
                        {e.invoice_number}
                      </Link>
                    ) : e.suggestion ? (
                      <div>
                        <Link
                          to={`/invoices/${e.suggestion.invoice_id}`}
                          className="text-accent-700 hover:text-accent-600 font-medium"
                        >
                          {e.suggestion.invoice_number}
                        </Link>
                        <span className="text-alpine-600"> · {e.suggestion.contact_name}</span>
                        <p className={`text-xs ${CONFIDENCE[e.suggestion.confidence] ?? 'text-alpine-500'}`}>
                          {e.suggestion.confidence} — {e.suggestion.reason}
                        </p>
                      </div>
                    ) : (
                      <span className="text-alpine-500 text-sm">
                        {t('rc.aucuneCorrespondance')}
                      </span>
                    )}
                  </td>
                  <td className="whitespace-nowrap text-right">
                    {/* Rapprocher, défaire et écarter modifient les livres :
                        un lecteur voit la colonne, sans commande dedans. */}
                    {!peutEcrire ? null : e.invoice_id ? (
                      <button
                        onClick={() => unmatch.mutate(e.id)}
                        className="btn-ghost btn-sm flex items-center gap-1"
                        title={t('rc.infoBulleDefaire')}
                      >
                        <Undo2 size={13} /> {t('rc.defaire')}
                      </button>
                    ) : (
                      <div className="flex items-center justify-end gap-1">
                        {e.suggestion && (
                          <button
                            onClick={() => match.mutate({ id: e.id, invoiceId: e.suggestion!.invoice_id })}
                            className="btn-secondary btn-sm flex items-center gap-1"
                          >
                            <Link2 size={13} /> {t('rc.rapprocher')}
                          </button>
                        )}
                        <button
                          onClick={() => ignore.mutate({ id: e.id, ignored: true })}
                          className="btn-ghost btn-sm flex items-center gap-1"
                          title={t('rc.infoBulleEcarter')}
                        >
                          <EyeOff size={13} /> {t('rc.ecarter')}
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
