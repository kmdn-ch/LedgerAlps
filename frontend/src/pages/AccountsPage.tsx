// LedgerAlps — Plan comptable et balance de vérification

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Download, BookOpen } from 'lucide-react'
import { accountsApi, exportsApi, downloadBlob } from '@/api/client'
import { PageHeader, LoadingSpinner, EmptyState, SectionTitle } from '@/components/ui'
import { formatCHF } from '@/utils'
import type { Account, AccountBalance } from '@/types'

const TYPE_LABELS: Record<string, string> = {
  asset:     'Actif',
  liability: 'Passif',
  equity:    'Capitaux propres',
  revenue:   'Produits',
  expense:   'Charges',
}

const TYPE_COLOR: Record<string, string> = {
  asset:     'text-blue-700 bg-blue-50',
  liability: 'text-purple-700 bg-purple-50',
  equity:    'text-emerald-700 bg-emerald-50',
  revenue:   'text-green-700 bg-green-50',
  expense:   'text-red-700 bg-red-50',
}

export function AccountsPage() {
  const [view, setView] = useState<'accounts' | 'balance'>('accounts')

  const { data: accounts = [], isLoading: accLoading } = useQuery<Account[]>({
    queryKey: ['accounts'],
    queryFn:  () => accountsApi.list().then(r => r.data),
    enabled:  view === 'accounts',
  })

  const { data: balance = [], isLoading: balLoading } = useQuery<AccountBalance[]>({
    queryKey: ['trial-balance'],
    queryFn:  () => accountsApi.trialBalance().then(r => r.data),
    enabled:  view === 'balance',
  })

  const handleExportBalance = async () => {
    const resp = await exportsApi.trialBalance()
    downloadBlob(resp.data, `balance_${new Date().toISOString().slice(0,10)}.csv`)
  }

  // Grouper les comptes par type
  const grouped = accounts.reduce<Record<string, Account[]>>((acc, a) => {
    const t = a.account_type
    if (!acc[t]) acc[t] = []
    acc[t].push(a)
    return acc
  }, {})

  const totalRow = balance.find(b => b.account_number === 'TOTAL')
  const balanceRows = balance.filter(b => b.account_number !== 'TOTAL')

  return (
    <div>
      <PageHeader
        title="Plan comptable"
        subtitle="PME suisse — CO art. 957"
        actions={
          <div className="flex gap-2">
            {view === 'balance' && (
              <button onClick={handleExportBalance} className="btn-secondary">
                <Download size={15} /> Export CSV
              </button>
            )}
          </div>
        }
      />

      {/* Tabs */}
      <div className="flex gap-1 mb-5 bg-alpine-100 rounded-lg p-1 w-fit">
        {[
          { key: 'accounts', label: 'Plan comptable' },
          { key: 'balance',  label: 'Balance de vérification' },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setView(tab.key as typeof view)}
            className={`px-4 py-2 rounded-md text-sm font-medium transition-all ${
              view === tab.key
                ? 'bg-white text-alpine-900 shadow-sm'
                : 'text-alpine-600 hover:text-alpine-800'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Plan comptable */}
      {view === 'accounts' && (
        <div className="space-y-5">
          {accLoading && <LoadingSpinner />}
          {Object.entries(grouped).map(([type, accs]) => (
            <div key={type} className="card">
              <div className="card-header">
                <div className="flex items-center gap-2">
                  <BookOpen size={15} className="text-alpine-500" />
                  <span className="font-semibold text-sm text-alpine-800">
                    {TYPE_LABELS[type] ?? type}
                  </span>
                  <span className="badge badge-draft">{accs.length} comptes</span>
                </div>
              </div>
              <div className="overflow-x-auto">
                <table className="table">
                  <thead>
                    <tr>
                      <th style={{ width: '90px' }}>N°</th>
                      <th>Désignation</th>
                      <th style={{ width: '120px' }}>Type</th>
                    </tr>
                  </thead>
                  <tbody>
                    {accs.map(a => (
                      <tr key={a.id}>
                        <td>
                          <span className="font-mono text-accent-700 font-medium">
                            {a.number}
                          </span>
                        </td>
                        <td className="text-alpine-800">{a.name}</td>
                        <td>
                          <span className={`badge text-xs ${TYPE_COLOR[a.account_type] ?? 'badge-draft'}`}>
                            {TYPE_LABELS[a.account_type] ?? a.account_type}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
          {!accLoading && accounts.length === 0 && (
            <EmptyState
              title="Plan comptable vide"
              description="Exécutez 'make seed' pour charger le plan comptable PME suisse."
            />
          )}
        </div>
      )}

      {/* Balance de vérification */}
      {view === 'balance' && (
        <div className="card">
          <div className="table-wrapper">
            <table className="table">
              <thead>
                <tr>
                  <th style={{ width: '90px' }}>Compte</th>
                  <th>Désignation</th>
                  <th className="text-right">Débit CHF</th>
                  <th className="text-right">Crédit CHF</th>
                  <th className="text-right">Solde CHF</th>
                </tr>
              </thead>
              <tbody>
                {balLoading && (
                  <tr><td colSpan={5}><LoadingSpinner /></td></tr>
                )}
                {balanceRows.map(row => (
                  <tr key={row.account_number}>
                    <td>
                      <span className="font-mono text-accent-700 font-medium">
                        {row.account_number}
                      </span>
                    </td>
                    <td className="text-alpine-800">{row.account_name}</td>
                    <td className="text-right font-mono tabular-nums text-alpine-700">
                      {parseFloat(row.debit) > 0 ? formatCHF(row.debit) : '—'}
                    </td>
                    <td className="text-right font-mono tabular-nums text-alpine-700">
                      {parseFloat(row.credit) > 0 ? formatCHF(row.credit) : '—'}
                    </td>
                    <td className={`text-right font-mono tabular-nums font-medium ${
                      parseFloat(row.balance) < 0 ? 'text-danger-600' : 'text-alpine-900'
                    }`}>
                      {formatCHF(row.balance)}
                    </td>
                  </tr>
                ))}
                {totalRow && (
                  <tr className="bg-alpine-900 text-white font-semibold">
                    <td className="font-mono">TOTAL</td>
                    <td>Balance de vérification</td>
                    <td className="text-right font-mono">{formatCHF(totalRow.debit)}</td>
                    <td className="text-right font-mono">{formatCHF(totalRow.credit)}</td>
                    <td className="text-right font-mono">
                      {parseFloat(totalRow.balance) === 0
                        ? <span className="text-success-400">Équilibrée ✓</span>
                        : formatCHF(totalRow.balance)
                      }
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          {!balLoading && balanceRows.length === 0 && (
            <EmptyState title="Aucune écriture" description="Le journal est vide." />
          )}
        </div>
      )}
    </div>
  )
}
