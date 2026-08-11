// Comptes et rôles — Paramètres → Sécurité.
//
// Le cas central : donner un accès à sa fiduciaire sans lui donner les clés.
//
// Cet écran ne décide de rien. Les droits se vérifient dans la base à chaque
// requête, et le serveur refuse quoi qu'affiche le navigateur. Ce qu'il fait,
// c'est éviter de proposer un bouton qui répondra 403 — un bouton qui échoue
// use la confiance dans l'interface aussi sûrement qu'un avertissement périmé.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  UserPlus, ShieldCheck, Eye, Calculator, Loader2, UserX, UserCheck,
  KeyRound, Smartphone, Copy, Check, AlertTriangle,
} from 'lucide-react'
import { usersApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import type { AppUser, UserRole } from '@/types'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

// Les rôles sont décrits par des CLÉS : cette table est construite au
// chargement du module, où `useT()` n'existe pas encore.
const ROLES: Array<{ v: UserRole; cle: Cle; icon: typeof Eye; desc: Cle }> = [
  { v: 'admin',      cle: 'role.admin',     icon: ShieldCheck, desc: 'us.roleAdminDesc' },
  { v: 'accountant', cle: 'role.comptable', icon: Calculator,  desc: 'us.roleComptableDesc' },
  { v: 'viewer',     cle: 'role.lecture',   icon: Eye,         desc: 'us.roleLectureDesc' },
]

export function UsersPanel() {
  const t = useT()
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [form, setForm] = useState({ name: '', email: '', password: '', role: 'viewer' as UserRole })

  // Le mot de passe temporaire ne s'affiche qu'une fois, ici, et n'est jamais
  // conservé : ni en base sous forme lisible, ni dans le journal de sécurité —
  // celui-ci est consultable, exportable et sauvegardé.
  const [issued, setIssued] = useState<{ email: string; password: string } | null>(null)
  const [copied, setCopied] = useState(false)
  const [confirmReset, setConfirmReset] = useState<AppUser | null>(null)
  const [confirmMfa, setConfirmMfa] = useState<AppUser | null>(null)

  const users = useQuery<{ items: AppUser[] }>({
    queryKey: ['users'],
    queryFn:  () => usersApi.list().then(r => r.data),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] })
  const fail = (fallback: string) => (e: unknown) => setError(refusalMessage(e, fallback))

  const create = useMutation({
    mutationFn: () => usersApi.create(form),
    onSuccess: () => {
      setError(null); setCreating(false)
      setForm({ name: '', email: '', password: '', role: 'viewer' })
      invalidate()
    },
    onError: fail(t('us.echecCreation')),
  })

  const setRole = useMutation({
    mutationFn: (v: { id: string; role: UserRole }) => usersApi.setRole(v.id, v.role),
    onSuccess: () => { setError(null); invalidate() },
    onError: fail(t('us.echecRole')),
  })

  const setActive = useMutation({
    mutationFn: (v: { id: string; active: boolean }) => usersApi.setActive(v.id, v.active),
    onSuccess: () => { setError(null); invalidate() },
    onError: fail(t('us.echecEtat')),
  })

  const reset = useMutation({
    mutationFn: (u: AppUser) => usersApi.resetPassword(u.id).then(r => ({ u, data: r.data })),
    onSuccess: ({ u, data }) => {
      setError(null); setConfirmReset(null)
      setIssued({ email: u.email, password: data.temporary_password })
      invalidate()
    },
    onError: (e) => { setConfirmReset(null); fail(t('us.echecReinit'))(e) },
  })

  const removeMfa = useMutation({
    mutationFn: (u: AppUser) => usersApi.removeMfa(u.id),
    onSuccess: () => { setError(null); setConfirmMfa(null); invalidate() },
    onError: (e) => { setConfirmMfa(null); fail(t('us.echecMfa'))(e) },
  })

  const copyPassword = async () => {
    if (!issued) return
    try {
      await navigator.clipboard.writeText(issued.password)
      setCopied(true)
      setTimeout(() => setCopied(false), 2500)
    } catch {
      setError(t('us.echecCopie'))
    }
  }

  const items = users.data?.items ?? []
  const passwordOK = form.password.length >= 8
  const canCreate = form.name.trim() !== '' && form.email.includes('@') && passwordOK

  return (
    <div className="mt-6">
      <SectionTitle>{t('us.comptesEtRoles')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        {t('us.introduction')}
      </p>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}
      {users.isLoading && <LoadingSpinner />}
      {users.isError && <ErrorBanner message={t('us.erreurListe')} />}

      {items.length > 0 && (
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr>
                <th>{t('us.colNom')}</th><th>{t('us.colEmail')}</th>
                <th>{t('us.colRole')}</th><th>{t('us.colEtat')}</th><th />
              </tr>
            </thead>
            <tbody>
              {items.map(u => (
                <tr key={u.id} className={u.is_active ? '' : 'opacity-60'}>
                  <td>{u.name}</td>
                  <td className="text-alpine-600">{u.email}</td>
                  <td>
                    <select
                      className="select select-sm"
                      value={u.role}
                      disabled={setRole.isPending}
                      onChange={e => setRole.mutate({ id: u.id, role: e.target.value as UserRole })}
                    >
                      {ROLES.map(r => <option key={r.v} value={r.v}>{t(r.cle)}</option>)}
                    </select>
                  </td>
                  <td>
                    {u.is_active
                      ? <span className="text-success-700">{t('us.actif')}</span>
                      : <span className="text-alpine-500">{t('us.desactive')}</span>}
                  </td>
                  <td className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      {u.is_active && (
                        <button
                          onClick={() => { setError(null); setIssued(null); setConfirmReset(u) }}
                          disabled={reset.isPending}
                          title={t('us.infoBulleReinit')}
                          className="btn-ghost btn-sm flex items-center gap-1"
                        >
                          <KeyRound size={13} /> {t('us.reinitialiser')}
                        </button>
                      )}
                      <button
                        onClick={() => { setError(null); setConfirmMfa(u) }}
                        disabled={removeMfa.isPending}
                        title={t('us.infoBulleMfa')}
                        className="btn-ghost btn-sm flex items-center gap-1"
                      >
                        <Smartphone size={13} /> {t('us.secondFacteur')}
                      </button>
                      <button
                        onClick={() => setActive.mutate({ id: u.id, active: !u.is_active })}
                        disabled={setActive.isPending}
                        className="btn-ghost btn-sm flex items-center gap-1"
                      >
                        {u.is_active
                          ? <><UserX size={13} /> {t('us.desactiver')}</>
                          : <><UserCheck size={13} /> {t('us.reactiver')}</>}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Le mot de passe temporaire, montré une seule fois.
          Il n'entre ni dans la base sous forme lisible, ni dans le journal de
          sécurité : celui-ci est consultable, exportable et sauvegardé. */}
      {issued && (
        <div className="mt-4 rounded-md border border-warning-500 bg-warning-100 px-4 py-3">
          <p className="text-sm font-medium flex items-center gap-1.5">
            <AlertTriangle size={15} /> {t('us.motDePasseTemporaire', { email: issued.email })}
          </p>
          <p className="mt-2 font-mono text-base tracking-wider bg-white border border-neutral-200
                        rounded px-3 py-2 select-all">
            {issued.password}
          </p>
          <div className="mt-2 flex items-center gap-2">
            <button onClick={copyPassword} className="btn-secondary btn-sm flex items-center gap-1.5">
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? t('us.copie') : t('us.copier')}
            </button>
            <button onClick={() => setIssued(null)} className="btn-ghost btn-sm">
              {t('us.transmis')}
            </button>
          </div>
          <p className="text-xs text-alpine-600 mt-2">
            {t('us.transmisAide')}
          </p>
        </div>
      )}

      {/* Réinitialiser coupe l'accès en cours. Le dire avant, pas après. */}
      {confirmReset && (
        <div className="mt-4 rounded-md border border-neutral-300 bg-neutral-50 px-4 py-3">
          <p className="text-sm font-medium">
            {t('us.confirmerReinit', { nom: confirmReset.name, email: confirmReset.email })}
          </p>
          <ul className="mt-2 text-sm text-alpine-600 list-disc list-inside space-y-0.5">
            <li>{t('us.reinitCons1')}</li>
            <li>{t('us.reinitCons2')}</li>
            <li>{t('us.reinitCons3')}</li>
            <li>{t('us.reinitCons4')}</li>
          </ul>
          <p className="text-xs text-alpine-500 mt-2">
            {t('us.reinitSansMfa')}
          </p>
          <div className="mt-3 flex items-center gap-2">
            <button onClick={() => reset.mutate(confirmReset)} disabled={reset.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5">
              {reset.isPending && <Loader2 size={13} className="animate-spin" />}
              {t('us.reinitialiserAcces')}
            </button>
            <button onClick={() => setConfirmReset(null)} className="btn-ghost btn-sm">{t('action.annuler')}</button>
          </div>
        </div>
      )}

      {/* Retirer le second facteur affaiblit délibérément un compte : le geste
          doit être choisi, pas subi d'un clic malencontreux. */}
      {confirmMfa && (
        <div className="mt-4 rounded-md border border-danger-500 bg-danger-500/5 px-4 py-3">
          <p className="text-sm font-medium">
            {t('us.confirmerMfa', { nom: confirmMfa.name, email: confirmMfa.email })}
          </p>
          <p className="text-sm text-alpine-600 mt-1">
            {t('us.mfaAide')}
          </p>
          <div className="mt-3 flex items-center gap-2">
            <button onClick={() => removeMfa.mutate(confirmMfa)} disabled={removeMfa.isPending}
                    className="btn-danger btn-sm flex items-center gap-1.5">
              {removeMfa.isPending && <Loader2 size={13} className="animate-spin" />}
              {t('us.retirerSecondFacteur')}
            </button>
            <button onClick={() => setConfirmMfa(null)} className="btn-ghost btn-sm">{t('action.annuler')}</button>
          </div>
        </div>
      )}

      {/* Un compte n'est jamais supprimé. Les écritures et les documents portent
          l'identifiant de leur auteur : effacer la ligne casserait la
          traçabilité que le CO art. 957a al. 2 ch. 5 exige. */}
      <p className="text-xs text-alpine-500 mt-2">
        {t('us.pasDeSuppression')}
      </p>

      {!creating ? (
        <button onClick={() => { setError(null); setCreating(true) }}
                className="btn-secondary btn-sm flex items-center gap-1.5 mt-4">
          <UserPlus size={14} /> {t('us.ajouterCompte')}
        </button>
      ) : (
        <div className="mt-4 rounded-md border border-neutral-200 px-4 py-3 space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="label" htmlFor="u-name">{t('us.colNom')}</label>
              <input id="u-name" className="input" value={form.name}
                     onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
            <div>
              <label className="label" htmlFor="u-email">{t('us.colEmail')}</label>
              <input id="u-email" type="email" className="input" value={form.email}
                     onChange={e => setForm({ ...form, email: e.target.value })} />
            </div>
          </div>

          <div>
            <label className="label" htmlFor="u-pass">{t('securite.motDePasse')}</label>
            <input id="u-pass" type="password" className="input" autoComplete="new-password"
                   value={form.password}
                   onChange={e => setForm({ ...form, password: e.target.value })} />
            <p className={`text-xs mt-1 ${passwordOK ? 'text-success-700' : 'text-alpine-500'}`}>
              {t('us.motDePasseAide')}
            </p>
          </div>

          <div>
            <span className="label">{t('us.colRole')}</span>
            <div className="space-y-2 mt-1">
              {ROLES.map(r => (
                <label key={r.v} className={`flex items-start gap-2 rounded-md border px-3 py-2 cursor-pointer ${
                  form.role === r.v ? 'border-accent-700 bg-accent-100/40' : 'border-neutral-200'
                }`}>
                  <input type="radio" name="role" className="mt-1" checked={form.role === r.v}
                         onChange={() => setForm({ ...form, role: r.v })} />
                  <span>
                    <span className="text-sm font-medium flex items-center gap-1.5">
                      <r.icon size={14} /> {t(r.cle)}
                    </span>
                    <span className="block text-xs text-alpine-600 mt-0.5">{t(r.desc)}</span>
                  </span>
                </label>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-2">
            <button onClick={() => create.mutate()} disabled={!canCreate || create.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5">
              {create.isPending && <Loader2 size={13} className="animate-spin" />}
              {t('us.creerCompte')}
            </button>
            <button onClick={() => { setCreating(false); setError(null) }} className="btn-ghost btn-sm">
              {t('action.annuler')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
