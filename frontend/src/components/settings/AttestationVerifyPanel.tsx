// Vérifier une attestation d'intégrité.
//
// # Pourquoi cet écran existe
//
// L'attestation était produite et remise à un tiers qui n'avait aucun moyen de
// la contrôler. Un document invérifiable ne vaut pas mieux qu'une affirmation
// orale : il documentait l'état d'un mécanisme, et il fallait le croire.
//
// # Ce que l'écran montre, et dans quel ordre
//
// Trois lignes, de la plus décisive à la moins :
//
//   1. Le SCEAU — le document a-t-il été retouché depuis son émission ?
//   2. La CORRESPONDANCE — les livres portent-ils toujours la même empreinte
//      au maillon attesté ? C'est le contrôle qui a du pouvoir de preuve.
//   3. L'ÉTAT ACTUEL — la chaîne est-elle intacte aujourd'hui ?
//
// Le verdict est une phrase, pas trois pastilles. Quelqu'un qui vient vérifier
// une attestation veut savoir s'il peut s'y fier, pas décoder un tableau.

import { useState } from 'react'
import { ShieldCheck, ShieldAlert, Upload, Loader2, Info } from 'lucide-react'
import { auditApi } from '@/api/client'
import { SectionTitle, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { useT, useFormats } from '@/i18n/useT'

interface Verdict {
  seal_valid: boolean
  head_matches: boolean
  chain_intact: boolean
  // Le serveur dit lui-même si le verdict est bon. L'écran ne le recalcule
  // pas : un ET des trois booléens ci-dessus mettait le cadre en rouge sous
  // « Attestation vérifiée » quand la chaîne était vide.
  ok: boolean
  nothing_covered: boolean
  issued_at: string
  issued_by: string
  covered_sequence: number
  attested_head_hash: string
  current_head_hash: string
  entries_since: number
  verdict: string
  detail: string
}

export function AttestationVerifyPanel() {
  const t = useT()
  const { date } = useFormats()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [v, setV] = useState<Verdict | null>(null)

  async function verifier(fichier: File) {
    setBusy(true); setError(null); setV(null)
    try {
      const res = await auditApi.verifyAttestation(fichier)
      setV(res.data as Verdict)
    } catch (e) {
      setError(refusalMessage(e, t('vf.echec')))
    } finally {
      setBusy(false)
    }
  }

  // Un seul contrôle qui tombe suffit à changer la couleur : nuancer
  // inviterait à passer outre. C'est le serveur qui tranche — il a rédigé la
  // phrase, et la couleur doit dire la même chose qu'elle.
  const bon = v !== null && v.ok

  return (
    <div className="mt-6">
      <SectionTitle>{t('vf.titre')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">{t('vf.introduction')}</p>

      <label className="btn-secondary btn-sm inline-flex items-center gap-1.5 cursor-pointer">
        {busy ? <Loader2 size={14} className="animate-spin" /> : <Upload size={14} />}
        {busy ? t('vf.verification') : t('vf.choisirFichier')}
        <input
          type="file"
          accept=".json,application/json"
          className="hidden"
          onChange={e => {
            const f = e.target.files?.[0]
            if (f) verifier(f)
            e.target.value = ''
          }}
        />
      </label>

      {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

      {v && (
        <div className={`mt-4 rounded-md border px-4 py-3 text-sm ${
          bon ? 'border-neutral-200 bg-neutral-50' : 'border-danger-500 bg-danger-100'
        }`}>
          <div className="flex items-start gap-2">
            {bon
              ? <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
              : <ShieldAlert size={16} className="mt-0.5 flex-shrink-0 text-danger-700" />}
            <div className="flex-1">
              <p className={`font-medium ${bon ? 'text-success-700' : 'text-danger-700'}`}>
                {v.verdict}
              </p>
              <p className="text-alpine-700 mt-1">{v.detail}</p>

              <p className="text-xs text-alpine-500 mt-2">
                {t('vf.emiseLe', { date: date(v.issued_at), auteur: v.issued_by })}
              </p>

              <ul className="mt-2 space-y-0.5 text-xs text-alpine-700">
                <Ligne
                  ok={v.seal_valid}
                  quoi={t('vf.sceau')}
                  etat={v.seal_valid ? t('vf.sceauOk') : t('vf.sceauKo')}
                />
                {v.nothing_covered ? (
                  <Ligne
                    etatCouleur="neutre"
                    quoi={t('vf.correspondanceRien')}
                    etat={t('vf.correspondanceSansObjet')}
                  />
                ) : (
                  <Ligne
                    ok={v.head_matches}
                    quoi={t('vf.correspondance', { n: v.covered_sequence })}
                    etat={v.head_matches ? t('vf.correspondanceOk') : t('vf.correspondanceKo')}
                  />
                )}
                <Ligne
                  ok={v.chain_intact}
                  quoi={t('vf.chaine')}
                  etat={v.chain_intact ? t('vf.chaineOk') : t('vf.chaineKo')}
                />
              </ul>

              {v.entries_since > 0 && (
                <p className="text-xs text-alpine-500 mt-2">
                  {t('vf.ecrituresDepuis', { n: v.entries_since })}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      <div className="mt-3 flex items-start gap-2 rounded-md border border-alpine-200
                      bg-alpine-50 px-3 py-2">
        <Info size={14} className="text-alpine-500 flex-shrink-0 mt-0.5" />
        <p className="text-xs text-alpine-600">
          <strong>{t('vf.commentLaFiduciaireVerifie')}</strong> — {t('vf.explicationFiduciaire')}
        </p>
      </div>
    </div>
  )
}

// Une ligne de contrôle. Le troisième état — « neutre » — existe parce qu'un
// contrôle peut être SANS OBJET : sur des livres vides, il n'y a pas
// d'empreinte à comparer, et l'afficher en rouge accuserait à tort.
function Ligne({
  ok,
  quoi,
  etat,
  etatCouleur,
}: {
  ok?: boolean
  quoi: string
  etat: string
  etatCouleur?: 'neutre'
}) {
  const couleur =
    etatCouleur === 'neutre'
      ? 'text-alpine-500'
      : ok
        ? 'text-success-700 font-medium'
        : 'text-danger-700 font-medium'
  return (
    <li className="flex items-center justify-between gap-3">
      <span>{quoi}</span>
      <span className={couleur}>{etat}</span>
    </li>
  )
}
