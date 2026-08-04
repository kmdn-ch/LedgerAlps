// LedgerAlps — Chiffre d'affaires par année, par mois ou par client
//
// La courbe du tableau de bord montre une tendance sur six mois. Elle ne répond
// pas aux questions qu'on se pose vraiment : « combien ai-je facturé en 2025 ? »,
// « quel client pèse le plus ? ». Ce tableau répond aux deux, sur la période
// qu'on choisit.
//
// La convention de calcul vient du serveur et s'affiche sous le tableau. Un
// total sans sa définition invite à le comparer à un autre calculé autrement —
// et c'est ainsi qu'on finit par croire à un écart qui n'existe pas.

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CalendarRange, Users, Calendar, Info } from 'lucide-react'
import { revenueApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { formatCHF } from '@/utils'

type GroupBy = 'year' | 'month' | 'contact'

interface RevenueRow {
  key: string
  label: string
  invoiced: number
  paid: number
  count: number
}

interface RevenueResponse {
  group_by: GroupBy
  rows: RevenueRow[]
  total_invoiced: number
  total_paid: number
  basis: string
}

const GROUPS: { key: GroupBy; label: string; icon: typeof Calendar }[] = [
  { key: 'year',    label: 'Par année',  icon: CalendarRange },
  { key: 'month',   label: 'Par mois',   icon: Calendar },
  { key: 'contact', label: 'Par client', icon: Users },
]

// Un libellé « 2026-03 » se lit mal dans un tableau qu'on parcourt.
function prettyLabel(groupBy: GroupBy, label: string): string {
  if (groupBy !== 'month') return label
  const [y, m] = label.split('-')
  const months = ['janvier', 'février', 'mars', 'avril', 'mai', 'juin',
                  'juillet', 'août', 'septembre', 'octobre', 'novembre', 'décembre']
  const idx = Number(m) - 1
  return months[idx] ? `${months[idx]} ${y}` : label
}

export function RevenuePanel() {
  const [groupBy, setGroupBy] = useState<GroupBy>('year')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const revenue = useQuery<RevenueResponse>({
    queryKey: ['revenue', groupBy, from, to],
    queryFn: () => revenueApi.get({
      group_by: groupBy,
      from: from || undefined,
      to: to || undefined,
    }).then(r => r.data),
  })

  const rows = revenue.data?.rows ?? []
  const maxInvoiced = Math.max(1, ...rows.map(r => Math.abs(r.invoiced)))

  return (
    <div className="card p-5">
      <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
        <SectionTitle>Chiffre d'affaires</SectionTitle>
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex gap-1">
            {GROUPS.map(g => (
              <button
                key={g.key}
                onClick={() => setGroupBy(g.key)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded text-xs font-medium transition-all ${
                  groupBy === g.key
                    ? 'bg-alpine-700 text-white'
                    : 'bg-alpine-50 text-alpine-600 hover:bg-alpine-100'
                }`}
              >
                <g.icon size={12} />
                {g.label}
              </button>
            ))}
          </div>
          <input type="date" className="input text-xs py-1" value={from}
                 onChange={e => setFrom(e.target.value)} aria-label="Depuis" />
          <input type="date" className="input text-xs py-1" value={to}
                 onChange={e => setTo(e.target.value)} aria-label="Jusqu'au" />
          {(from || to) && (
            <button onClick={() => { setFrom(''); setTo('') }}
                    className="btn-ghost btn-sm text-xs">
              Toute la période
            </button>
          )}
        </div>
      </div>

      {revenue.isLoading && <LoadingSpinner />}
      {revenue.isError && <ErrorBanner message="Le chiffre d'affaires n'a pas pu être calculé." />}

      {revenue.data && rows.length === 0 && (
        <p className="text-sm text-alpine-500 py-4">
          Aucune facture émise sur cette période.
        </p>
      )}

      {revenue.data && rows.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-200 text-left text-xs uppercase tracking-wider text-alpine-500">
                  <th className="py-2 pr-3 font-medium">
                    {groupBy === 'contact' ? 'Client' : 'Période'}
                  </th>
                  <th className="py-2 pr-3 font-medium text-right">Facturé</th>
                  <th className="py-2 pr-3 font-medium text-right">Encaissé</th>
                  <th className="py-2 pr-3 font-medium text-right">Pièces</th>
                  <th className="py-2 font-medium w-1/4">Part</th>
                </tr>
              </thead>
              <tbody>
                {rows.map(r => (
                  <tr key={r.key} className="border-b border-neutral-100">
                    <td className="py-2 pr-3">{prettyLabel(groupBy, r.label)}</td>
                    <td className="py-2 pr-3 text-right tabular-nums font-medium">
                      {formatCHF(r.invoiced)}
                    </td>
                    <td className="py-2 pr-3 text-right tabular-nums text-alpine-600">
                      {formatCHF(r.paid)}
                    </td>
                    <td className="py-2 pr-3 text-right tabular-nums text-alpine-500">{r.count}</td>
                    <td className="py-2">
                      {/* Barre proportionnelle : lire vingt lignes de chiffres
                          pour repérer laquelle domine est un travail que
                          l'écran peut faire à la place du lecteur. */}
                      <div className="h-1.5 bg-alpine-100 rounded overflow-hidden">
                        <div
                          className="h-full bg-alpine-500"
                          style={{ width: `${Math.max(0, (r.invoiced / maxInvoiced) * 100)}%` }}
                        />
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="font-medium">
                  <td className="py-2 pr-3">Total</td>
                  <td className="py-2 pr-3 text-right tabular-nums">
                    {formatCHF(revenue.data.total_invoiced)}
                  </td>
                  <td className="py-2 pr-3 text-right tabular-nums text-alpine-600">
                    {formatCHF(revenue.data.total_paid)}
                  </td>
                  <td colSpan={2} />
                </tr>
              </tfoot>
            </table>
          </div>

          <p className="mt-3 text-xs text-alpine-500 flex items-start gap-1.5">
            <Info size={12} className="mt-0.5 flex-shrink-0" />
            {revenue.data.basis}
          </p>
        </>
      )}
    </div>
  )
}
