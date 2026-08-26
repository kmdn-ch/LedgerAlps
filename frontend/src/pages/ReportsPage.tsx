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
import { Link } from 'react-router-dom'
import { exportApi, accountingExportApi } from '@/api/client'
import { PaymentRunPanel } from '@/components/payments/PaymentRunPanel'
import { CarnetDuLait } from '@/components/reports/CarnetDuLait'
import { PageHeader, SectionTitle, ErrorBanner } from '@/components/ui'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { useT } from '@/i18n/useT'
import { refusalMessage } from '@/utils/refusal'

export function ReportsPage() {
  const t = useT()
  const peutEcrire = useCanWrite()
  const [startDate, setStartDate] = useState(() =>
    new Date(new Date().getFullYear(), 0, 1).toISOString().slice(0, 10)
  )
  const [endDate, setEndDate] = useState(() => new Date().toISOString().slice(0, 10))
  const [loading, setLoading] = useState<string | null>(null)
  const [error, setError] = useState('')

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

  const periode = `${startDate}_${endDate}`

  return (
    <div>
      <PageHeader
        title={t('nav.rapports')}
        aide={t('aide.rapports')}
        subtitle={t('rp.sousTitre')}
      />

      {error && <div className="mb-4"><ErrorBanner message={error} /></div>}

      {/* Période */}
      <div className="card mb-6">
        <div className="card-body flex items-center gap-4 flex-wrap">
          <Calendar size={16} className="text-alpine-400" />
          <div className="flex items-center gap-2">
            <label className="text-sm text-alpine-600" htmlFor="du">{t('rp.du')}</label>
            <input id="du" type="date" className="input w-40"
                   value={startDate} onChange={e => setStartDate(e.target.value)} />
          </div>
          <div className="flex items-center gap-2">
            <label className="text-sm text-alpine-600" htmlFor="au">{t('rp.au')}</label>
            <input id="au" type="date" className="input w-40"
                   value={endDate} onChange={e => setEndDate(e.target.value)} />
          </div>
          <p className="text-xs text-alpine-400">
            {t('rp.periodeAide')}
          </p>
        </div>
      </div>

      <SectionTitle>{t('rp.documentsComptables')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        {t('rp.csvAide')}
      </p>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">
        <ExportCard
          icon={<BookOpen size={20} />}
          title={t('rp.journalGeneral')}
          description={t('rp.journalGeneralAide')}
          loading={loading === 'journal'}
          onClick={() => download('journal',
            () => accountingExportApi.journal(startDate, endDate),
            `journal_${periode}.csv`)}
        />
        <ExportCard
          icon={<FileSpreadsheet size={20} />}
          title={t('compta.grandLivre')}
          description={t('rp.grandLivreAide')}
          loading={loading === 'ledger'}
          onClick={() => download('ledger',
            () => accountingExportApi.ledger(startDate, endDate),
            `grand-livre_${periode}.csv`)}
        />
        <ExportCard
          icon={<BarChart3 size={20} />}
          title={t('compta.balance')}
          description={t('rp.balanceAide')}
          loading={loading === 'balance'}
          onClick={() => download('balance',
            () => accountingExportApi.trialBalance(startDate, endDate),
            `balance_${periode}.csv`)}
        />
      </div>

      {/* La comptabilité simplifiée : une LECTURE, donc ouverte à tous les
          rôles, y compris la fiduciaire en lecture seule — c'est justement
          elle qui établit ce document pour l'administration. */}
      <SectionTitle>{t('carnet.titre')}</SectionTitle>
      <CarnetDuLait du={startDate} au={endDate} />

      {/* L'import camt.053 ne vit PLUS ici.

          Il existait a deux endroits — ici et Parametres -> Banque — pour la
          meme route et le meme effet : les deux ecrivaient en base. Celui-ci
          presentait une liste de lecture, « voila ce que j'ai lu », sans dire
          que les ecritures etaient PERSISTEES au passage. L'utilisateur
          croyait consulter un fichier ; il alimentait la file de
          rapprochement.

          Deux boutons pour un meme geste, dont l'un cache ce qu'il fait, ne
          sont pas une commodite. L'import vit la ou se fait le travail qui
          le suit : le rapprochement. */}
      {peutEcrire && (
      <>
      <SectionTitle>{t('rp.importBancaire')}</SectionTitle>
      <div className="card mb-8">
        <div className="card-body flex items-start gap-4">
          <div className="w-10 h-10 rounded-xl bg-alpine-100 flex items-center
                          justify-center flex-shrink-0">
            <Upload size={18} className="text-alpine-600" />
          </div>
          <div className="flex-1">
            <h3 className="font-semibold text-alpine-900 mb-1">{t('rp.releveCamt')}</h3>
            <p className="text-sm text-alpine-600 mb-3">{t('rp.importVitAilleurs')}</p>
            {/* Un FRAGMENT, et la clé exacte : SettingsPage lit `#banking`, pas
                `?tab=`. Une adresse qui ne correspond à rien ne produit aucune
                erreur — elle retombe en silence sur le premier onglet, et le
                renvoi déposait le lecteur sur « Identité » sans rien dire. */}
            <Link to="/settings#banking"
                  className="btn-secondary btn-sm inline-flex items-center gap-1.5">
              <Upload size={14} />
              {t('rp.allerAuRapprochement')}
            </Link>
          </div>
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
              {t(RAISON_LECTURE_SEULE)} {t('rp.exportsOuverts')}
            </p>
          </div>
        </div>
      )}

      {/* Archivage légal */}
      <SectionTitle>{t('rp.archivage')}</SectionTitle>
      <div className="card border-alpine-200">
        <div className="card-body">
          <div className="flex items-start gap-4">
            <div className="w-10 h-10 rounded-xl bg-alpine-800 flex items-center
                            justify-center flex-shrink-0">
              <Archive size={18} className="text-white" />
            </div>
            <div className="flex-1">
              <h3 className="font-semibold text-alpine-900 mb-1">{t('rp.archiveZIP')}</h3>
              <p className="text-sm text-alpine-600 mb-3">
                {t('rp.archiveZIPAide')}
              </p>
              <button
                onClick={() => download('archive',
                  () => exportApi.legalArchive(),
                  `archive-legale_${new Date().toISOString().slice(0, 10)}.zip`)}
                disabled={loading === 'archive'}
                className="btn-primary btn-sm flex items-center gap-1.5"
              >
                <Download size={14} />
                {loading === 'archive' ? t('rp.preparation') : t('rp.telechargerArchive')}
              </button>
              <p className="text-xs text-alpine-500 mt-2">
                {t('rp.archiveAide')}
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
  const t = useT()
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
          {loading ? t('rp.preparation') : t('rp.telechargerCSV')}
        </button>
      </div>
    </div>
  )
}
