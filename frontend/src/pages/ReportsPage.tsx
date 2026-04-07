// LedgerAlps — Rapports et exports

import { useState } from 'react'
import {
  Download, Archive, Upload, FileSpreadsheet,
  BookOpen, BarChart3, Calendar,
} from 'lucide-react'
import { exportsApi, downloadBlob } from '@/api/client'
import { PageHeader, SectionTitle, ErrorBanner } from '@/components/ui'

export function ReportsPage() {
  const [startDate, setStartDate] = useState(() =>
    new Date(new Date().getFullYear(), 0, 1).toISOString().slice(0, 10)
  )
  const [endDate, setEndDate] = useState(() =>
    new Date().toISOString().slice(0, 10)
  )
  const [loading, setLoading] = useState<string | null>(null)
  const [error,   setError]   = useState('')

  const run = async (key: string, fn: () => Promise<{ data: Blob }>, filename: string) => {
    setLoading(key)
    setError('')
    try {
      const resp = await fn()
      downloadBlob(resp.data, filename)
    } catch {
      setError(`Erreur lors de l'export "${key}".`)
    } finally {
      setLoading(null)
    }
  }

  const fmt = (d: string) => d.replace(/-/g, '')

  return (
    <div>
      <PageHeader
        title="Rapports"
        subtitle="Exports comptables et archivage légal"
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

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">

        {/* Journal */}
        <ExportCard
          icon={<BookOpen size={20} />}
          title="Journal général"
          description="Toutes les écritures validées sur la période. Format CSV (UTF-8 BOM, compatible Excel CH)."
          badge="CO art. 957a"
          loading={loading === 'journal'}
          onClick={() => run(
            'journal',
            () => exportsApi.journal(startDate, endDate),
            `journal_${fmt(startDate)}_${fmt(endDate)}.csv`
          )}
        />

        {/* Grand Livre */}
        <ExportCard
          icon={<FileSpreadsheet size={20} />}
          title="Grand Livre"
          description="Mouvements par compte avec soldes cumulés. Outil de contrôle fiduciaire."
          badge="CO art. 957"
          loading={loading === 'ledger'}
          onClick={() => run(
            'ledger',
            () => exportsApi.generalLedger(startDate, endDate),
            `grand_livre_${fmt(startDate)}_${fmt(endDate)}.csv`
          )}
        />

        {/* Balance */}
        <ExportCard
          icon={<BarChart3 size={20} />}
          title="Balance de vérification"
          description="Synthèse débit/crédit/solde par compte. Vérifie l'équilibre de la comptabilité."
          badge="Contrôle"
          loading={loading === 'balance'}
          onClick={() => run(
            'balance',
            () => exportsApi.trialBalance(endDate),
            `balance_${fmt(endDate)}.csv`
          )}
        />

      </div>

      {/* Archivage légal */}
      <SectionTitle>Archivage légal — CO art. 958f (10 ans)</SectionTitle>
      <div className="card mb-8 border-alpine-300">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-alpine-800 flex items-center
                            justify-center flex-shrink-0">
              <Archive size={18} className="text-white" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">Archive annuelle ZIP</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Génère une archive ZIP contenant journal, Grand Livre et balance de vérification,
                accompagnée d'un fichier <code className="font-mono text-xs bg-alpine-100 px-1 rounded">manifest.json</code> avec
                les hash SHA-256 de chaque fichier. Conforme CO art. 958f — conservation 10 ans.
              </p>
              <div className="flex items-center gap-3 flex-wrap">
                <select className="select w-40">
                  <option value="">Sélectionner un exercice…</option>
                  <option value="2025">Exercice 2025</option>
                  <option value="2024">Exercice 2024</option>
                </select>
                <button
                  className="btn-primary"
                  disabled={loading === 'archive'}
                  onClick={() => {
                    // TODO: passer le fiscal_year_id réel
                    setError('Sélectionnez d\'abord un exercice comptable clôturé.')
                  }}
                >
                  <Archive size={15} />
                  {loading === 'archive' ? 'Génération…' : 'Créer l\'archive'}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Import camt.053 */}
      <SectionTitle>Import bancaire — ISO 20022</SectionTitle>
      <div className="card">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-alpine-100 flex items-center
                            justify-center flex-shrink-0">
              <Upload size={18} className="text-alpine-600" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">Relevé bancaire camt.053</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Importez le fichier XML de votre banque pour réconcilier automatiquement
                les paiements reçus avec les factures ouvertes (correspondance QRR/RF ou montant).
              </p>
              <label className="btn-secondary cursor-pointer">
                <Upload size={14} />
                Importer un fichier camt.053 (.xml)
                <input
                  type="file"
                  accept=".xml"
                  className="hidden"
                  onChange={e => {
                    if (e.target.files?.[0]) {
                      // TODO: envoyer au backend /api/v1/iso20022/camt053/parse
                      alert(`Fichier sélectionné : ${e.target.files[0].name}\nParsing disponible en phase finale.`)
                    }
                  }}
                />
              </label>
              <p className="text-xs text-alpine-400 mt-2">
                Formats supportés : camt.053.001.02, .06, .08 (PostFinance, UBS, Raiffeisen, Credit Suisse)
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Composant carte export ───────────────────────────────────────────────────

interface ExportCardProps {
  icon:        React.ReactNode
  title:       string
  description: string
  badge:       string
  loading:     boolean
  onClick:     () => void
}

function ExportCard({ icon, title, description, badge, loading, onClick }: ExportCardProps) {
  return (
    <div className="card hover:shadow-md transition-shadow">
      <div className="card-body flex flex-col gap-3">
        <div className="flex items-start justify-between">
          <div className="w-9 h-9 rounded-lg bg-alpine-100 flex items-center
                          justify-center text-alpine-700">
            {icon}
          </div>
          <span className="badge badge-draft text-[10px]">{badge}</span>
        </div>
        <div>
          <h3 className="font-semibold text-alpine-900 text-sm">{title}</h3>
          <p className="text-xs text-alpine-500 mt-1 leading-relaxed">{description}</p>
        </div>
        <button
          onClick={onClick}
          disabled={loading}
          className="btn-secondary btn-sm mt-auto"
        >
          <Download size={14} />
          {loading ? 'Export…' : 'Télécharger CSV'}
        </button>
      </div>
    </div>
  )
}
