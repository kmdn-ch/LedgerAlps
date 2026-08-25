// La comptabilité simplifiée — le « carnet du lait » du CO art. 957 al. 2.
//
// # Pourquoi les chiffres s'affichent AVANT de pouvoir être téléchargés
//
// C'est un document qui part aux impôts. Le télécharger sans l'avoir regardé,
// c'est remettre à l'administration un chiffre qu'on découvre en même temps
// qu'elle. L'écran montre donc les trois parties — recettes, dépenses,
// patrimoine — et le téléchargement ne fait que reprendre ce qui est à l'écran.
//
// # Le bandeau d'éligibilité n'est pas décoratif
//
// Au-delà de 500 000 francs de chiffre d'affaires, la comptabilité en partie
// double redevient obligatoire (CO art. 957 al. 1) et ce document ne peut plus
// être présenté seul. Le taire pour ne pas encombrer l'écran laisserait
// quelqu'un remettre une pièce que la loi ne reconnaît pas dans son cas.

import { useState } from 'react'
import { BookMarked, FileDown, FileSpreadsheet, AlertTriangle, CheckCircle2, Info } from 'lucide-react'
import { carnetApi } from '@/api/client'
import { useT, useFormats } from '@/i18n/useT'
import { refusalMessage } from '@/utils/refusal'
import { ErrorBanner } from '@/components/ui'

interface Ligne {
  code: string
  libelle: string
  montant: number
}

interface Carnet {
  du: string
  au: string
  recettes: Ligne[] | null
  depenses: Ligne[] | null
  total_recettes: number
  total_depenses: number
  resultat: number
  avoirs: Ligne[] | null
  engagements: Ligne[] | null
  total_avoirs: number
  total_engagements: number
  fortune: number
  eligibilite: {
    chiffre_affaires: number
    eligible: boolean
    assujetti_tva: boolean
    statut_declare: string
  }
}

export function CarnetDuLait({ du, au }: { du: string; au: string }) {
  const t = useT()
  const { montant } = useFormats()
  const [carnet, setCarnet] = useState<Carnet | null>(null)
  const [chargement, setChargement] = useState<string | null>(null)
  const [erreur, setErreur] = useState('')

  const etablir = async () => {
    setChargement('lecture')
    setErreur('')
    try {
      const res = await carnetApi.lire(du, au)
      setCarnet(res.data as Carnet)
    } catch (e) {
      setErreur(refusalMessage(e, t('carnet.erreur')))
    } finally {
      setChargement(null)
    }
  }

  const telecharger = async (
    cle: string, fetcher: () => Promise<{ data: BlobPart }>, nom: string,
  ) => {
    setChargement(cle)
    setErreur('')
    try {
      const res = await fetcher()
      const url = URL.createObjectURL(new Blob([res.data]))
      const a = document.createElement('a')
      a.href = url
      a.download = nom
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch (e) {
      setErreur(refusalMessage(e, t('carnet.erreur')))
    } finally {
      setChargement(null)
    }
  }

  const periode = `${du}_${au}`

  return (
    <div className="card mb-8">
      <div className="card-body">
        <div className="flex items-start gap-3 mb-3">
          <BookMarked size={20} className="text-accent-600 mt-0.5 shrink-0" />
          <div>
            <h3 className="font-semibold">{t('carnet.titre')}</h3>
            <p className="text-sm text-alpine-600 mt-1">{t('carnet.aide')}</p>
          </div>
        </div>

        {/* Ce que « base de caisse » veut dire, dit une fois et clairement.
            Sans cette phrase, l'écart entre le total des recettes et le chiffre
            d'affaires passe pour une erreur de calcul. */}
        <div className="flex items-start gap-2 rounded bg-alpine-50 p-3 text-xs text-alpine-700 mb-4">
          <Info size={14} className="mt-0.5 shrink-0 text-alpine-500" />
          <span>{t('carnet.baseCaisse')}</span>
        </div>

        {erreur && <ErrorBanner message={erreur} />}

        <button className="btn btn-primary" onClick={etablir} disabled={chargement !== null}>
          {chargement === 'lecture' ? t('carnet.calcul') : t('carnet.etablir')}
        </button>

        {carnet && (
          <div className="mt-5">
            {/* Le régime applicable, en tête : c'est ce qui décide si le
                document suffit. */}
            <div className={`flex items-start gap-2 rounded p-3 text-sm mb-5 ${
              carnet.eligibilite.eligible
                ? 'bg-emerald-50 text-emerald-900'
                : 'bg-red-50 text-red-900'
            }`}>
              {carnet.eligibilite.eligible
                ? <CheckCircle2 size={16} className="mt-0.5 shrink-0" />
                : <AlertTriangle size={16} className="mt-0.5 shrink-0" />}
              <div>
                <p className="font-medium">
                  {t('carnet.ca')} {montant(carnet.eligibilite.chiffre_affaires)}
                </p>
                <p className="mt-1">
                  {carnet.eligibilite.eligible ? t('carnet.admise') : t('carnet.refusee')}
                </p>
                <p className="mt-1">
                  {carnet.eligibilite.assujetti_tva ? t('carnet.tvaDue') : t('carnet.tvaLibere')}
                </p>
              </div>
            </div>

            <Bloc titre={t('carnet.recettes')} lignes={carnet.recettes}
                  total={carnet.total_recettes} libelleTotal={t('carnet.totalRecettes')}
                  siVide={t('carnet.aucun')} />
            <Bloc titre={t('carnet.depenses')} lignes={carnet.depenses}
                  total={carnet.total_depenses} libelleTotal={t('carnet.totalDepenses')}
                  siVide={t('carnet.aucun')} />

            <div className="flex justify-between items-center rounded bg-brand-navyCard px-4 py-3 text-white font-semibold mb-6">
              <span>{t('carnet.resultat')}</span>
              <span className="tabular-nums">{montant(carnet.resultat)}</span>
            </div>

            <h4 className="font-semibold text-sm mb-2">
              {t('carnet.patrimoine')} {carnet.au}
            </h4>
            <Bloc titre={t('carnet.avoirs')} lignes={carnet.avoirs}
                  total={carnet.total_avoirs} libelleTotal={t('carnet.totalAvoirs')}
                  siVide={t('carnet.neant')} />
            <Bloc titre={t('carnet.engagements')} lignes={carnet.engagements}
                  total={carnet.total_engagements} libelleTotal={t('carnet.totalEngagements')}
                  siVide={t('carnet.neant')} />

            <div className="flex justify-between items-center border-t-2 border-alpine-300 pt-2 font-semibold mb-6">
              <span>{t('carnet.fortune')}</span>
              <span className="tabular-nums">{montant(carnet.fortune)}</span>
            </div>

            <div className="flex flex-wrap gap-3">
              <button
                className="btn btn-secondary inline-flex items-center gap-2"
                disabled={chargement !== null}
                onClick={() => telecharger('pdf',
                  () => carnetApi.pdf(du, au), `comptabilite-simplifiee_${periode}.pdf`)}>
                <FileDown size={16} />
                {chargement === 'pdf' ? t('carnet.preparation') : t('carnet.pdf')}
              </button>
              <button
                className="btn btn-secondary inline-flex items-center gap-2"
                disabled={chargement !== null}
                onClick={() => telecharger('csv',
                  () => carnetApi.csv(du, au), `comptabilite-simplifiee_${periode}.csv`)}>
                <FileSpreadsheet size={16} />
                {chargement === 'csv' ? t('carnet.preparation') : t('carnet.csv')}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

/** Un bloc de postes avec son total. */
function Bloc({ titre, lignes, total, libelleTotal, siVide }: {
  titre: string; lignes: Ligne[] | null; total: number; libelleTotal: string
  // « Aucun mouvement » vaut pour un flux ; un état du patrimoine vide se dit
  // « néant ». Le même libellé pour les deux ferait lire un relevé là où il y a
  // une situation à une date.
  siVide: string
}) {
  const { montant } = useFormats()
  return (
    <div className="mb-5">
      <h4 className="font-semibold text-sm mb-1">{titre}</h4>
      <table className="w-full text-sm">
        <tbody>
          {(!lignes || lignes.length === 0) && (
            <tr>
              <td className="py-1 text-alpine-400 italic" colSpan={3}>{siVide}</td>
            </tr>
          )}
          {(lignes ?? []).map(l => (
            <tr key={l.code} className="border-b border-neutral-100">
              <td className="py-1 pr-3 font-mono text-xs text-alpine-500 w-16">{l.code}</td>
              <td className="py-1 pr-3">{l.libelle}</td>
              <td className="py-1 text-right tabular-nums">{montant(l.montant)}</td>
            </tr>
          ))}
          <tr className="border-t border-alpine-300">
            <td />
            <td className="py-1.5 pr-3 text-right font-medium">{libelleTotal}</td>
            <td className="py-1.5 text-right font-medium tabular-nums">{montant(total)}</td>
          </tr>
        </tbody>
      </table>
    </div>
  )
}
