// LedgerAlps — Tableau de bord

import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  FileText, Users, TrendingUp, AlertCircle,
  ArrowRight, Plus,
} from 'lucide-react'
import {
  AreaChart, Area, XAxis, YAxis, Tooltip,
  ResponsiveContainer, CartesianGrid,
} from 'recharts'
import { invoicesApi, statsApi } from '@/api/client'
import { PageHeader, StatCard, StatusBadge, LoadingSpinner } from '@/components/ui'
import { formatCHF, formatDate } from '@/utils'
import type { Invoice } from '@/types'

// Format "YYYY-MM" → short French month label
function shortMonth(yyyyMM: string): string {
  const [y, m] = yyyyMM.split('-').map(Number)
  const d = new Date(y, m - 1, 1)
  return d.toLocaleDateString('fr-CH', { month: 'short' })
    .replace('.', '')
    .replace(/^./, s => s.toUpperCase())
}

export function DashboardPage() {
  const { data: invoices = [], isLoading: invLoading } = useQuery<Invoice[]>({
    queryKey: ['invoices', 'all'],
    queryFn:  () => invoicesApi.list().then(r => r.data.items as Invoice[]),
  })

  const { data: stats } = useQuery({
    queryKey: ['stats'],
    queryFn:  () => statsApi.get().then(r => r.data),
  })

  const totalDue = invoices
    .filter(i => i.status === 'sent' || i.status === 'overdue')
    .reduce((s, i) => s + i.total_amount - i.amount_paid, 0)

  const totalOverdue = invoices
    .filter(i => i.status === 'overdue')
    .reduce((s, i) => s + i.total_amount - i.amount_paid, 0)

  const recentInvoices = [...invoices]
    .sort((a, b) => b.issue_date.localeCompare(a.issue_date))
    .slice(0, 5)

  // Real chart data from stats API
  const chartData = (stats?.monthly_revenue ?? []).map((p: { month: string; total: number; paid: number }) => ({
    month: shortMonth(p.month),
    ca:    p.total,
    paid:  p.paid,
  }))

  const hasChartData = chartData.some((p: { ca: number }) => p.ca > 0)

  return (
    <div>
      <PageHeader
        title="Tableau de bord"
        subtitle={`Aujourd'hui — ${formatDate(new Date().toISOString())}`}
        actions={
          <Link to="/invoices/new" className="btn-primary">
            <Plus size={15} />
            Nouvelle facture
          </Link>
        }
      />

      {/* Stats */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <StatCard
          label="Créances ouvertes"
          value={formatCHF(totalDue)}
          icon={<TrendingUp size={18} />}
          accent={totalDue > 0}
        />
        <StatCard
          label="En retard"
          value={formatCHF(totalOverdue)}
          icon={<AlertCircle size={18} />}
        />
        <StatCard
          label="Factures ce mois"
          value={String(invoices.filter(i =>
            i.issue_date.startsWith(new Date().toISOString().slice(0, 7))
          ).length)}
          icon={<FileText size={18} />}
        />
        <StatCard
          label="Clients actifs"
          value={String(stats?.contacts?.customers ?? '—')}
          icon={<Users size={18} />}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Graphique CA réel */}
        <div className="lg:col-span-2 card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">
              Chiffre d'affaires — 6 mois
            </h2>
          </div>
          <div className="card-body">
            {hasChartData ? (
              <ResponsiveContainer width="100%" height={220}>
                <AreaChart data={chartData} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
                  <defs>
                    <linearGradient id="gradCA" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%"  stopColor="#334e68" stopOpacity={0.15} />
                      <stop offset="95%" stopColor="#334e68" stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="gradPaid" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%"  stopColor="#f97316" stopOpacity={0.15} />
                      <stop offset="95%" stopColor="#f97316" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="#e2e8f0" />
                  <XAxis dataKey="month" tick={{ fontSize: 11, fill: '#627d98' }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fontSize: 11, fill: '#627d98' }} axisLine={false} tickLine={false}
                         tickFormatter={v => v >= 1000 ? `${(v/1000).toFixed(0)}k` : String(v)} />
                  <Tooltip
                    contentStyle={{ fontSize: 12, borderRadius: 8, border: '1px solid #d9e2ec' }}
                    formatter={(v: number, name: string) => [
                      formatCHF(v), name === 'ca' ? 'Facturé' : 'Encaissé'
                    ]}
                  />
                  <Area type="monotone" dataKey="ca"   stroke="#334e68" strokeWidth={2}
                        fill="url(#gradCA)"   dot={false} />
                  <Area type="monotone" dataKey="paid" stroke="#f97316" strokeWidth={2}
                        fill="url(#gradPaid)" dot={false} />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <div className="h-[220px] flex flex-col items-center justify-center text-alpine-400">
                <TrendingUp size={32} className="mb-2 opacity-30" />
                <p className="text-sm">Aucune donnée de facturation sur les 6 derniers mois.</p>
                <p className="text-xs mt-1">Le graphique s'affichera dès la première facture émise.</p>
              </div>
            )}
          </div>
        </div>

        {/* Factures récentes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Factures récentes</h2>
            <Link to="/invoices" className="text-xs text-accent-600 hover:text-accent-700
                                            flex items-center gap-1">
              Voir tout <ArrowRight size={12} />
            </Link>
          </div>
          <div className="divide-y divide-alpine-100">
            {invLoading && <LoadingSpinner />}
            {recentInvoices.map(inv => (
              <Link
                key={inv.id}
                to={`/invoices/${inv.id}`}
                className="flex items-center justify-between px-4 py-3
                           hover:bg-alpine-50 transition-colors"
              >
                <div className="min-w-0">
                  <div className="text-sm font-medium text-alpine-800 truncate">
                    {inv.invoice_number}
                  </div>
                  <div className="text-xs text-alpine-400">{formatDate(inv.issue_date)}</div>
                </div>
                <div className="text-right ml-2 flex-shrink-0">
                  <div className="text-sm font-medium font-mono tabular-nums text-alpine-900">
                    {formatCHF(inv.total_amount)}
                  </div>
                  <StatusBadge status={inv.status} />
                </div>
              </Link>
            ))}
            {!invLoading && recentInvoices.length === 0 && (
              <p className="text-sm text-alpine-400 text-center py-8">Aucune facture</p>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
