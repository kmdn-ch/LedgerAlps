// LedgerAlps — Plan comptable et balance de vérification
//
// Cette page lisait des champs que le serveur n'envoie pas.
//
// La colonne des numéros lisait `number` là où l'API rend `code` : elle était
// donc VIDE sur les quatre-vingt-un comptes. La balance lisait
// `account_number`, `account_name`, `debit`, `credit` là où l'API rend `code`,
// `name`, `total_debit`, `total_credit` : chaque colonne affichait « — », et le
// total « Équilibrée ✓ » ne pouvait jamais apparaître puisque la ligne TOTAL
// qu'elle cherchait n'existe pas côté serveur — elle se calcule ici.
//
// TypeScript ne pouvait rien signaler : les types décrivaient fidèlement une
// API qui n'existait plus. Le défaut ne se voyait qu'à l'écran.
//
// # À quoi sert cette page
//
// Le plan comptable est le dictionnaire du journal : sans lui, on ne sait pas
// qu'une vente de services se crédite au 3200. La balance de vérification est
// le document de contrôle — la seule vue qui prouve que débit = crédit sur
// l'ensemble des livres, et le premier que demande une fiduciaire. Les deux
// portent sur des écritures COMPTABILISÉES : un brouillon n'y figure pas.
//
// On n'y crée pas de compte. Le plan PME suisse est normalisé, et inventer un
// numéro fausserait les états qui regroupent par tranche.

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { BookOpen, Scale } from 'lucide-react'
import { accountsApi } from '@/api/client'
import { PageHeader, LoadingSpinner, EmptyState, ErrorBanner } from '@/components/ui'
import { formatCHF } from '@/utils'
import type { Account, TrialBalanceLine } from '@/types'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

const TYPE_LABELS: Record<string, Cle> = {
  asset:     'ac.actif',
  liability: 'ac.passif',
  equity:    'ac.capitauxPropres',
  revenue:   'ac.produits',
  expense:   'ac.charges',
}

// L'ordre du plan comptable suisse, et non l'ordre d'arrivée : « Actif, Passif,
// Capitaux propres, Produits, Charges » est celui qu'on lit et qu'on apprend.
const TYPE_ORDER = ['asset', 'liability', 'equity', 'revenue', 'expense']

export function AccountsPage() {
  const t = useT()
  const [view, setView] = useState<'accounts' | 'balance'>('accounts')
  // Quatre-vingt-un comptes dont trois bougent : montrer les soixante-dix-huit
  // autres à zéro noie le contrôle. Le repli reste possible — une balance
  // complète est ce qu'une fiduciaire demande.
  const [tousLesComptes, setTousLesComptes] = useState(false)

  const accounts = useQuery<Account[]>({
    queryKey: ['accounts'],
    queryFn:  () => accountsApi.list().then(r => r.data),
  })

  const balance = useQuery<TrialBalanceLine[]>({
    queryKey: ['trial-balance'],
    queryFn:  () => accountsApi.trialBalance().then(r => r.data),
    enabled:  view === 'balance',
  })

  const grouped = useMemo(() => {
    const g: Record<string, Account[]> = {}
    for (const a of accounts.data ?? []) {
      (g[a.account_type] ??= []).push(a)
    }
    return g
  }, [accounts.data])

  const lignes = balance.data ?? []
  const avecMouvement = lignes.filter(l => l.total_debit !== 0 || l.total_credit !== 0)
  const affichees = tousLesComptes ? lignes : avecMouvement

  // Le total se calcule ici : le serveur rend des lignes, pas un pied de
  // tableau. Dans des livres cohérents, débit et crédit sont égaux — c'est
  // exactement ce que la balance sert à vérifier.
  const totalDebit  = lignes.reduce((s, l) => s + l.total_debit, 0)
  const totalCredit = lignes.reduce((s, l) => s + l.total_credit, 0)
  const ecart = Math.round((totalDebit - totalCredit) * 100) / 100

  return (
    <div>
      <PageHeader
        title={t('nav.planComptable')}
        subtitle={t('ac.sousTitre')}
      />

      <div className="flex gap-1 mb-5 bg-alpine-100 rounded-lg p-1 w-fit">
        {([
          { key: 'accounts', cle: 'ac.ongletPlan' },
          { key: 'balance',  cle: 'ac.ongletBalance' },
        ] as const).map(tab => (
          <button
            key={tab.key}
            onClick={() => setView(tab.key as typeof view)}
            className={`px-4 py-2 rounded-md text-sm font-medium transition-all ${
              view === tab.key
                ? 'bg-white text-alpine-900 shadow-sm'
                : 'text-alpine-600 hover:text-alpine-800'
            }`}
          >
            {t(tab.cle)}
          </button>
        ))}
      </div>

      {/* ── Plan comptable ─────────────────────────────────────────────────── */}
      {view === 'accounts' && (
        <div className="space-y-5">
          {accounts.isLoading && <LoadingSpinner />}
          {accounts.isError && <ErrorBanner message={t('ac.erreurPlan')} />}

          {!accounts.isLoading && (accounts.data?.length ?? 0) > 0 && (
            <p className="text-sm text-alpine-600">
              {t('ac.planAide')}
            </p>
          )}

          {TYPE_ORDER.filter(t => grouped[t]?.length).map(type => (
            <div key={type} className="card">
              <div className="card-header">
                <div className="flex items-center gap-2">
                  <BookOpen size={15} className="text-alpine-500" />
                  <span className="font-semibold text-sm text-alpine-800">
                    {TYPE_LABELS[type] ? t(TYPE_LABELS[type]) : type}
                  </span>
                  <span className="badge badge-draft">{t('ac.nComptes', { n: grouped[type].length })}</span>
                </div>
              </div>
              <div className="overflow-x-auto">
                <table className="table">
                  <thead>
                    <tr>
                      <th style={{ width: '90px' }}>{t('ac.colNumero')}</th>
                      <th>{t('ac.colDesignation')}</th>
                      <th>{t('ac.colUsage')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {grouped[type].map(a => (
                      <tr key={a.id} className={a.is_active ? '' : 'opacity-50'}>
                        <td>
                          <span className="font-mono text-accent-700 font-medium">{a.code}</span>
                        </td>
                        <td className="text-alpine-800">
                          {a.name}
                          {!a.is_active && (
                            <span className="ml-2 text-xs text-alpine-500">{t('ac.desactive')}</span>
                          )}
                        </td>
                        <td className="text-alpine-600 text-xs">{a.description || '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}

          {!accounts.isLoading && !accounts.isError && (accounts.data?.length ?? 0) === 0 && (
            <EmptyState
              title={t('ac.planVide')}
              description={t('ac.planVideAide')}
            />
          )}
        </div>
      )}

      {/* ── Balance de vérification ────────────────────────────────────────── */}
      {view === 'balance' && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-alpine-600 max-w-2xl">
              {t('ac.balanceAide')}
            </p>
            <label className="flex items-center gap-2 text-sm text-alpine-600 shrink-0">
              <input type="checkbox" checked={tousLesComptes}
                     onChange={e => setTousLesComptes(e.target.checked)} />
              {t('ac.montrerSansMouvement')}
            </label>
          </div>

          <div className="card">
            <div className="table-wrapper">
              <table className="table">
                <thead>
                  <tr>
                    <th style={{ width: '90px' }}>{t('ac.colCompte')}</th>
                    <th>{t('ac.colDesignation')}</th>
                    <th className="text-right">{t('ac.colDebit')}</th>
                    <th className="text-right">{t('ac.colCredit')}</th>
                    <th className="text-right">{t('ac.colSolde')}</th>
                  </tr>
                </thead>
                <tbody>
                  {balance.isLoading && (
                    <tr><td colSpan={5}><LoadingSpinner /></td></tr>
                  )}
                  {balance.isError && (
                    <tr><td colSpan={5}>
                      <ErrorBanner message={t('ac.erreurBalance')} />
                    </td></tr>
                  )}
                  {affichees.map(row => (
                    <tr key={row.id}>
                      <td>
                        <span className="font-mono text-accent-700 font-medium">{row.code}</span>
                      </td>
                      <td className="text-alpine-800">{row.name}</td>
                      <td className="text-right font-mono tabular-nums text-alpine-700">
                        {row.total_debit > 0 ? formatCHF(row.total_debit) : '—'}
                      </td>
                      <td className="text-right font-mono tabular-nums text-alpine-700">
                        {row.total_credit > 0 ? formatCHF(row.total_credit) : '—'}
                      </td>
                      <td className={`text-right font-mono tabular-nums font-medium ${
                        row.balance < 0 ? 'text-danger-700' : 'text-alpine-900'
                      }`}>
                        {formatCHF(row.balance)}
                      </td>
                    </tr>
                  ))}
                  {!balance.isLoading && !balance.isError && affichees.length > 0 && (
                    <tr className="bg-alpine-900 text-white font-semibold">
                      <td className="font-mono">{t('ac.total')}</td>
                      <td>{t('ac.ongletBalance')}</td>
                      <td className="text-right font-mono tabular-nums">{formatCHF(totalDebit)}</td>
                      <td className="text-right font-mono tabular-nums">{formatCHF(totalCredit)}</td>
                      <td className="text-right font-mono tabular-nums">
                        {ecart === 0
                          ? <span className="text-success-500">{t('ac.equilibree')}</span>
                          : formatCHF(ecart)}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            {!balance.isLoading && !balance.isError && avecMouvement.length === 0 && (
              <EmptyState
                title={t('ac.aucuneComptabilisee')}
                description={t('ac.aucuneComptabiliseeAide')}
              />
            )}
          </div>

          {/* Un écart à la balance est un défaut d'intégrité, pas une erreur de
              saisie : le serveur refuse toute écriture déséquilibrée. S'il
              apparaît, il faut le dire clairement plutôt que d'afficher un
              nombre au milieu d'un tableau. */}
          {!balance.isLoading && affichees.length > 0 && ecart !== 0 && (
            <div className="rounded-md border border-danger-500 bg-danger-500/5 px-4 py-3 text-sm">
              <p className="font-medium flex items-center gap-1.5">
                <Scale size={15} /> {t('ac.pasEquilibree', { ecart: formatCHF(ecart) })}
              </p>
              <p className="text-alpine-700 mt-1">
                {t('ac.pasEquilibreeAide')}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
