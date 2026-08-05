// Second facteur du compte connecté — Paramètres → Sécurité.
//
// Obligatoire pour un administrateur, proposé aux autres. Ce que le second
// facteur protège : le cas où le MOT DE PASSE fuit — réutilisé ailleurs, deviné,
// lu par-dessus l'épaule. Il ne protège pas de quelqu'un qui lit déjà le fichier
// de base ; celui-là n'a besoin d'aucun code, et c'est le chiffrement de la base
// et du disque qui répond à cette menace.
//
// Le dire évite de croire couvert un risque qui ne l'est pas — ce qui est
// exactement la façon dont on cesse de faire confiance aux avertissements.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Smartphone, ShieldCheck, Loader2, Copy, Check, AlertTriangle, ShieldOff,
} from 'lucide-react'
import { authApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'

interface Status {
  enabled: boolean
  recovery_codes_left: number
  required_for_this_role: boolean
}

export function MFAPanel() {
  const qc = useQueryClient()
  const [setup, setSetup] = useState<{ secret: string; qr_png: string } | null>(null)
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)
  const [copied, setCopied] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  const status = useQuery<Status>({
    queryKey: ['mfa-status'],
    queryFn:  () => authApi.mfaStatus().then(r => r.data),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['mfa-status'] })
  const fail = (fallback: string) => (e: unknown) => setError(refusalMessage(e, fallback))

  const start = useMutation({
    mutationFn: () => authApi.mfaSetup(),
    onSuccess: (r) => { setError(null); setSetup(r.data); setCodes(null) },
    onError: fail("L'inscription n'a pas pu démarrer."),
  })

  const confirm = useMutation({
    mutationFn: () => authApi.mfaConfirm(code.trim()),
    onSuccess: (r) => {
      setError(null); setSetup(null); setCode('')
      setCodes((r.data.recovery_codes ?? []) as string[])
      invalidate()
    },
    onError: (e) => { setCode(''); fail("Le code n'a pas été accepté.")(e) },
  })

  const disable = useMutation({
    mutationFn: () => authApi.mfaDisable(password),
    onSuccess: () => { setError(null); setRemoving(false); setPassword(''); invalidate() },
    onError: fail('Le second facteur n’a pas pu être retiré.'),
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

  const st = status.data

  return (
    <div className="mt-6">
      <SectionTitle>Second facteur (code à usage unique)</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        Un code de six chiffres, calculé par votre téléphone, en plus du mot de passe. Il protège
        du cas où votre mot de passe fuiterait. Il ne protège pas d'un accès direct au fichier de
        base : c'est le rôle du chiffrement de la base et du disque.
      </p>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}
      {status.isLoading && <LoadingSpinner />}
      {status.isError && <ErrorBanner message="L'état du second facteur n'a pas pu être lu." />}

      {/* Les codes de secours, montrés une seule fois. */}
      {codes && (
        <div className="mb-4 rounded-md border border-warning-500 bg-warning-100 px-4 py-3">
          <p className="text-sm font-medium flex items-center gap-1.5">
            <AlertTriangle size={15} /> Notez ces codes de secours maintenant
          </p>
          <p className="text-sm text-alpine-700 mt-1">
            Ils ne seront plus jamais affichés. Sans eux, un téléphone perdu vous ferme
            définitivement l'accès.
          </p>
          <ul className="mt-3 grid grid-cols-2 sm:grid-cols-3 gap-2 font-mono text-sm">
            {codes.map(c => (
              <li key={c} className="rounded border border-neutral-200 bg-white px-2 py-1.5
                                     text-center tracking-wider select-all">
                {c}
              </li>
            ))}
          </ul>
          <div className="mt-3 flex items-center gap-2">
            <button onClick={copyCodes} className="btn-secondary btn-sm flex items-center gap-1.5">
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? 'Copié' : 'Copier les codes'}
            </button>
            <button onClick={() => setCodes(null)} className="btn-ghost btn-sm">
              Je les ai notés
            </button>
          </div>
        </div>
      )}

      {st && !setup && (
        <div className="rounded-md border border-neutral-200 px-4 py-3">
          <p className="text-sm flex items-center gap-2">
            {st.enabled
              ? <><ShieldCheck size={16} className="text-success-700" />
                  <span><strong>Actif</strong> sur ce compte.
                    {' '}{st.recovery_codes_left} code(s) de secours encore utilisable(s).</span></>
              : <><ShieldOff size={16} className="text-alpine-500" />
                  <span><strong>Inactif</strong> : seul votre mot de passe protège ce compte.</span></>}
          </p>

          {st.required_for_this_role && !st.enabled && (
            <p className="text-sm text-danger-700 mt-2">
              Ce compte est administrateur : le second facteur est exigé. Tant qu'il n'est pas
              inscrit, aucune autre action n'est possible.
            </p>
          )}

          {st.enabled && st.recovery_codes_left <= 2 && (
            <p className="text-sm text-warning-700 mt-2">
              Il vous reste peu de codes de secours. Réinscrivez votre application pour en obtenir
              une nouvelle série.
            </p>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-2">
            {!st.enabled && (
              <button onClick={() => start.mutate()} disabled={start.isPending}
                      className="btn-primary btn-sm flex items-center gap-1.5">
                {start.isPending && <Loader2 size={13} className="animate-spin" />}
                <Smartphone size={13} /> Inscrire mon téléphone
              </button>
            )}
            {st.enabled && !removing && (
              <button onClick={() => { setError(null); setRemoving(true) }}
                      className="btn-ghost btn-sm flex items-center gap-1.5">
                <ShieldOff size={13} /> Retirer le second facteur
              </button>
            )}
          </div>

          {/* Le mot de passe est redemandé : sans cela, un poste laissé ouvert
              suffirait à désactiver la protection. */}
          {removing && (
            <div className="mt-3 rounded-md border border-danger-500 bg-danger-500/5 px-3 py-3">
              <p className="text-sm">
                Confirmez avec votre mot de passe. Le compte reviendra au mot de passe seul.
                {st.required_for_this_role && ' Étant administrateur, vous devrez en inscrire ' +
                  'un nouveau avant de pouvoir travailler — c’est le chemin à suivre pour ' +
                  'changer de téléphone.'}
              </p>
              <input type="password" className="input mt-2" autoComplete="current-password"
                     placeholder="Votre mot de passe"
                     value={password} onChange={e => setPassword(e.target.value)} />
              <div className="mt-2 flex items-center gap-2">
                <button onClick={() => disable.mutate()}
                        disabled={password === '' || disable.isPending}
                        className="btn-danger btn-sm flex items-center gap-1.5">
                  {disable.isPending && <Loader2 size={13} className="animate-spin" />}
                  Retirer
                </button>
                <button onClick={() => { setRemoving(false); setPassword('') }}
                        className="btn-ghost btn-sm">Annuler</button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* Assistant d'inscription. Rien n'est actif tant qu'un premier code n'a
          pas été validé : abandonner ici n'enferme personne. */}
      {setup && (
        <div className="rounded-md border border-neutral-200 px-4 py-4">
          <p className="text-sm text-alpine-700">
            Scannez ce code avec votre application d'authentification, puis saisissez celui
            qu'elle affiche. Aegis, KeePassXC et FreeOTP sont libres et fonctionnent hors ligne.
          </p>
          <div className="mt-3 flex flex-col sm:flex-row gap-4 items-start">
            <img src={setup.qr_png} alt="Code à scanner"
                 className="rounded border border-neutral-200 bg-white p-2" width={200} height={200} />
            <div className="flex-1 w-full">
              <details>
                <summary className="text-xs text-alpine-600 cursor-pointer">
                  Impossible de scanner ? Saisir la clé à la main
                </summary>
                <p className="mt-1.5 font-mono text-xs break-all rounded border border-neutral-200
                              bg-neutral-50 px-2 py-1.5 select-all">
                  {setup.secret}
                </p>
              </details>

              <label className="label mt-3" htmlFor="mfa-code">Code affiché</label>
              <input id="mfa-code" inputMode="numeric" maxLength={7}
                     className="input text-center text-lg tracking-[0.35em]"
                     placeholder="000000"
                     value={code} onChange={e => setCode(e.target.value)}
                     onKeyDown={e => { if (e.key === 'Enter' && code.trim().length >= 6) confirm.mutate() }} />

              <div className="mt-3 flex items-center gap-2">
                <button onClick={() => confirm.mutate()}
                        disabled={code.trim().length < 6 || confirm.isPending}
                        className="btn-primary btn-sm flex items-center gap-1.5">
                  {confirm.isPending && <Loader2 size={13} className="animate-spin" />}
                  Activer
                </button>
                <button onClick={() => { setSetup(null); setCode(''); setError(null) }}
                        className="btn-ghost btn-sm">Annuler</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
