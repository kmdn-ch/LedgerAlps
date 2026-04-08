// LedgerAlps — Rapports et exports

import { useState } from 'react'
import {
  Download, Archive, Upload, FileSpreadsheet,
  BookOpen, BarChart3, Calendar, AlertCircle,
} from 'lucide-react'
import { isoApi, downloadBlob } from '@/api/client'
import { PageHeader, SectionTitle, ErrorBanner } from '@/components/ui'

export function ReportsPage() {
  const [startDate, setStartDate] = useState(() =>
    new Date(new Date().getFullYear(), 0, 1).toISOString().slice(0, 10)
  )
  const [endDate, setEndDate] = useState(() =>
    new Date().toISOString().slice(0, 10)
  )
  const [loading,  setLoading]  = useState<string | null>(null)
  const [error,    setError]    = useState('')
  const [camtResult, setCamtResult] = useState<null | { count: number; entries: unknown[] }>(null)

  const fmt = (d: string) => d.replace(/-/g, '')

  // ── camt.053 import ────────────────────────────────────────────────────────
  const handleCamtUpload = async (file: File) => {
    setLoading('camt')
    setError('')
    setCamtResult(null)
    try {
      const res = await isoApi.importCamt053(file)
      setCamtResult(res.data)
    } catch {
      setError('Erreur lors de l\'import du relevé bancaire.')
    } finally {
      setLoading(null)
    }
  }

  return (
    <div>
      <PageHeader
        title="Rapports"
        subtitle="Exports comptables et import bancaire ISO 20022"
      />

      {error && <ErrorBanner message={error} />}

      {/* Sélection de période */}
      <div className="card mb-6">
        <div className="card-body flex items-center gap-4 flex-wrap">
          <Calendar size={16} className="text-alpine-400" />
          <div className="flex items-center gap-2">
            <label className="text-sm text-alpine-600">Du</label>
            <input type="date" className="input w-40"
              value={startDate} onChange={e => setStartDate(e.target.value)} />
          </div>
          <div className="flex items-center gap-2">
            <label className="text-sm text-alpine-600">Au</label>
            <input type="date" className="input w-40"
              value={endDate} onChange={e => setEndDate(e.target.value)} />
          </div>
          <p className="text-xs text-alpine-400">
            Sélectionnez la période avant de télécharger les rapports.
          </p>
        </div>
      </div>

      {/* Exports comptables — à venir */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">
        <ExportCard
          icon={<BookOpen size={20} />}
          title="Journal général"
          description="Export CSV des écritures validées (CO art. 957a)."
          badge="Bientôt disponible"
          loading={false}
          disabled
          onClick={() => {}}
        />
        <ExportCard
          icon={<FileSpreadsheet size={20} />}
          title="Grand Livre"
          description="Mouvements par compte avec soldes cumulés."
          badge="Bientôt disponible"
          loading={false}
          disabled
          onClick={() => {}}
        />
        <ExportCard
          icon={<BarChart3 size={20} />}
          title="Balance de vérification"
          description="Consultez la balance dans l'onglet Comptes."
          badge="Disponible"
          loading={false}
          onClick={() => { window.location.href = '/accounts' }}
        />
      </div>

      {/* Import camt.053 */}
      <SectionTitle>Import bancaire — ISO 20022 camt.053</SectionTitle>
      <div className="card mb-8">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-accent-100 flex items-center
                            justify-center flex-shrink-0">
              <Upload size={18} className="text-accent-600" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">Relevé bancaire camt.053</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Importez le fichier XML de votre banque (PostFinance, UBS, Raiffeisen…)
                pour réconcilier les paiements reçus avec les factures ouvertes.
              </p>
              <label className={`btn-secondary cursor-pointer ${loading === 'camt' ? 'opacity-50 pointer-events-none' : ''}`}>
                <Upload size={14} />
                {loading === 'camt' ? 'Import en cours…' : 'Importer un fichier camt.053 (.xml)'}
                <input
                  type="file"
                  accept=".xml"
                  className="hidden"
                  onChange={e => { if (e.target.files?.[0]) handleCamtUpload(e.target.files[0]) }}
                />
              </label>
              <p className="text-xs text-alpine-400 mt-2">
                Formats : camt.053.001.06 / .08 (SIX Interbank Clearing)
              </p>
            </div>
          </div>

          {/* camt result */}
          {camtResult && (
            <div className="mt-4 p-4 bg-success-50 border border-success-200 rounded-lg">
              <p className="text-sm font-medium text-success-800 mb-2">
                {camtResult.count} transaction{camtResult.count !== 1 ? 's' : ''} importée{camtResult.count !== 1 ? 's' : ''}
              </p>
              <div className="space-y-1 max-h-48 overflow-y-auto">
                {(camtResult.entries as Array<{
                  booking_date: string; amount: number; currency: string
                  is_credit: boolean; counterpart_name?: string; unstructured?: string
                }>).map((e, i) => (
                  <div key={i} className="flex items-center justify-between text-xs text-success-700 bg-white/60 rounded px-2 py-1">
                    <span>{e.booking_date}</span>
                    <span className={e.is_credit ? 'text-success-700 font-medium' : 'text-danger-600 font-medium'}>
                      {e.is_credit ? '+' : '-'}{e.amount.toFixed(2)} {e.currency}
                    </span>
                    <span className="truncate max-w-[200px] text-alpine-500">
                      {e.counterpart_name || e.unstructured || '—'}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>

      {/* pain.001 export */}
      <SectionTitle>Paiements — ISO 20022 pain.001</SectionTitle>
      <div className="card mb-8">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-alpine-100 flex items-center
                            justify-center flex-shrink-0">
              <Download size={18} className="text-alpine-600" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">Fichier de paiements pain.001</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Générez un fichier XML pain.001.001.09 pour initier des virements en masse
                depuis votre e-banking (compatible UBS, PostFinance, Raiffeisen, CS/UBS).
              </p>
              <div className="flex items-center gap-2 p-3 bg-alpine-50 rounded-lg text-sm text-alpine-600">
                <AlertCircle size={14} className="text-alpine-400 flex-shrink-0" />
                Utilisez le journal comptable pour sélectionner les écritures à payer,
                puis exportez via l'API <code className="font-mono text-xs">POST /api/v1/payments/export</code>.
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Archivage légal */}
      <SectionTitle>Archivage légal — CO art. 958f (10 ans)</SectionTitle>
      <div className="card border-alpine-200">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-alpine-800 flex items-center
                            justify-center flex-shrink-0">
              <Archive size={18} className="text-white" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">Archive annuelle ZIP</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Archive ZIP contenant journal, Grand Livre et balance de vérification
                avec manifest SHA-256. Conservation légale 10 ans (CO art. 958f).
              </p>
              <div className="flex items-center gap-2 p-3 bg-amber-50 border border-amber-200 rounded-lg text-sm text-amber-700">
                <AlertCircle size={14} className="flex-shrink-0" />
                Fonctionnalité prévue dans une prochaine version.
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── ExportCard component ────────────────────────────────────────────────────

function ExportCard({
  icon, title, description, badge, loading, disabled, onClick,
}: {
  icon: React.ReactNode
  title: string
  description: string
  badge: string
  loading: boolean
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <div className={`card ${disabled ? 'opacity-60' : ''}`}>
      <div className="card-body">
        <div className="flex items-start justify-between mb-3">
          <div className="w-9 h-9 rounded-lg bg-accent-100 flex items-center justify-center text-accent-600">
            {icon}
          </div>
          <span className="text-xs font-medium text-alpine-500 bg-alpine-100 px-2 py-0.5 rounded-full">
            {badge}
          </span>
        </div>
        <h3 className="font-semibold text-alpine-900 mb-1">{title}</h3>
        <p className="text-xs text-alpine-500 mb-4 min-h-[2.5rem]">{description}</p>
        <button
          className="btn-secondary w-full"
          disabled={loading || disabled}
          onClick={onClick}
        >
          <Download size={14} />
          {loading ? 'Génération…' : 'Télécharger'}
        </button>
      </div>
    </div>
  )
}
