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
import type { BankEntry } from '@/types'

const CONFIDENCE: Record<string, string> = {
  certaine: 'text-success-700',
  probable: 'text-warning-700',
  possible: 'text-alpine-600',
}

export function ReconciliationPanel() {
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
        `${d.imported ?? 0} écriture(s) ajoutée(s)` +
        (d.duplicate ? `, ${d.duplicate} déjà connue(s) et ignorée(s)` : ''),
      )
      invalidate()
    },
    onError: (e) => setError(refusalMessage(e, "Le relevé n'a pas pu être lu.")),
  })

  const match   = useMutation({ mutationFn: (v: { id: string; invoiceId: string }) =>
                                  bankEntriesApi.match(v.id, v.invoiceId), onSuccess: invalidate })
  const unmatch = useMutation({ mutationFn: (id: string) => bankEntriesApi.unmatch(id), onSuccess: invalidate })
  const ignore  = useMutation({ mutationFn: (v: { id: string; ignored: boolean }) =>
                                  bankEntriesApi.ignore(v.id, v.ignored), onSuccess: invalidate })

  const items = entries.data?.items ?? []

  return (
    <div>
      <SectionTitle>Rapprochement bancaire</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        Importez le relevé <code className="text-xs">camt.053</code> de votre banque. LedgerAlps
        garde les écritures et propose la facture correspondante quand il peut la désigner sans
        deviner.
      </p>

      <div className="rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm mb-4">
        <div className="flex items-start gap-2">
          <Info size={15} className="mt-0.5 flex-shrink-0 text-alpine-500" />
          <p className="text-alpine-700">
            <strong>Rapprocher n'encaisse pas.</strong> Identifier un versement et enregistrer un
            paiement sont deux gestes distincts : le second se fait depuis la facture, après
            vérification. Une facture soldée parce qu'un montant correspondait est une erreur qu'on
            ne découvre qu'en relançant un client qui a déjà payé.
          </p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-3 mb-4">
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
          Importer un relevé
        </button>
        <label className="flex items-center gap-1.5 text-sm text-alpine-700">
          <input type="checkbox" checked={showAll} onChange={e => setShowAll(e.target.checked)} />
          Voir aussi les écritures déjà traitées
        </label>
      </div>

      {imported && <p className="text-sm text-success-700 mb-3">{imported}</p>}
      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}

      {entries.isLoading && <LoadingSpinner />}
      {entries.isError && <ErrorBanner message="Les écritures bancaires n'ont pas pu être lues." />}

      {entries.data && items.length === 0 && (
        <EmptyState
          icon={<Upload size={28} />}
          title={showAll ? 'Aucune écriture' : 'Rien à rapprocher'}
          description={showAll
            ? "Importez un relevé camt.053 pour commencer."
            : "Toutes les écritures importées ont été traitées."}
        />
      )}

      {items.length > 0 && (
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th>Date</th>
                <th className="text-right">Montant</th>
                <th>Contrepartie</th>
                <th>Proposition</th>
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
                        aucune facture ne correspond sans ambiguïté
                      </span>
                    )}
                  </td>
                  <td className="whitespace-nowrap text-right">
                    {e.invoice_id ? (
                      <button
                        onClick={() => unmatch.mutate(e.id)}
                        className="btn-ghost btn-sm flex items-center gap-1"
                        title="Défaire le rapprochement"
                      >
                        <Undo2 size={13} /> Défaire
                      </button>
                    ) : (
                      <div className="flex items-center justify-end gap-1">
                        {e.suggestion && (
                          <button
                            onClick={() => match.mutate({ id: e.id, invoiceId: e.suggestion!.invoice_id })}
                            className="btn-secondary btn-sm flex items-center gap-1"
                          >
                            <Link2 size={13} /> Rapprocher
                          </button>
                        )}
                        <button
                          onClick={() => ignore.mutate({ id: e.id, ignored: true })}
                          className="btn-ghost btn-sm flex items-center gap-1"
                          title="Ne concerne aucune facture — frais bancaires, virement interne"
                        >
                          <EyeOff size={13} /> Écarter
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
