// Changement du mot de passe temporaire.
//
// Le mot de passe créé par un administrateur pour quelqu'un d'autre lui a été
// transmis par message, par téléphone ou sur un papier. Il est donc connu de
// deux personnes et a voyagé par un canal qui n'est pas fait pour ça.
//
// Tant qu'il n'est pas remplacé, l'administrateur peut se connecter au nom de
// l'autre — et les actions seraient tracées sous un compte qui n'est pas celui
// de leur auteur réel. C'est ce que cet écran empêche, et c'est pourquoi il
// n'offre aucune sortie : le serveur refuse de toute façon toute autre requête.

import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation } from '@tanstack/react-query'
import { Mountain, KeyRound, Check, Minus, Loader2 } from 'lucide-react'
import { authApi } from '@/api/client'
import { ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { useAuthStore } from '@/store/auth'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

// La même règle que le serveur (internal/api/handlers/password_change.go).
// Reproduite pour être visible pendant la frappe plutôt que révélée par un
// refus après coup ; le serveur reste l'autorité.
const MIN_LEN = 12

// Les critères portent leur CLÉ : la fonction est appelée hors composant, et
// c'est le rendu qui traduit. Les mêmes clés que PassphraseField, parce que
// ce sont les mêmes critères — les dupliquer les ferait diverger.
function checks(p: string): { cle: Cle; met: boolean }[] {
  return [
    { cle: 'pf.longueurMini', met: [...p].length >= MIN_LEN },
    { cle: 'pf.uneMinuscule', met: /\p{Ll}/u.test(p) },
    { cle: 'pf.uneMajuscule', met: /\p{Lu}/u.test(p) },
    { cle: 'pf.unChiffre',    met: /\p{Nd}/u.test(p) },
  ]
}

export function ChangePasswordPage() {
  const t = useT()
  const navigate = useNavigate()
  const clearMustChange = useAuthStore(s => s.clearMustChange)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)

  const list = checks(next)
  const strong = list.every(c => c.met)
  const matches = next !== '' && next === confirm
  const different = next !== current

  const change = useMutation({
    mutationFn: () => authApi.changePassword(current, next),
    onSuccess: () => { clearMustChange(); navigate('/', { replace: true }) },
    onError: (e) => setError(refusalMessage(e, t('mdp.echecChangement'))),
  })

  return (
    <div className="min-h-screen flex items-center justify-center bg-alpine-950 p-4">
      <div className="relative w-full max-w-md">
        <div className="flex items-center justify-center gap-3 mb-8">
          <div className="w-10 h-10 rounded-xl bg-accent-500 flex items-center justify-center
                          shadow-lg shadow-accent-500/40">
            <Mountain size={20} className="text-white" />
          </div>
          <div className="font-display font-700 text-xl text-white">LedgerAlps</div>
        </div>

        <div className="bg-alpine-900 border border-alpine-700 rounded-2xl p-6">
          <h1 className="font-display font-700 text-lg text-white flex items-center gap-2">
            <KeyRound size={18} className="text-accent-500" />
            {t('mdp.titre')}
          </h1>
          <p className="text-sm text-alpine-400 mt-2">
            {t('mdp.introduction')}
          </p>

          <div className="mt-5 space-y-4">
            <div>
              <label className="label text-alpine-300" htmlFor="cur">{t('mdp.champTemporaire')}</label>
              <input id="cur" type="password" className="input" autoComplete="current-password"
                     value={current} onChange={e => setCurrent(e.target.value)} autoFocus />
            </div>

            <div>
              <label className="label text-alpine-300" htmlFor="new">{t('mdp.champNouveau')}</label>
              <input id="new" type="password" className="input" autoComplete="new-password"
                     value={next} onChange={e => setNext(e.target.value)} />
              {next !== '' && (
                <ul className="mt-2 space-y-0.5">
                  {list.map(c => (
                    <li key={c.cle} className={`text-xs flex items-center gap-1.5 ${
                      c.met ? 'text-success-500' : 'text-alpine-500'
                    }`}>
                      {c.met ? <Check size={12} /> : <Minus size={12} />}
                      {t(c.cle, { n: MIN_LEN })}
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div>
              <label className="label text-alpine-300" htmlFor="cfm">{t('mdp.champConfirmer')}</label>
              <input id="cfm" type="password" className="input" autoComplete="new-password"
                     value={confirm} onChange={e => setConfirm(e.target.value)} />
              {confirm !== '' && !matches && (
                <p className="text-xs text-danger-500 mt-1">{t('mdp.saisiesDifferent')}</p>
              )}
              {next !== '' && !different && (
                <p className="text-xs text-danger-500 mt-1">
                  {t('mdp.doitEtreDifferent')}
                </p>
              )}
            </div>

            {error && <ErrorBanner message={error} />}

            <button
              onClick={() => { setError(null); change.mutate() }}
              disabled={!strong || !matches || !different || current === '' || change.isPending}
              className="btn-primary w-full flex items-center justify-center gap-2"
            >
              {change.isPending && <Loader2 size={14} className="animate-spin" />}
              {t('mdp.changerBouton')}
            </button>

            <p className="text-xs text-alpine-500">
              {t('mdp.autresSessions')}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
