// LedgerAlps — Rapports et exports
//
// Trois cartes de cet écran portaient une pastille « Bientôt disponible », un
// bouton désactivé et un gestionnaire vide ; l'archive légale annonçait une
// « fonctionnalité prévue dans une prochaine version » alors que la route
// existait et fonctionnait. Ce n'étaient pas des fonctions en panne : rien ne
// les avait jamais reliées à quoi que ce soit.
//
// Une maquette laissée dans une application livrée est pire qu'une absence :
// elle promet, on planifie autour, et le manque se découvre au moment où l'on
// en a besoin.

import { useState } from 'react'
import {
  Archive, Upload, FileSpreadsheet, BookOpen, BarChart3, Calendar, Download, Lock,
} from 'lucide-react'
import { isoApi, exportApi, accountingExportApi } from '@/api/client'
import { PaymentRunPanel } from '@/components/payments/PaymentRunPanel'
import { PageHeader, SectionTitle, ErrorBanner } from '@/components/ui'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { refusalMessage } from '@/utils/refusal'

export function ReportsPage() {
  const peutEcrire = useCanWrite()
  const [startDate, setStartDate] = useState(() =>
    new Date(new Date().getFullYear(), 0, 1).toISOString().slice(0, 10)
  )
  const [endDate, setEndDate] = useState(() => new Date().toISOString().slice(0, 10))
  const [loading, setLoading] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [camtResult, setCamtResult] = useState<null | { count: number; entries: unknown[] }>(null)

  // Un téléchargement passe par un Blob et non par window.location : la requête
  // porte le jeton d'authentification, qu'une navigation directe n'emporterait
  // pas — elle recevrait un 401 et afficherait une page blanche.
  const download = async (
    key: string, fetcher: () => Promise<{ data: BlobPart }>, filename: string,
  ) => {
    setLoading(key)
    setError('')
    try {
      const res = await fetcher()
      const url = URL.createObjectURL(new Blob([res.data]))
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch (e) {
      setError(refusalMessage(e, "Le fichier n'a pas pu être produit."))
    } finally {
      setLoading(null)
    }
  }

  const handleCamtUpload = async (file: File) => {
    setLoading('camt')
    setError('')
    setCamtResult(null)
    try {
      const res = await isoApi.importCamt053(file)
      setCamtResult(res.data)
    } catch (e) {
      setError(refusalMessage(e, "Le relevé bancaire n'a pas pu être lu."))
    } finally {
      setLoading(null)
    }
  }

  const periode = `${startDate}_${endDate}`

  return (
    <div>
      <PageHeader
        title="Rapports"
        subtitle="Exports comptables et import bancaire ISO 20022"
      />

      {error && <div className="mb-4"><ErrorBanner message={error} /></div>}

      {/* Période */}
      <div className="card mb-6">
        <div className="card-body flex items-center gap-4 flex-wrap">
          <Calendar size={16} className="text-alpine-400" />
          <div className="flex items-center gap-2">
            <label className="text-sm text-alpine-600" htmlFor="du">Du</label>
            <input id="du" type="date" className="input w-40"
                   value={startDate} onChange={e => setStartDate(e.target.value)} />
          </div>
          <div className="flex items-center gap-2">
            <label className="text-sm text-alpine-600" htmlFor="au">Au</label>
            <input id="au" type="date" className="input w-40"
                   value={endDate} onChange={e => setEndDate(e.target.value)} />
          </div>
          <p className="text-xs text-alpine-400">
            La période s&rsquo;applique aux trois exports ci-dessous.
          </p>
        </div>
      </div>

      <SectionTitle>Documents comptables</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        Fichiers CSV séparés par des point-virgules, lisibles directement dans Excel.
        Seules les écritures <strong>comptabilisées</strong> y figurent : un brouillon
        n&rsquo;est scellé par rien et pourrait encore changer.
      </p>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">
        <ExportCard
          icon={<BookOpen size={20} />}
          title="Journal général"
          description="Toutes les écritures dans l'ordre chronologique, avec leur auteur et leur empreinte (CO art. 957a)."
          loading={loading === 'journal'}
          onClick={() => download('journal',
            () => accountingExportApi.journal(startDate, endDate),
            `journal_${periode}.csv`)}
        />
        <ExportCard
          icon={<FileSpreadsheet size={20} />}
          title="Grand livre"
          description="Les mêmes mouvements rangés par compte, avec le solde cumulé après chaque ligne."
          loading={loading === 'ledger'}
          onClick={() => download('ledger',
            () => accountingExportApi.ledger(startDate, endDate),
            `grand-livre_${periode}.csv`)}
        />
        <ExportCard
          icon={<BarChart3 size={20} />}
          title="Balance de vérification"
          description="Totaux par compte et contrôle d'équilibre. Le premier document que demande une fiduciaire."
          loading={loading === 'balance'}
          onClick={() => download('balance',
            () => accountingExportApi.trialBalance(startDate, endDate),
            `balance_${periode}.csv`)}
        />
      </div>

      {/* Import camt.053 — déposer un fichier est une écriture.
          Les EXPORTS ci-dessus restent ouverts à tous : consulter et sortir ses
          propres livres n'est pas une modification, et c'est précisément ce
          qu'on attend d'un accès de fiduciaire. */}
      {peutEcrire && (
      <>
      <SectionTitle>Import bancaire — ISO 20022 camt.053</SectionTitle>
      <div className="card mb-8">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-alpine-100 flex items-center
                            justify-center flex-shrink-0">
              <Upload size={18} className="text-alpine-600" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">Relevé bancaire camt.053</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Déposez le fichier XML téléchargé depuis votre e-banking : LedgerAlps y
                reconnaît vos encaissements et propose les rapprochements.
              </p>
              <label className="btn-secondary btn-sm inline-flex items-center gap-1.5 cursor-pointer">
                <Upload size={14} />
                {loading === 'camt' ? 'Lecture…' : 'Choisir un fichier XML'}
                <input type="file" accept=".xml,text/xml,application/xml" className="hidden"
                       onChange={e => {
                         const f = e.target.files?.[0]
                         if (f) handleCamtUpload(f)
                         e.target.value = ''
                       }} />
              </label>
            </div>
          </div>

          {camtResult && (
            <div className="mt-4 p-4 bg-success-100 border border-success-100 rounded-lg">
              <p className="text-sm font-medium text-success-700 mb-2">
                {camtResult.count} transaction{camtResult.count !== 1 ? 's' : ''} lue
                {camtResult.count !== 1 ? 's' : ''} — rapprochez-les dans
                Paramètres&nbsp;→&nbsp;Banque
              </p>
              <div className="space-y-1 max-h-48 overflow-y-auto">
                {(camtResult.entries as Array<{
                  booking_date: string; amount: number; currency: string
                  is_credit: boolean; counterpart_name?: string; unstructured?: string
                }>).map((e, i) => (
                  <div key={i} className="flex items-center justify-between text-xs
                                          text-success-700 bg-white/60 rounded px-2 py-1">
                    <span>{e.booking_date}</span>
                    <span className={e.is_credit
                      ? 'text-success-700 font-medium' : 'text-danger-700 font-medium'}>
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

      {/* pain.001 — produire un ordre de virement engage la trésorerie. */}
      <div className="card card-pad mb-8">
        <PaymentRunPanel />
      </div>
      </>
      )}

      {!peutEcrire && (
        <div className="card mb-8">
          <div className="card-body flex items-start gap-3">
            <Lock size={18} className="text-alpine-500 flex-shrink-0 mt-0.5" />
            <p className="text-sm text-alpine-600">
              {RAISON_LECTURE_SEULE} L&rsquo;import d&rsquo;un relevé bancaire et
              la production d&rsquo;un ordre de paiement demandent un compte
              autorisé à écrire. <strong>Les exports et l&rsquo;archive légale
              ci-dessous vous restent ouverts.</strong>
            </p>
          </div>
        </div>
      )}

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
              <h3 className="font-semibold text-alpine-900 mb-1">Archive légale ZIP</h3>
              <p className="text-sm text-alpine-600 mb-3">
                Toute la comptabilité en JSON <em>et</em> en CSV — écritures, factures,
                contacts, exercices — avec un manifeste SHA-256. Le format est ce qui rend
                la sortie réelle : une archive qu&rsquo;on ne peut pas relire ailleurs
                n&rsquo;est pas une archive.
              </p>
              <button
                onClick={() => download('archive',
                  () => exportApi.legalArchive(),
                  `archive-legale_${new Date().toISOString().slice(0, 10)}.zip`)}
                disabled={loading === 'archive'}
                className="btn-primary btn-sm flex items-center gap-1.5"
              >
                <Download size={14} />
                {loading === 'archive' ? 'Préparation…' : "Télécharger l'archive"}
              </button>
              <p className="text-xs text-alpine-500 mt-2">
                L&rsquo;archive couvre l&rsquo;ensemble des données, sans filtre de période :
                c&rsquo;est ce que la conservation légale demande.
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function ExportCard({
  icon, title, description, loading, onClick,
}: {
  icon: React.ReactNode
  title: string
  description: string
  loading: boolean
  onClick: () => void
}) {
  return (
    <div className="card">
      <div className="card-body flex flex-col h-full">
        <div className="w-9 h-9 rounded-lg bg-accent-100 flex items-center justify-center
                        text-accent-600 mb-3">
          {icon}
        </div>
        <h3 className="font-semibold text-alpine-900 mb-1">{title}</h3>
        <p className="text-sm text-alpine-600 mb-4 flex-1">{description}</p>
        <button onClick={onClick} disabled={loading}
                className="btn-secondary btn-sm flex items-center gap-1.5 w-fit">
          <Download size={14} />
          {loading ? 'Préparation…' : 'Télécharger le CSV'}
        </button>
      </div>
    </div>
  )
}
