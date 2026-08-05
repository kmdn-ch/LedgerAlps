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

interface SetupData {
  secret: string
  uri: string
  qr_png: string
  account: string
}

export function MFAEnrolmentPage() {
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
    onError: (e) => setError(refusalMessage(e, "L'inscription n'a pas pu démarrer.")),
  })

  const confirm = useMutation({
    mutationFn: () => authApi.mfaConfirm(code.trim()),
    onSuccess: (r) => {
      setError(null)
      setCodes((r.data.recovery_codes ?? []) as string[])
      clearMfaEnrolment()
    },
    onError: (e) => { setError(refusalMessage(e, 'Le code n’a pas été accepté.')); setCode('') },
  })

  const copyCodes = async () => {
    if (!codes) return
    try {
      await navigator.clipboard.writeText(codes.join('\n'))
      setCopied(true)
      setTimeout(() => setCopied(false), 2500)
    } catch {
      setError('La copie a échoué. Notez les codes à la main.')
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
                Second facteur activé
              </h1>

              <div className="mt-4 rounded-lg border border-warning-500/40 bg-warning-500/10 p-4">
                <p className="text-sm text-warning-200 flex items-start gap-2">
                  <AlertTriangle size={16} className="shrink-0 mt-0.5" />
                  <span>
                    <strong>Notez ces codes maintenant.</strong> Ils ne seront plus jamais
                    affichés. Sans eux, un téléphone perdu vous ferme définitivement l&rsquo;accès —
                    et personne d&rsquo;autre ne peut vous le rendre.
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
                  {copied ? 'Copié' : 'Copier les codes'}
                </button>
              </div>

              <p className="text-xs text-alpine-500 mt-3">
                Rangez-les ailleurs que sur ce PC et hors de votre téléphone : un papier dans un
                tiroir fermé fait très bien l&rsquo;affaire. Chacun ne sert qu&rsquo;une fois.
              </p>

              <button
                onClick={() => navigate('/', { replace: true })}
                className="btn-primary w-full mt-5"
              >
                J&rsquo;ai noté mes codes de secours
              </button>
            </>
          ) : setup ? (
            /* ── Étape 2 : scanner et confirmer ────────────────────────────── */
            <>
              <h1 className="font-display font-700 text-lg text-white flex items-center gap-2">
                <Smartphone size={18} className="text-accent-500" />
                Enregistrez votre application
              </h1>
              <p className="text-sm text-alpine-400 mt-2">
                Scannez ce code avec votre application d&rsquo;authentification, puis saisissez
                celui qu&rsquo;elle affiche.
              </p>

              <div className="mt-5 flex flex-col items-center gap-3">
                <img src={setup.qr_png} alt="Code à scanner"
                     className="rounded-lg bg-white p-3" width={240} height={240} />
                <details className="w-full">
                  <summary className="text-xs text-alpine-400 cursor-pointer">
                    Impossible de scanner ? Saisir la clé à la main
                  </summary>
                  <p className="mt-2 font-mono text-xs break-all rounded border border-alpine-700
                                bg-alpine-800 px-3 py-2 text-alpine-200">
                    {setup.secret}
                  </p>
                  <p className="text-xs text-alpine-500 mt-1">
                    Compte : {setup.account} — type « temporel », six chiffres, 30 secondes.
                  </p>
                </details>
              </div>

              <div className="mt-5">
                <label htmlFor="otp" className="label text-alpine-300">
                  Code affiché par l&rsquo;application
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
                Activer le second facteur
              </button>
            </>
          ) : (
            /* ── Étape 1 : pourquoi, et avec quoi ──────────────────────────── */
            <>
              <h1 className="font-display font-700 text-lg text-white flex items-center gap-2">
                <KeyRound size={18} className="text-accent-500" />
                Protégez le compte administrateur
              </h1>
              <p className="text-sm text-alpine-400 mt-2">
                Ce compte peut créer des comptes, restaurer une sauvegarde et déverrouiller une
                période. Aujourd&rsquo;hui, seul votre mot de passe l&rsquo;en sépare. Un second
                facteur fait qu&rsquo;un mot de passe volé ne suffit plus.
              </p>

              <div className="mt-4 rounded-lg border border-alpine-700 bg-alpine-800/50 p-4">
                <p className="text-sm text-alpine-300">
                  Il vous faut une application d&rsquo;authentification sur votre téléphone.
                  Toutes conviennent ; les suivantes sont libres et fonctionnent hors ligne :
                </p>
                <ul className="mt-2 text-sm text-alpine-400 list-disc list-inside space-y-0.5">
                  <li>Aegis Authenticator (Android)</li>
                  <li>KeePassXC (Windows, macOS, Linux)</li>
                  <li>FreeOTP (Android, iOS)</li>
                </ul>
                <p className="text-xs text-alpine-500 mt-3">
                  Rien ne sort de cette machine : le code se calcule à partir d&rsquo;un secret
                  partagé une seule fois, sans aucun appel réseau.
                </p>
              </div>

              {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

              <button
                onClick={() => start.mutate()}
                disabled={start.isPending}
                className="btn-primary w-full mt-5 flex items-center justify-center gap-2"
              >
                {start.isPending && <Loader2 size={14} className="animate-spin" />}
                Commencer
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
