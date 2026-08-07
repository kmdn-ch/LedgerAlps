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

  const reset = useMutation({
    mutationFn: (u: AppUser) => usersApi.resetPassword(u.id).then(r => ({ u, data: r.data })),
    onSuccess: ({ u, data }) => {
      setError(null); setConfirmReset(null)
      setIssued({ email: u.email, password: data.temporary_password })
      invalidate()
    },
    onError: (e) => { setConfirmReset(null); fail("L'accès n'a pas pu être réinitialisé.")(e) },
  })

  const removeMfa = useMutation({
    mutationFn: (u: AppUser) => usersApi.removeMfa(u.id),
    onSuccess: () => { setError(null); setConfirmMfa(null); invalidate() },
    onError: (e) => { setConfirmMfa(null); fail("Le second facteur n'a pas pu être retiré.")(e) },
  })

  const copyPassword = async () => {
    if (!issued) return
    try {
      await navigator.clipboard.writeText(issued.password)
      setCopied(true)
      setTimeout(() => setCopied(false), 2500)
    } catch {
      setError('La copie a échoué. Recopiez le mot de passe à la main.')
    }
  }

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
                    <div className="flex items-center justify-end gap-1">
                      {u.is_active && (
                        <button
                          onClick={() => { setError(null); setIssued(null); setConfirmReset(u) }}
                          disabled={reset.isPending}
                          title="Remplacer le mot de passe par un mot de passe temporaire"
                          className="btn-ghost btn-sm flex items-center gap-1"
                        >
                          <KeyRound size={13} /> Réinitialiser
                        </button>
                      )}
                      <button
                        onClick={() => { setError(null); setConfirmMfa(u) }}
                        disabled={removeMfa.isPending}
                        title="Retirer le second facteur (application 2FA/OTP perdue)"
                        className="btn-ghost btn-sm flex items-center gap-1"
                      >
                        <Smartphone size={13} /> Second facteur
                      </button>
                      <button
                        onClick={() => setActive.mutate({ id: u.id, active: !u.is_active })}
                        disabled={setActive.isPending}
                        className="btn-ghost btn-sm flex items-center gap-1"
                      >
                        {u.is_active ? <><UserX size={13} /> Désactiver</> : <><UserCheck size={13} /> Réactiver</>}
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
            <AlertTriangle size={15} /> Mot de passe temporaire pour {issued.email}
          </p>
          <p className="mt-2 font-mono text-base tracking-wider bg-white border border-neutral-200
                        rounded px-3 py-2 select-all">
            {issued.password}
          </p>
          <div className="mt-2 flex items-center gap-2">
            <button onClick={copyPassword} className="btn-secondary btn-sm flex items-center gap-1.5">
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {copied ? 'Copié' : 'Copier'}
            </button>
            <button onClick={() => setIssued(null)} className="btn-ghost btn-sm">
              J'ai transmis le mot de passe
            </button>
          </div>
          <p className="text-xs text-alpine-600 mt-2">
            Il ne s'affichera plus. Transmettez-le de vive voix plutôt que par message : il sera
            remplacé à la première connexion, mais tant qu'il vaut, il ouvre le compte.
          </p>
        </div>
      )}

      {/* Réinitialiser coupe l'accès en cours. Le dire avant, pas après. */}
      {confirmReset && (
        <div className="mt-4 rounded-md border border-neutral-300 bg-neutral-50 px-4 py-3">
          <p className="text-sm font-medium">
            Réinitialiser l'accès de {confirmReset.name} ({confirmReset.email}) ?
          </p>
          <ul className="mt-2 text-sm text-alpine-600 list-disc list-inside space-y-0.5">
            <li>Son mot de passe actuel est <strong>détruit</strong> — personne ne peut le lire.</li>
            <li>Un mot de passe temporaire s'affichera ici, une seule fois.</li>
            <li>Ses sessions ouvertes sont fermées immédiatement.</li>
            <li>Il devra choisir son propre mot de passe avant de pouvoir faire quoi que ce soit.</li>
          </ul>
          <p className="text-xs text-alpine-500 mt-2">
            Son second facteur, s'il en a un, n'est pas touché : c'est une action séparée.
          </p>
          <div className="mt-3 flex items-center gap-2">
            <button onClick={() => reset.mutate(confirmReset)} disabled={reset.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5">
              {reset.isPending && <Loader2 size={13} className="animate-spin" />}
              Réinitialiser l'accès
            </button>
            <button onClick={() => setConfirmReset(null)} className="btn-ghost btn-sm">Annuler</button>
          </div>
        </div>
      )}

      {/* Retirer le second facteur affaiblit délibérément un compte : le geste
          doit être choisi, pas subi d'un clic malencontreux. */}
      {confirmMfa && (
        <div className="mt-4 rounded-md border border-danger-500 bg-danger-500/5 px-4 py-3">
          <p className="text-sm font-medium">
            Retirer le second facteur de {confirmMfa.name} ({confirmMfa.email}) ?
          </p>
          <p className="text-sm text-alpine-600 mt-1">
            À faire quand son téléphone est perdu et que ses codes de secours sont épuisés. Le
            compte revient au mot de passe seul, et ses sessions ouvertes sont fermées. Si c'est un
            administrateur, il devra inscrire un nouveau téléphone avant de pouvoir travailler.
          </p>
          <div className="mt-3 flex items-center gap-2">
            <button onClick={() => removeMfa.mutate(confirmMfa)} disabled={removeMfa.isPending}
                    className="btn-danger btn-sm flex items-center gap-1.5">
              {removeMfa.isPending && <Loader2 size={13} className="animate-spin" />}
              Retirer le second facteur
            </button>
            <button onClick={() => setConfirmMfa(null)} className="btn-ghost btn-sm">Annuler</button>
          </div>
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
