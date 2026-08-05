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
import { UserPlus, ShieldCheck, Eye, Calculator, Loader2, UserX, UserCheck } from 'lucide-react'
import { usersApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import type { AppUser, UserRole } from '@/types'

const ROLES: Array<{ v: UserRole; label: string; icon: typeof Eye; desc: string }> = [
  {
    v: 'admin', label: 'Administrateur', icon: ShieldCheck,
    desc: "Tout, y compris les comptes, les sauvegardes et la sécurité.",
  },
  {
    v: 'accountant', label: 'Comptable', icon: Calculator,
    desc: "Tient les livres. Ne touche ni aux comptes, ni aux sauvegardes, ni à la sécurité.",
  },
  {
    v: 'viewer', label: 'Lecture seule', icon: Eye,
    desc: "Consulte et exporte, n'écrit rien. C'est le rôle de votre fiduciaire.",
  },
]

export function UsersPanel() {
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [form, setForm] = useState({ name: '', email: '', password: '', role: 'viewer' as UserRole })

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
    onError: fail("Le compte n'a pas pu être créé."),
  })

  const setRole = useMutation({
    mutationFn: (v: { id: string; role: UserRole }) => usersApi.setRole(v.id, v.role),
    onSuccess: () => { setError(null); invalidate() },
    onError: fail("Le rôle n'a pas pu être changé."),
  })

  const setActive = useMutation({
    mutationFn: (v: { id: string; active: boolean }) => usersApi.setActive(v.id, v.active),
    onSuccess: () => { setError(null); invalidate() },
    onError: fail("L'état du compte n'a pas pu être changé."),
  })

  const items = users.data?.items ?? []
  const passwordOK = form.password.length >= 8
  const canCreate = form.name.trim() !== '' && form.email.includes('@') && passwordOK

  return (
    <div className="mt-6">
      <SectionTitle>Comptes et rôles</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        Donnez un accès à votre fiduciaire sans partager votre compte. Un rôle change
        <strong> immédiatement</strong> : les droits sont relus à chaque requête, sans attendre
        l'expiration d'une session.
      </p>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}
      {users.isLoading && <LoadingSpinner />}
      {users.isError && <ErrorBanner message="La liste des comptes n'a pas pu être lue." />}

      {items.length > 0 && (
        <div className="overflow-x-auto">
          <table className="table">
            <thead>
              <tr><th>Nom</th><th>Adresse e-mail</th><th>Rôle</th><th>État</th><th /></tr>
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
                      {ROLES.map(r => <option key={r.v} value={r.v}>{r.label}</option>)}
                    </select>
                  </td>
                  <td>
                    {u.is_active
                      ? <span className="text-success-700">Actif</span>
                      : <span className="text-alpine-500">Désactivé</span>}
                  </td>
                  <td className="text-right">
                    <button
                      onClick={() => setActive.mutate({ id: u.id, active: !u.is_active })}
                      disabled={setActive.isPending}
                      className="btn-ghost btn-sm flex items-center gap-1 ml-auto"
                    >
                      {u.is_active ? <><UserX size={13} /> Désactiver</> : <><UserCheck size={13} /> Réactiver</>}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Un compte n'est jamais supprimé. Les écritures et les documents portent
          l'identifiant de leur auteur : effacer la ligne casserait la
          traçabilité que le CO art. 957a al. 2 ch. 5 exige. */}
      <p className="text-xs text-alpine-500 mt-2">
        Un compte se désactive, il ne se supprime pas : les écritures et les documents portent le
        nom de leur auteur, et l'effacer romprait la traçabilité que le CO art. 957a exige. Un
        compte désactivé ne peut plus rien faire, y compris avec une session déjà ouverte.
      </p>

      {!creating ? (
        <button onClick={() => { setError(null); setCreating(true) }}
                className="btn-secondary btn-sm flex items-center gap-1.5 mt-4">
          <UserPlus size={14} /> Ajouter un compte
        </button>
      ) : (
        <div className="mt-4 rounded-md border border-neutral-200 px-4 py-3 space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="label" htmlFor="u-name">Nom</label>
              <input id="u-name" className="input" value={form.name}
                     onChange={e => setForm({ ...form, name: e.target.value })} />
            </div>
            <div>
              <label className="label" htmlFor="u-email">Adresse e-mail</label>
              <input id="u-email" type="email" className="input" value={form.email}
                     onChange={e => setForm({ ...form, email: e.target.value })} />
            </div>
          </div>

          <div>
            <label className="label" htmlFor="u-pass">Mot de passe</label>
            <input id="u-pass" type="password" className="input" autoComplete="new-password"
                   value={form.password}
                   onChange={e => setForm({ ...form, password: e.target.value })} />
            <p className={`text-xs mt-1 ${passwordOK ? 'text-success-700' : 'text-alpine-500'}`}>
              Au moins 8 caractères. Transmettez-le à la personne par un autre canal que
              l'e-mail où vous lui annoncez son accès.
            </p>
          </div>

          <div>
            <span className="label">Rôle</span>
            <div className="space-y-2 mt-1">
              {ROLES.map(r => (
                <label key={r.v} className={`flex items-start gap-2 rounded-md border px-3 py-2 cursor-pointer ${
                  form.role === r.v ? 'border-accent-700 bg-accent-100/40' : 'border-neutral-200'
                }`}>
                  <input type="radio" name="role" className="mt-1" checked={form.role === r.v}
                         onChange={() => setForm({ ...form, role: r.v })} />
                  <span>
                    <span className="text-sm font-medium flex items-center gap-1.5">
                      <r.icon size={14} /> {r.label}
                    </span>
                    <span className="block text-xs text-alpine-600 mt-0.5">{r.desc}</span>
                  </span>
                </label>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-2">
            <button onClick={() => create.mutate()} disabled={!canCreate || create.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5">
              {create.isPending && <Loader2 size={13} className="animate-spin" />}
              Créer le compte
            </button>
            <button onClick={() => { setCreating(false); setError(null) }} className="btn-ghost btn-sm">
              Annuler
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
