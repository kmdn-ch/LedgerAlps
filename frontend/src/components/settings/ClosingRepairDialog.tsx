// LedgerAlps — Complément d'une clôture déséquilibrée (Paramètres → Maintenance → Diagnostic)
//
// Les clôtures passées avant la 1.6.6 ignoraient les produits à solde débiteur et les charges à
// solde créditeur : l'écriture scellée ne s'équilibrait pas, et ces comptes restaient ouverts.
// Le journal est immuable, et une écriture ordinaire ne rétablit pas l'équilibre d'une écriture
// qui ne l'est pas.
//
// Cette fenêtre montre le COMPLÉMENT — exactement les lignes qui manquaient — avant de le passer.
// Rien n'est modifié ni supprimé ; le geste est tracé au journal d'audit comme toute écriture.
// L'application exige une confirmation, après l'aperçu : une réparation du journal ne part pas
// d'un clic distrait.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, X } from 'lucide-react'
import { fiscalYearsApi } from '@/api/client'
import { ErrorBanner, LoadingSpinner } from '@/components/ui'
import { formatCHF, formatDate } from '@/utils'
import { refusalMessage } from '@/utils/refusal'
import type { ClosingRepairPlan, FindingRepair } from '@/types'
import { useT } from '@/i18n/useT'

interface Props {
  repair: FindingRepair
  onClose: () => void
  /** Appelé une fois le complément passé, avec sa référence. */
  onDone: (reference: string) => void
}

export function ClosingRepairDialog({ repair, onClose, onDone }: Props) {
  const t = useT()
  const qc = useQueryClient()

  const plan = useQuery<ClosingRepairPlan>({
    queryKey: ['closing-repair', repair.fiscal_year_id],
    queryFn:  () => fiscalYearsApi.closingRepairPlan(repair.fiscal_year_id).then(r => r.data),
    retry: false,
  })

  const apply = useMutation({
    mutationFn: () => fiscalYearsApi.repairClosing(repair.fiscal_year_id).then(r => r.data as ClosingRepairPlan),
    onSuccess: (fait) => {
      // Le complément entre au journal et à la balance : tout ce qui les lit est périmé.
      qc.invalidateQueries({ queryKey: ['maintenance'] })
      qc.invalidateQueries({ queryKey: ['journal'] })
      qc.invalidateQueries({ queryKey: ['trial-balance'] })
      qc.invalidateQueries({ queryKey: ['closing-repair'] })
      onDone(fait.reference)
    },
  })

  const p = plan.data

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div className="bg-white rounded-xl shadow-2xl w-full max-w-2xl mx-4 max-h-[90vh] overflow-y-auto"
           role="dialog" aria-modal="true" aria-labelledby="closing-repair-title">
        <div className="flex items-center justify-between px-5 py-4 border-b border-alpine-200">
          <h3 id="closing-repair-title" className="text-sm font-semibold text-alpine-900">
            {t('dg.complementTitre', { exercice: repair.fiscal_year })}
          </h3>
          <button type="button" onClick={onClose} className="btn-ghost btn-sm p-1 text-alpine-400"
                  aria-label={t('action.fermer')}>
            <X size={16} />
          </button>
        </div>

        <div className="px-5 py-4 space-y-3 text-sm">
          {plan.isLoading && <LoadingSpinner />}
          {plan.isError && <ErrorBanner message={refusalMessage(plan.error, t('dg.complementErreurApercu'))} />}

          {p && (
            <>
              <p className="text-alpine-700">
                {t('dg.complementEcart', {
                  exercice: p.fiscal_year, ref: p.closing_reference, ecart: formatCHF(p.gap),
                })}
              </p>
              <p className="text-alpine-600">{t('dg.complementIntro')}</p>

              <div>
                <p className="text-xs text-alpine-500 mb-1">
                  {t('dg.complementEntree', { ref: p.reference, date: formatDate(p.date) })}
                </p>
                <div className="table-wrapper">
                  <table className="table">
                    <thead>
                      <tr>
                        <th>{t('dg.complementCompte')}</th>
                        <th>{t('dg.complementLibelle')}</th>
                        <th className="text-right">{t('dg.complementDebit')}</th>
                        <th className="text-right">{t('dg.complementCredit')}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {p.lines.map(l => (
                        <tr key={l.account_code}>
                          <td className="font-mono">{l.account_code}</td>
                          <td>{l.account_name}</td>
                          <td className="text-right tabular-nums">{l.debit > 0 ? formatCHF(l.debit) : ''}</td>
                          <td className="text-right tabular-nums">{l.credit > 0 ? formatCHF(l.credit) : ''}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>

              <p className="text-xs text-alpine-500">{t('dg.complementFiduciaire')}</p>
            </>
          )}

          {apply.isError && (
            <ErrorBanner message={refusalMessage(apply.error, t('dg.complementErreur'))} />
          )}
        </div>

        <div className="flex justify-end gap-2 px-5 py-3 border-t border-alpine-200">
          <button type="button" className="btn-ghost btn-sm" onClick={onClose}>
            {t('action.annuler')}
          </button>
          <button type="button" className="btn-primary btn-sm flex items-center gap-1.5"
                  disabled={!p || apply.isPending} onClick={() => apply.mutate()}>
            {apply.isPending && <Loader2 size={13} className="animate-spin" />}
            {apply.isPending ? t('dg.complementEnCours') : t('dg.complementPasser')}
          </button>
        </div>
      </div>
    </div>
  )
}
