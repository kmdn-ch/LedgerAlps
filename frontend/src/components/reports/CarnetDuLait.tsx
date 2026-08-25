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

import { useEffect, useMemo, useState } from 'react'
import { BookMarked, FileDown, FileSpreadsheet, AlertTriangle, CheckCircle2, Info } from 'lucide-react'
import { carnetApi, fiscalYearsApi, settingsApi } from '@/api/client'
import { useT, useFormats } from '@/i18n/useT'
import { refusalMessage } from '@/utils/refusal'
import { ErrorBanner } from '@/components/ui'
import {
  CLE_LIBRE, choisirParDefaut, depuisDeclares, derives,
  type Exercice, type ExerciceDeclare,
} from '@/utils/exercices'

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
    sur_exercice_complet: boolean
    assujetti_tva: boolean
    statut_declare: string
  }
}

/** Seuil du CO art. 957 al. 2 ch. 1, en francs. */
const SEUIL_REGIME = 500_000

// Les props `du`/`au` sont la période des autres exports de l'écran. Le carnet
// ne s'en sert PLUS par défaut — il propose un exercice — mais elle amorce
// l'option « période personnalisée », pour ne rien retirer à qui l'utilisait.
export function CarnetDuLait({ du, au }: { du: string; au: string }) {
  const t = useT()
  const { montant } = useFormats()
  const [carnet, setCarnet] = useState<Carnet | null>(null)
  const [chargement, setChargement] = useState<string | null>(null)
  const [erreur, setErreur] = useState('')

  const [exercices, setExercices] = useState<Exercice[]>([])
  const [declares, setDeclares] = useState(false)
  const [cle, setCle] = useState<string>('')
  const [libreDu, setLibreDu] = useState(du)
  const [libreAu, setLibreAu] = useState(au)

  // Les exercices déclarés d'abord ; à défaut, ceux que le mois de début
  // d'exercice permet de déduire. Un échec de lecture n'est pas remonté à
  // l'utilisateur : il lui resterait la période personnalisée, et une bannière
  // rouge sur un menu qui a des valeurs de repli l'inquiéterait pour rien.
  useEffect(() => {
    let vivant = true
    ;(async () => {
      let liste: Exercice[] = []
      let sontDeclares = false
      try {
        // La route rend `{ items, total }`, pas un tableau nu — lire `data`
        // directement donnait un objet, dont le `.map` levait une exception
        // que le `catch` avalait : les exercices déclarés n'auraient JAMAIS
        // servi, et le repli déduit serait passé pour le comportement normal.
        const res = await fiscalYearsApi.list()
        const brut = (res.data as { items?: ExerciceDeclare[] })?.items ?? []
        liste = depuisDeclares(brut)
        sontDeclares = liste.length > 0
      } catch { /* on se rabat sur la déduction */ }

      if (liste.length === 0) {
        let mois = 1
        try {
          const res = await settingsApi.getCompany()
          mois = Number((res.data as { fiscal_year_start_month?: number })
            ?.fiscal_year_start_month) || 1
        } catch { /* janvier, le cas de loin le plus courant */ }
        liste = derives(mois)
      }

      if (!vivant) return
      setExercices(liste)
      setDeclares(sontDeclares)
      setCle(choisirParDefaut(liste)?.cle ?? CLE_LIBRE)
    })()
    return () => { vivant = false }
  }, [])

  // La période effectivement demandée à l'API.
  const choisi = useMemo(() => exercices.find(e => e.cle === cle) ?? null, [exercices, cle])
  const periodeDu = choisi ? choisi.du : libreDu
  const periodeAu = choisi ? choisi.au : libreAu
  const pret = Boolean(periodeDu && periodeAu)

  const etablir = async () => {
    setChargement('lecture')
    setErreur('')
    try {
      const res = await carnetApi.lire(periodeDu, periodeAu)
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

  const periode = `${periodeDu}_${periodeAu}`

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

        {/* L'exercice, et non la plage partagée par les autres exports.
            Les deux seuils portent sur le chiffre d'affaires du dernier
            exercice : sur une plage partielle le document ne conclut pas, et
            la valeur par défaut de l'écran n'en était jamais un. */}
        <div className="mb-4">
          <label className="label" htmlFor="carnet-exercice">{t('carnet.exercice')}</label>
          <select
            id="carnet-exercice"
            className="select max-w-sm"
            value={cle}
            onChange={e => setCle(e.target.value)}
            disabled={chargement !== null}
          >
            {exercices.map(e => (
              <option key={e.cle} value={e.cle}>
                {e.nom} — {e.du} → {e.au}
              </option>
            ))}
            <option value={CLE_LIBRE}>{t('carnet.periodeLibre')}</option>
          </select>

          {/* Dire d'où vient la liste. Un exercice DÉDUIT du mois de début
              peut ne pas correspondre à celui que l'entreprise tient
              réellement ; le lecteur doit pouvoir s'en méfier. */}
          {exercices.length > 0 && !declares && (
            <p className="text-xs text-alpine-500 mt-1">{t('carnet.exerciceDeduit')}</p>
          )}

          {!choisi && (
            <div className="flex flex-wrap items-end gap-3 mt-3">
              <div>
                <label className="label" htmlFor="carnet-du">{t('rp.du')}</label>
                <input id="carnet-du" type="date" className="input" value={libreDu}
                       onChange={e => setLibreDu(e.target.value)} />
              </div>
              <div>
                <label className="label" htmlFor="carnet-au">{t('rp.au')}</label>
                <input id="carnet-au" type="date" className="input" value={libreAu}
                       onChange={e => setLibreAu(e.target.value)} />
              </div>
            </div>
          )}
        </div>

        {erreur && <ErrorBanner message={erreur} />}

        <button className="btn btn-primary" onClick={etablir}
                disabled={chargement !== null || !pret}>
          {chargement === 'lecture' ? t('carnet.calcul') : t('carnet.etablir')}
        </button>

        {carnet && (
          <div className="mt-5">
            {/* Le régime applicable, en tête : c'est ce qui décide si le
                document suffit. */}
            {(() => {
              // Trois états, et non deux. Sur une période qui n'est pas un
              // exercice, l'écran SUSPEND son verdict au lieu d'en rendre un
              // favorable : les seuils du CO art. 957 et de la LTVA art. 10
              // portent sur le chiffre d'affaires du dernier exercice. Un
              // dépassement, lui, reste concluant sur toute période — il ne
              // peut que s'aggraver en s'allongeant.
              const depasse = carnet.eligibilite.chiffre_affaires >= SEUIL_REGIME
              const suspendu = !carnet.eligibilite.sur_exercice_complet && !depasse
              const ton = suspendu
                ? 'bg-amber-50 text-amber-900'
                : carnet.eligibilite.eligible
                  ? 'bg-emerald-50 text-emerald-900'
                  : 'bg-red-50 text-red-900'
              return (
                <div className={`flex items-start gap-2 rounded p-3 text-sm mb-5 ${ton}`}>
                  {suspendu
                    ? <Info size={16} className="mt-0.5 shrink-0" />
                    : carnet.eligibilite.eligible
                      ? <CheckCircle2 size={16} className="mt-0.5 shrink-0" />
                      : <AlertTriangle size={16} className="mt-0.5 shrink-0" />}
                  <div>
                    <p className="font-medium">
                      {t('carnet.ca')} {montant(carnet.eligibilite.chiffre_affaires)}
                    </p>
                    {suspendu ? (
                      <p className="mt-1">{t('carnet.periodePartielle')}</p>
                    ) : (
                      <>
                        <p className="mt-1">
                          {carnet.eligibilite.eligible ? t('carnet.admise') : t('carnet.refusee')}
                        </p>
                        <p className="mt-1">
                          {carnet.eligibilite.assujetti_tva ? t('carnet.tvaDue') : t('carnet.tvaLibere')}
                        </p>
                      </>
                    )}
                  </div>
                </div>
              )
            })()}

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
                  () => carnetApi.pdf(periodeDu, periodeAu), `comptabilite-simplifiee_${periode}.pdf`)}>
                <FileDown size={16} />
                {chargement === 'pdf' ? t('carnet.preparation') : t('carnet.pdf')}
              </button>
              <button
                className="btn btn-secondary inline-flex items-center gap-2"
                disabled={chargement !== null}
                onClick={() => telecharger('csv',
                  () => carnetApi.csv(periodeDu, periodeAu), `comptabilite-simplifiee_${periode}.csv`)}>
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
