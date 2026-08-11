// Inscription du second facteur.
//
// Un compte administrateur peut créer des comptes, restaurer une sauvegarde,
// déverrouiller une période. Son mot de passe est la seule chose qui en sépare
// quelqu'un. Cet écran n'offre donc aucune sortie pour un administrateur qui
// n'est pas encore inscrit : le serveur refuse de toute façon toute autre
// requête, et proposer un bouton « plus tard » ne ferait qu'afficher une
// application dont chaque appel échouerait.
//
// Ce que le second facteur protège : le cas où le MOT DE PASSE fuit — réutilisé
// ailleurs, deviné, lu par-dessus l'épaule. Il ne protège pas de quelqu'un qui
// lit déjà le fichier de base ; celui-là n'a besoin d'aucun code. Ce qui répond
// à cette menace est le chiffrement de la base et celui du disque.

import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import {
  Mountain, Smartphone, Copy, Check, Loader2, ShieldCheck, KeyRound, AlertTriangle,
} from 'lucide-react'
import { authApi } from '@/api/client'
import { ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { useAuthStore } from '@/store/auth'
import { useT } from '@/i18n/useT'

interface SetupData {
  secret: string
  uri: string
  qr_png: string
  account: string
}

export function MFAEnrolmentPage() {
  const t = useT()
  const navigate = useNavigate()
  const clearMfaEnrolment = useAuthStore(s => s.clearMfaEnrolment)

  const [setup, setSetup] = useState<SetupData | null>(null)
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const start = useMutation({
    mutationFn: () => authApi.mfaSetup(),
    onSuccess: (r) => { setError(null); setSetup(r.data as SetupData) },
    onError: (e) => setError(refusalMessage(e, t('mf.echecDemarrage'))),
  })

  const confirm = useMutation({
    mutationFn: () => authApi.mfaConfirm(code.trim()),
    onSuccess: (r) => {
      setError(null)
      setCodes((r.data.recovery_codes ?? []) as string[])
      clearMfaEnrolment()
    },
    onError: (e) => { setError(refusalMessage(e, t('mf.codeRefuse'))); setCode('') },
  })

  const copyCodes = async () => {
    if (!codes) return
    try {
      await navigator.clipboard.writeText(codes.join('\n'))
      setCopied(true)
      setTimeout(() => setCopied(false), 2500)
    } catch {
      setError(t('mf.echecCopie'))
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-alpine-950 p-4">
      <div className="relative w-full max-w-lg">
        <div className="flex items-center justify-center gap-3 mb-8">
          <div className="w-10 h-10 rounded-xl bg-accent-500 flex items-center justify-center
                          shadow-lg shadow-accent-500/40">
            <Mountain size={20} className="text-white" />
          </div>
          <div className="font-display font-700 text-xl text-white">LedgerAlps</div>
        </div>

        <div className="bg-alpine-900 border border-alpine-700 rounded-2xl p-6">
          {/* ── Étape 3 : les codes de secours ─────────────────────────────── */}
          {codes ? (
            <>
              <h1 className="font-display font-700 text-lg text-white flex items-center gap-2">
                <ShieldCheck size={18} className="text-success-500" />
                {t('mf.active')}
              </h1>

              <div className="mt-4 rounded-lg border border-warning-500/40 bg-warning-500/10 p-4">
                <p className="text-sm text-warning-200 flex items-start gap-2">
                  <AlertTriangle size={16} className="shrink-0 mt-0.5" />
                  <span>
                    {t('mf.notezMaintenant')}
                  </span>
                </p>
              </div>

              <ul className="mt-4 grid grid-cols-2 gap-2 font-mono text-sm text-alpine-100">
                {codes.map(c => (
                  <li key={c} className="rounded border border-alpine-700 bg-alpine-800 px-3 py-2
                                         text-center tracking-wider">
                    {c}
                  </li>
                ))}
              </ul>

              <div className="mt-4 flex items-center gap-2">
                <button onClick={copyCodes} className="btn-secondary btn-sm flex items-center gap-1.5">
                  {copied ? <Check size={13} /> : <Copy size={13} />}
                  {copied ? t('us.copie') : t('mf.copierCodes')}
                </button>
              </div>

              <p className="text-xs text-alpine-500 mt-3">
                {t('mf.rangezAilleurs')}
              </p>
              {/* Dire OÙ ils se saisissent, pas seulement de les noter : un code
                  de secours se ressort des mois plus tard, dans un moment
                  d'urgence, et chercher le bouton n'est pas le moment. */}
              <p className="text-xs text-alpine-400 mt-2">
                {t('mf.ouLesSaisir')}
              </p>

              <button
                onClick={() => navigate('/', { replace: true })}
                className="btn-primary w-full mt-5"
              >
                {t('mf.jaiNote')}
              </button>
            </>
          ) : setup ? (
            /* ── Étape 2 : scanner et confirmer ────────────────────────────── */
            <>
              <h1 className="font-display font-700 text-lg text-white flex items-center gap-2">
                <Smartphone size={18} className="text-accent-500" />
                {t('mf.activezTitre')}
              </h1>
              <p className="text-sm text-alpine-400 mt-2">
                {t('mf.scannezAide')}
              </p>

              <div className="mt-5 flex flex-col items-center gap-3">
                <img src={setup.qr_png} alt={t('mf.altCodeAScanner')}
                     className="rounded-lg bg-white p-3" width={240} height={240} />
                <details className="w-full">
                  <summary className="text-xs text-alpine-400 cursor-pointer">
                    {t('mf.impossibleScanner')}
                  </summary>
                  <p className="mt-2 font-mono text-xs break-all rounded border border-alpine-700
                                bg-alpine-800 px-3 py-2 text-alpine-200">
                    {setup.secret}
                  </p>
                  <p className="text-xs text-alpine-500 mt-1">
                    {t('mf.compteEtType', { compte: setup.account })}
                  </p>
                </details>
              </div>

              <div className="mt-5">
                <label htmlFor="otp" className="label text-alpine-300">
                  {t('mf.codeAffiche')}
                </label>
                <input
                  id="otp" autoFocus inputMode="numeric" maxLength={7}
                  value={code} onChange={e => setCode(e.target.value)}
                  onKeyDown={e => { if (e.key === 'Enter' && code.trim().length >= 6) confirm.mutate() }}
                  className="input text-center text-xl tracking-[0.4em]"
                  placeholder="000000"
                />
              </div>

              {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

              <button
                onClick={() => confirm.mutate()}
                disabled={code.trim().length < 6 || confirm.isPending}
                className="btn-primary w-full mt-4 flex items-center justify-center gap-2"
              >
                {confirm.isPending && <Loader2 size={14} className="animate-spin" />}
                {t('mf.activerLeSecondFacteur')}
              </button>
            </>
          ) : (
            /* ── Étape 1 : pourquoi, et avec quoi ──────────────────────────── */
            <>
              <h1 className="font-display font-700 text-lg text-white flex items-center gap-2">
                <KeyRound size={18} className="text-accent-500" />
                {t('mf.protegezTitre')}
              </h1>
              <p className="text-sm text-alpine-400 mt-2">
                {t('mf.protegezAide')}
              </p>

              <div className="mt-4 rounded-lg border border-alpine-700 bg-alpine-800/50 p-4">
                <p className="text-sm text-alpine-300">
                  {t('mf.ilVousFaut')}
                </p>
                <ul className="mt-2 text-sm text-alpine-400 list-disc list-inside space-y-0.5">
                  <li>Aegis Authenticator (Android)</li>
                  <li>KeePassXC (Windows, macOS, Linux)</li>
                  <li>FreeOTP (Android, iOS)</li>
                </ul>
                <p className="text-xs text-alpine-500 mt-3">
                  {t('mf.rienNeSort')}
                </p>
              </div>

              {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

              <button
                onClick={() => start.mutate()}
                disabled={start.isPending}
                className="btn-primary w-full mt-5 flex items-center justify-center gap-2"
              >
                {start.isPending && <Loader2 size={14} className="animate-spin" />}
                {t('mf.commencer')}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
