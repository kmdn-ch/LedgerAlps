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
import { useAuthStore } from '@/store/auth'
import { useT } from '@/i18n/useT'

interface Status {
  enabled: boolean
  recovery_codes_left: number
  required_for_this_role: boolean
}

export function MFAPanel() {
  const t = useT()
  const qc = useQueryClient()
  const role = useAuthStore(st => st.role)
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
    onError: fail(t('mf.echecDemarrage')),
  })

  const confirm = useMutation({
    mutationFn: () => authApi.mfaConfirm(code.trim()),
    onSuccess: (r) => {
      setError(null); setSetup(null); setCode('')
      setCodes((r.data.recovery_codes ?? []) as string[])
      invalidate()
    },
    onError: (e) => { setCode(''); fail(t('mf.codeRefuse'))(e) },
  })

  const disable = useMutation({
    mutationFn: () => authApi.mfaDisable(password),
    onSuccess: () => { setError(null); setRemoving(false); setPassword(''); invalidate() },
    onError: fail(t('us.echecMfa')),
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

  const st = status.data

  // Le message nommait « administrateur » quel que soit le rôle : un comptable
  // lisait donc une phrase fausse sur son propre écran, ce qui use la confiance
  // dans tout ce que le produit affirme par ailleurs.
  const roleLabel = t(role === 'admin' ? 'mp.roleAdmin'
    : role === 'accountant' ? 'mp.roleComptable'
    : 'mp.roleLecture')

  return (
    <div className="mt-6">
      <SectionTitle>{t('mp.titre')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        {t('mp.introduction')}
      </p>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}
      {status.isLoading && <LoadingSpinner />}
      {status.isError && <ErrorBanner message={t('mp.erreurEtat')} />}

      {/* Les codes de secours, montrés une seule fois. */}
      {codes && (
        <div className="mb-4 rounded-md border border-warning-500 bg-warning-100 px-4 py-3">
          <p className="text-sm font-medium flex items-center gap-1.5">
            <AlertTriangle size={15} /> {t('mp.notezCodes')}
          </p>
          <p className="text-sm text-alpine-700 mt-1">
            {t('mp.notezCodesAide')}
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
              {copied ? t('us.copie') : t('mf.copierCodes')}
            </button>
            <button onClick={() => setCodes(null)} className="btn-ghost btn-sm">
              {t('mp.jeLesAiNotes')}
            </button>
          </div>
        </div>
      )}

      {st && !setup && (
        <div className="rounded-md border border-neutral-200 px-4 py-3">
          <p className="text-sm flex items-center gap-2">
            {st.enabled
              ? <><ShieldCheck size={16} className="text-success-700" />
                  <span>{t('mp.actif', { n: st.recovery_codes_left })}</span></>
              : <><ShieldOff size={16} className="text-alpine-500" />
                  <span>{t('mp.inactif')}</span></>}
          </p>

          {st.required_for_this_role && !st.enabled && (
            <p className="text-sm text-danger-700 mt-2">
              {t('mp.exigePourRole', { role: roleLabel })}
            </p>
          )}

          {st.enabled && st.recovery_codes_left <= 2 && (
            <p className="text-sm text-warning-700 mt-2">
              {t('mp.peuDeCodes')}
            </p>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-2">
            {!st.enabled && (
              <button onClick={() => start.mutate()} disabled={start.isPending}
                      className="btn-primary btn-sm flex items-center gap-1.5">
                {start.isPending && <Loader2 size={13} className="animate-spin" />}
                <Smartphone size={13} /> {t('mp.activer2FA')}
              </button>
            )}
            {st.enabled && !removing && (
              <button onClick={() => { setError(null); setRemoving(true) }}
                      className="btn-ghost btn-sm flex items-center gap-1.5">
                <ShieldOff size={13} /> {t('mp.retirer')}
              </button>
            )}
          </div>

          {/* Le mot de passe est redemandé : sans cela, un poste laissé ouvert
              suffirait à désactiver la protection. */}
          {removing && (
            <div className="mt-3 rounded-md border border-danger-500 bg-danger-500/5 px-3 py-3">
              <p className="text-sm">
                {t('mp.confirmezMotDePasse')}
                {st.required_for_this_role
                  && ' ' + t('mp.devrezReactiver', { role: roleLabel })}
              </p>
              <input type="password" className="input mt-2" autoComplete="current-password"
                     placeholder={t('mp.placeholderMotDePasse')}
                     value={password} onChange={e => setPassword(e.target.value)} />
              <div className="mt-2 flex items-center gap-2">
                <button onClick={() => disable.mutate()}
                        disabled={password === '' || disable.isPending}
                        className="btn-danger btn-sm flex items-center gap-1.5">
                  {disable.isPending && <Loader2 size={13} className="animate-spin" />}
                  {t('mp.retirerBouton')}
                </button>
                <button onClick={() => { setRemoving(false); setPassword('') }}
                        className="btn-ghost btn-sm">{t('action.annuler')}</button>
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
            {t('mp.scannezAide')}
          </p>
          <div className="mt-3 flex flex-col sm:flex-row gap-4 items-start">
            <img src={setup.qr_png} alt={t('mf.altCodeAScanner')}
                 className="rounded border border-neutral-200 bg-white p-2" width={200} height={200} />
            <div className="flex-1 w-full">
              <details>
                <summary className="text-xs text-alpine-600 cursor-pointer">
                  {t('mf.impossibleScanner')}
                </summary>
                <p className="mt-1.5 font-mono text-xs break-all rounded border border-neutral-200
                              bg-neutral-50 px-2 py-1.5 select-all">
                  {setup.secret}
                </p>
              </details>

              <label className="label mt-3" htmlFor="mfa-code">{t('mp.codeAffiche')}</label>
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
                  {t('mp.activer')}
                </button>
                <button onClick={() => { setSetup(null); setCode(''); setError(null) }}
                        className="btn-ghost btn-sm">{t('action.annuler')}</button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
