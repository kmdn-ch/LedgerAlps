// LedgerAlps — Sauvegardes (onglet Paramètres)
//
// Deux verbes qui ne se comportent pas pareil, et l'interface doit le montrer :
// créer une sauvegarde est immédiat, restaurer demande de redémarrer
// l'application. Une restauration remplace toute la comptabilité, donc elle
// passe par un avertissement explicite avant d'être préparée.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Download, RotateCcw, ShieldCheck, ShieldOff, AlertTriangle, X, Loader2, Check, Minus, RefreshCw,
  Eye, EyeOff,
} from 'lucide-react'
import { backupsApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner, EmptyState } from '@/components/ui'
import { formatDate } from '@/utils'
import { waitForShutdownThenGo } from '@/utils/restart'
import { refusalMessage } from '@/utils/refusal'
import type { BackupItem, PendingRestore, BackupPolicy } from '@/types'

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} Mo`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} Ko`
  return `${bytes} o`
}


// ─── Robustesse de la phrase de passe ─────────────────────────────────────────
//
// Cette phrase protège un fichier qu'un attaquant peut emporter et attaquer
// tranquillement : pas de limitation de tentatives, personne pour surveiller.
// Argon2id rend chaque essai coûteux, mais rien ne sauve une phrase courte —
// d'où la longueur en premier critère. Le serveur applique la même règle
// (internal/db/passphrase.go) ; ceci ne fait que la rendre visible pendant la
// frappe, au lieu de la révéler par un refus après coup.
const MIN_LEN = 16
const PASSPHRASE_EXAMPLE = '34CryPt3DB4ckup5@26'

interface Check { label: string; met: boolean }

function checksFor(p: string): Check[] {
  return [
    { label: `${MIN_LEN} caractères ou plus`, met: [...p].length >= MIN_LEN },
    { label: 'une minuscule',                 met: /\p{Ll}/u.test(p) },
    { label: 'une majuscule',                 met: /\p{Lu}/u.test(p) },
    { label: 'un chiffre',                    met: /\p{Nd}/u.test(p) },
  ]
}

// Le symbole n'est pas exigé mais renforce : il est présenté comme un bonus,
// pas comme un obstacle de plus.
function strengthOf(p: string): { score: number; label: string; className: string } {
  if (p === '') return { score: 0, label: '', className: '' }
  const met = checksFor(p).filter(c => c.met).length
  const bonus = /[^\p{L}\p{Nd}]/u.test(p) ? 1 : 0
  const long  = [...p].length >= 24 ? 1 : 0
  const score = met + bonus + long // 0..6

  if (met < 4)      return { score, label: 'Insuffisante', className: 'bg-danger-500' }
  if (score >= 6)   return { score, label: 'Excellente',   className: 'bg-success-700' }
  if (score === 5)  return { score, label: 'Solide',       className: 'bg-success-500' }
  return { score, label: 'Acceptable', className: 'bg-warning-500' }
}

function PassphraseStrength({ value }: { value: string }) {
  const checks = checksFor(value)
  const { score, label, className } = strengthOf(value)
  const allMet = checks.every(c => c.met)

  return (
    <div className="mt-2">
      <div className="flex items-center gap-2">
        <div className="flex-1 h-1.5 bg-alpine-100 rounded overflow-hidden">
          <div
            className={`h-full transition-all ${className}`}
            style={{ width: `${(score / 6) * 100}%` }}
          />
        </div>
        {label && (
          <span className={`text-xs font-medium ${allMet ? 'text-success-700' : 'text-danger-700'}`}>
            {label}
          </span>
        )}
      </div>

      <ul className="mt-2 space-y-0.5">
        {checks.map(c => (
          <li key={c.label} className={`text-xs flex items-center gap-1.5 ${
            c.met ? 'text-success-700' : 'text-alpine-500'
          }`}>
            {c.met ? <Check size={12} /> : <Minus size={12} />}
            {c.label}
          </li>
        ))}
      </ul>
    </div>
  )
}

export function BackupPanel() {
  const qc = useQueryClient()
  // La phrase de passe n'est demandée qu'après le clic : la laisser en
  // permanence dans la page invite à la saisir puis à s'en aller sans rien
  // créer, et elle traîne alors dans un champ de formulaire.
  const [creating, setCreating]       = useState(false)
  const [passphrase, setPassphrase]   = useState('')
  // La phrase repart masquée à chaque ouverture du dialogue : la laisser
  // visible d'une fois sur l'autre exposerait une saisie que personne n'a
  // demandé à montrer.
  const [showPassphrase, setShowPassphrase] = useState(false)
  const [confirming, setConfirming]   = useState<BackupItem | null>(null)
  const [restorePass, setRestorePass] = useState('')

  const { data, isLoading, error } = useQuery<{
    items: BackupItem[]; directory: string; pending_restore?: PendingRestore
  }>({
    queryKey: ['backups'],
    queryFn:  () => backupsApi.list().then(r => r.data),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['backups'] })

  const create = useMutation({
    mutationFn: (pass: string) => backupsApi.create(pass),
    onSuccess: () => { setCreating(false); setPassphrase(''); setShowPassphrase(false); invalidate() },
  })

  const closeCreate = () => { setCreating(false); setPassphrase(''); setShowPassphrase(false); create.reset() }

  const stage = useMutation({
    mutationFn: () => backupsApi.stageRestore(confirming!.name, restorePass),
    onSuccess: () => { setConfirming(null); setRestorePass(''); invalidate() },
  })

  const cancelRestore = useMutation({
    mutationFn: () => backupsApi.cancelRestore(),
    onSuccess: invalidate,
  })

  // Le serveur répond, puis se coupe et relance une copie de lui-même. On
  // attend qu'il réponde à nouveau avant de recharger : recharger trop tôt
  // afficherait une erreur de connexion là où tout se passe bien.
  // La politique sert ici aussi : le dialogue de creation doit dire que laisser
  // le champ vide utilisera la phrase enregistree, et non produire du clair.
  const storedPolicy = useQuery<BackupPolicy>({
    queryKey: ['backups', 'policy'],
    queryFn:  () => backupsApi.policy().then(r => r.data),
  }).data

  const [restarting, setRestarting] = useState(false)
  const restart = useMutation({
    mutationFn: async () => {
      await backupsApi.restart()
      setRestarting(true)
      // Le schéma ne change pas ici, mais la logique d'attente est partagée :
      // deux copies finiraient par diverger, et celle-ci a déjà été corrigée
      // une fois pour une raison qui vaut partout.
      await waitForShutdownThenGo(window.location.origin + '/')
    },
    onError: () => setRestarting(false),
  })

  if (isLoading) return <LoadingSpinner />
  if (error)     return <ErrorBanner message="Impossible de lire les sauvegardes." />

  const backups = data?.items ?? []
  const pending = data?.pending_restore

  return (
    <div className="space-y-5">
      <AutoBackupPolicy />

      {/* ── Restauration en attente ─────────────────────────────────────── */}
      {pending && (
        <div className="rounded-md border border-warning-500 bg-warning-100 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <AlertTriangle size={16} className="mt-0.5 flex-shrink-0 text-warning-700" />
            <div className="flex-1">
              <p className="font-medium">Restauration en attente de redémarrage</p>
              <p className="mt-1">
                La sauvegarde <strong>{pending.source_name}</strong> a été préparée et vérifiée.
                Elle remplacera la comptabilité actuelle au prochain démarrage :
                <strong> fermez puis rouvrez LedgerAlps</strong>.
              </p>
              <div className="flex items-center gap-2 mt-3">
                <button
                  onClick={() => restart.mutate()}
                  disabled={restart.isPending || restarting}
                  className="btn-primary btn-sm flex items-center gap-1.5"
                >
                  {(restart.isPending || restarting)
                    ? <Loader2 size={14} className="animate-spin" />
                    : <RefreshCw size={14} />}
                  {restarting ? 'Redémarrage…' : 'Redémarrer LedgerAlps maintenant'}
                </button>
                <button
                  onClick={() => cancelRestore.mutate()}
                  disabled={cancelRestore.isPending || restart.isPending || restarting}
                  className="btn-ghost btn-sm"
                >
                  Annuler cette restauration
                </button>
              </div>
              {restarting && (
                <p className="text-xs mt-2">
                  LedgerAlps applique la restauration et redémarre. Cette page se
                  rechargera d'elle-même — ne fermez pas la fenêtre.
                </p>
              )}
              {restart.isError && (
                <p className="text-xs text-danger-700 mt-2">
                  Le serveur n'a pas répondu à temps. La restauration reste préparée :
                  fermez puis rouvrez l'application pour l'appliquer.
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ── Créer une sauvegarde ────────────────────────────────────────── */}
      <div>
        <SectionTitle>Créer une sauvegarde</SectionTitle>
        <p className="text-sm text-alpine-600 mb-3">
          Une copie complète de votre comptabilité est écrite dans le dossier de
          sauvegarde. L'opération est sûre pendant que vous travaillez.
        </p>
        <button
          onClick={() => { create.reset(); setPassphrase(''); setShowPassphrase(false); setCreating(true) }}
          className="btn-primary btn-sm flex items-center gap-1.5"
        >
          <Download size={14} />
          Créer une sauvegarde
        </button>
        {create.isSuccess && !creating && (
          <p className="text-sm text-success-700 mt-3">Sauvegarde créée et vérifiée.</p>
        )}
      </div>

      {/* ── Sauvegardes existantes ──────────────────────────────────────── */}
      <div>
        <SectionTitle>Sauvegardes disponibles ({backups.length})</SectionTitle>
        {data?.directory && (
          <p className="text-xs text-alpine-500 mb-3 font-mono break-all">{data.directory}</p>
        )}

        {backups.length === 0 ? (
          <EmptyState
            icon={<Download size={28} />}
            title="Aucune sauvegarde"
            description="Créez-en une ci-dessus. Une sauvegarde automatique est aussi prise au démarrage."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Fichier</th><th>Date</th><th className="text-right">Taille</th>
                  <th>Chiffrement</th><th></th>
                </tr>
              </thead>
              <tbody>
                {backups.map(b => (
                  <tr key={b.name}>
                    <td className="font-mono text-xs">{b.name}</td>
                    <td className="text-alpine-600">{formatDate(b.created_at)}</td>
                    <td className="text-right tabular-nums">{formatSize(b.size_bytes)}</td>
                    <td>
                      {b.encrypted ? (
                        <span className="inline-flex items-center gap-1 text-success-700 text-xs">
                          <ShieldCheck size={13} /> Chiffrée
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-alpine-500 text-xs">
                          <ShieldOff size={13} /> En clair
                        </span>
                      )}
                    </td>
                    <td className="text-right">
                      <button
                        onClick={() => { setConfirming(b); setRestorePass('') }}
                        className="btn-ghost btn-sm flex items-center gap-1.5 ml-auto"
                      >
                        <RotateCcw size={13} /> Restaurer
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* ── Choix du chiffrement, après le clic ─────────────────────────── */}
      {creating && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="create-title"
        >
          <div className="bg-white rounded-lg shadow-xl max-w-lg w-full p-5">
            <div className="flex items-start justify-between mb-3">
              <h2 id="create-title" className="text-base font-semibold flex items-center gap-2">
                <ShieldCheck size={18} className="text-alpine-600" />
                Chiffrer cette sauvegarde ?
              </h2>
              <button onClick={closeCreate} className="btn-ghost btn-sm" aria-label="Fermer">
                <X size={15} />
              </button>
            </div>

            <div className="text-sm space-y-3">
              <p className="text-alpine-600">
                Une sauvegarde chiffrée reste illisible si elle est copiée sur un NAS, une clé
                USB ou un disque externe qui vous échappe.
              </p>

              {/* Quand une phrase de passe est enregistrée, elle s'applique
                  aussi ici. Le dire : sans cela, laisser le champ vide semble
                  vouloir dire « en clair » — c'était d'ailleurs le cas, et le
                  bouton produisait un fichier lisible sur une installation dont
                  les sauvegardes automatiques étaient chiffrées. */}
              {storedPolicy?.encrypting && (
                <p className="rounded-md bg-neutral-50 border border-neutral-200 px-3 py-2 text-alpine-700">
                  Vous avez enregistré une phrase de passe pour vos sauvegardes.
                  <strong> Laissez ce champ vide</strong> pour l'utiliser. N'en saisissez une ici
                  que pour protéger cette copie-là avec une phrase différente — par exemple si
                  vous la remettez à votre fiduciaire.
                </p>
              )}

              <div>
                <label className="label" htmlFor="create-passphrase">
                  Phrase de passe de chiffrement
                </label>
                {/* Bascule d'affichage. Une phrase de passe qu'on ne relit
                    pas se saisit de travers, et l'erreur ne se découvre qu'à la
                    restauration — c'est-à-dire au pire moment. Ici le risque de
                    la montrer est faible : on la saisit une fois, sur sa propre
                    machine. Elle repart masquée au prochain dialogue. */}
                <div className="relative">
                  <input
                    id="create-passphrase"
                    type={showPassphrase ? 'text' : 'password'}
                    className="input w-full pr-10"
                    value={passphrase}
                    onChange={e => setPassphrase(e.target.value)}
                    autoComplete="new-password"
                    autoFocus
                    onKeyDown={e => {
                      if (e.key === 'Enter' && !create.isPending
                          && checksFor(passphrase).every(c => c.met)) {
                        create.mutate(passphrase)
                      }
                    }}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassphrase(v => !v)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-alpine-500
                               hover:text-alpine-700"
                    aria-label={showPassphrase ? 'Masquer la phrase de passe' : 'Afficher la phrase de passe'}
                    title={showPassphrase ? 'Masquer' : 'Afficher en clair'}
                    tabIndex={-1}
                  >
                    {showPassphrase ? <EyeOff size={15} /> : <Eye size={15} />}
                  </button>
                </div>
                <PassphraseStrength value={passphrase} />

                <p className="text-xs text-alpine-500 mt-2">
                  Exemple d'une phrase solide :{' '}
                  {/* Non sélectionnable et non copiable, délibérément : cet
                      exemple est publié dans le code source et dans la
                      documentation. Le copier-coller en ferait la phrase de
                      passe la plus répandue du produit, et donc la première
                      qu'un attaquant essaierait. Le lire et s'en inspirer est
                      le but ; le reprendre tel quel ne doit pas être à un
                      raccourci clavier de distance. */}
                  <code
                    className="font-mono bg-alpine-50 px-1.5 py-0.5 rounded select-none"
                    onCopy={e => e.preventDefault()}
                    onCut={e => e.preventDefault()}
                    onContextMenu={e => e.preventDefault()}
                    onDragStart={e => e.preventDefault()}
                    aria-label="Exemple de phrase de passe, à ne pas réutiliser"
                  >{PASSPHRASE_EXAMPLE}</code>
                  {' '}— n'utilisez pas celle-ci, elle est publique.
                </p>

                <p className="text-xs text-alpine-500 mt-1.5">
                  Choisissez-la <strong>différente de votre mot de passe de connexion</strong> :
                  sinon, perdre cet ordinateur revient à perdre aussi vos sauvegardes.
                  Notez-la ailleurs que sur cette machine — <strong>sans elle, la sauvegarde
                  est définitivement illisible</strong>, y compris pour vous.
                </p>
              </div>

              {create.isError && (
                <ErrorBanner message="La sauvegarde a échoué. Rien n'a été modifié." />
              )}
            </div>

            <div className="flex justify-between items-center gap-2 mt-5">
              {/* Sauvegarder sans chiffrer reste possible, mais c'est un choix
                  posé, pas la conséquence d'un champ laissé vide. */}
              <button
                onClick={() => create.mutate('')}
                disabled={create.isPending}
                className="btn-ghost btn-sm"
              >
                Sauvegarder sans chiffrer
              </button>
              <div className="flex gap-2">
                <button onClick={closeCreate} className="btn-secondary btn-sm">
                  Annuler
                </button>
                <button
                  onClick={() => create.mutate(passphrase)}
                  disabled={create.isPending
                    || !(checksFor(passphrase).every(c => c.met)
                         || (passphrase === '' && storedPolicy?.encrypting))}
                  className="btn-primary btn-sm flex items-center gap-1.5 disabled:opacity-50"
                >
                  {create.isPending && <Loader2 size={14} className="animate-spin" />}
                  {create.isPending ? 'Chiffrement…' : 'Chiffrer et sauvegarder'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ── Avertissement avant restauration ────────────────────────────── */}
      {confirming && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="restore-title"
        >
          <div className="bg-white rounded-lg shadow-xl max-w-lg w-full p-5">
            <div className="flex items-start justify-between mb-3">
              <h2 id="restore-title" className="text-base font-semibold flex items-center gap-2">
                <AlertTriangle size={18} className="text-warning-700" />
                Restaurer cette sauvegarde ?
              </h2>
              <button onClick={() => setConfirming(null)} className="btn-ghost btn-sm" aria-label="Fermer">
                <X size={15} />
              </button>
            </div>

            <div className="text-sm space-y-3">
              <p>
                Vous êtes sur le point de remplacer <strong>toute votre comptabilité
                actuelle</strong> par le contenu de&nbsp;:
              </p>
              <p className="font-mono text-xs bg-alpine-50 rounded px-3 py-2 break-all">
                {confirming.name}
              </p>

              <div className="rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5">
                <p className="font-medium mb-1">LedgerAlps devra être redémarré.</p>
                <p>
                  Une restauration remplace le fichier que le serveur utilise : elle ne peut
                  pas se faire pendant qu'il tourne. La sauvegarde va être préparée et
                  vérifiée maintenant, puis appliquée <strong>au prochain démarrage</strong>.
                  Vous devrez fermer puis rouvrir l'application.
                </p>
              </div>

              <p className="text-alpine-600">
                Votre comptabilité actuelle sera d'abord sauvegardée automatiquement — une
                restauration lancée par erreur reste réversible.
              </p>

              {confirming.encrypted && (
                <div className="rounded-md border border-alpine-200 bg-alpine-50 px-3 py-2.5">
                  <p className="flex items-center gap-1.5 font-medium mb-2">
                    <ShieldCheck size={15} className="text-success-700" />
                    Cette sauvegarde est chiffrée
                  </p>
                  {/* Nommer le fichier : plusieurs sauvegardes peuvent avoir été
                      créées avec des phrases de passe différentes. */}
                  <label className="label" htmlFor="restore-passphrase">
                    Phrase de passe utilisée pour <span className="font-mono text-xs">{confirming.name}</span>
                  </label>
                  <input
                    id="restore-passphrase"
                    type="password"
                    className="input w-full"
                    value={restorePass}
                    onChange={e => setRestorePass(e.target.value)}
                    onKeyDown={e => {
                      if (e.key === 'Enter' && restorePass !== '' && !stage.isPending) {
                        stage.mutate()
                      }
                    }}
                    autoComplete="off"
                    autoFocus
                  />
                  <p className="text-xs text-alpine-500 mt-1">
                    Elle est vérifiée immédiatement : si elle est incorrecte, vous le saurez
                    maintenant et non au redémarrage.
                  </p>
                </div>
              )}

              {stage.isError && (
                <ErrorBanner message="Préparation refusée. Vérifiez la phrase de passe — rien n'a été modifié." />
              )}
            </div>

            <div className="flex justify-end gap-2 mt-5">
              <button onClick={() => setConfirming(null)} className="btn-secondary btn-sm">
                Annuler
              </button>
              <button
                onClick={() => stage.mutate()}
                disabled={stage.isPending || (confirming.encrypted && restorePass === '')}
                className="btn-primary btn-sm flex items-center gap-1.5 disabled:opacity-50"
              >
                {stage.isPending && <Loader2 size={14} className="animate-spin" />}
                {stage.isPending ? 'Préparation…' : 'Préparer la restauration'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}


// ─── Chiffrement des sauvegardes automatiques ────────────────────────────────
//
// Le défaut était : ne rien chiffrer, sans le dire. Mesuré sur une installation
// réelle, le dossier contenait des fichiers SQLite dont l'en-tête, le numéro de
// TVA, les adresses e-mail et l'IBAN se lisaient sans aucune clé — jusqu'à
// quatorze copies complètes, dans le dossier que l'on copie justement sur un NAS
// ou une clé USB.
//
// Renverser le défaut ne suffisait pas : une phrase de passe que l'utilisateur
// ne peut pas produire après une panne de disque transforme dix ans de pièces à
// conserver (CO art. 958f) en dix ans de pièces perdues. D'où la case à cocher
// qui n'est pas une formalité — c'est le seul moment où il peut encore la noter.
function AutoBackupPolicy() {
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [pass, setPass] = useState('')
  const [show, setShow] = useState(false)
  const [noted, setNoted] = useState(false)
  const [alsoExisting, setAlsoExisting] = useState(true)

  const policy = useQuery<BackupPolicy>({
    queryKey: ['backups', 'policy'],
    queryFn:  () => backupsApi.policy().then(r => r.data),
  })

  const reset = () => { setEditing(false); setPass(''); setNoted(false); setShow(false) }

  const save = useMutation({
    mutationFn: () => backupsApi.setPolicy(pass, alsoExisting),
    onSuccess: () => {
      reset()
      qc.invalidateQueries({ queryKey: ['backups'] })
    },
  })

  const clear = useMutation({
    mutationFn: () => backupsApi.clearPolicy(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups'] }),
  })

  if (!policy.data) return null
  const p = policy.data
  const strong = checksFor(pass).every(c => c.met)

  return (
    <div className={`rounded-md border px-4 py-3 text-sm ${
      p.encrypting ? 'border-neutral-200 bg-neutral-50' : 'border-warning-500 bg-warning-100'
    }`}>
      <div className="flex items-start gap-2">
        {p.encrypting
          ? <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
          : <ShieldOff  size={16} className="mt-0.5 flex-shrink-0 text-warning-700" />}
        <div className="flex-1">
          {p.encrypting ? (
            <>
              <p className="font-medium">Les sauvegardes automatiques sont chiffrées</p>
              <p className="mt-1 text-alpine-700">
                {p.source === 'env'
                  ? <>La phrase de passe vient de la variable d'environnement <code className="text-xs">BACKUP_PASSPHRASE</code> de ce déploiement.</>
                  : <>Phrase de passe conservée par LedgerAlps — {p.mechanism}.
                      {!p.sealed && <> Sur ce système, seuls les droits du fichier la protègent.</>}</>}
              </p>
            </>
          ) : p.source === 'unavailable' ? (
            <>
              <p className="font-medium">Votre phrase de passe est illisible sur ce compte</p>
              <p className="mt-1 text-alpine-700">
                Elle a été scellée sur un autre compte Windows ou une autre machine. En l'état,
                les prochaines sauvegardes seront écrites <strong>en clair</strong>. Redéfinissez-la
                ci-dessous — vos anciennes copies chiffrées, elles, exigent toujours la phrase
                d'origine.
              </p>
            </>
          ) : (
            <>
              <p className="font-medium">Les sauvegardes automatiques ne sont pas chiffrées</p>
              <p className="mt-1 text-alpine-700">
                Une copie de sauvegarde est un fichier complet de votre comptabilité, qui finit
                souvent sur un NAS ou une clé USB. Sans phrase de passe, elle s'ouvre sans rien
                demander.
              </p>
            </>
          )}

          {p.plaintext_count > 0 && (
            <p className="mt-1.5 text-warning-700">
              <strong>{p.plaintext_count} copie(s) déjà sur ce disque se lisent sans clé.</strong>
              {' '}Enregistrer une phrase de passe ne les protège pas rétroactivement.
            </p>
          )}

          {/* Le formulaire n'apparaît que si on le demande : sur une machine
              déjà protégée, l'afficher en permanence invite à changer une
              phrase qui marche. */}
          {!editing && p.source !== 'env' && (
            <div className="flex flex-wrap items-center gap-3 mt-3">
              <button onClick={() => setEditing(true)} className="btn-secondary btn-sm">
                {p.encrypting ? 'Changer la phrase de passe' : 'Chiffrer les sauvegardes'}
              </button>
              {p.encrypting && (
                <button
                  onClick={() => clear.mutate()}
                  disabled={clear.isPending}
                  className="text-xs text-alpine-600 hover:text-danger-700 underline underline-offset-2"
                >
                  Revenir à des sauvegardes en clair
                </button>
              )}
            </div>
          )}

          {editing && (
            <div className="mt-3 space-y-3">
              <div>
                <label className="label" htmlFor="autopass">Phrase de passe des sauvegardes</label>
                <div className="relative">
                  <input
                    id="autopass"
                    type={show ? 'text' : 'password'}
                    value={pass}
                    onChange={e => setPass(e.target.value)}
                    autoComplete="new-password"
                    className="input pr-10"
                    placeholder="au moins 16 caractères"
                  />
                  <button
                    type="button"
                    onClick={() => setShow(!show)}
                    aria-label={show ? 'Masquer la phrase de passe' : 'Afficher la phrase de passe'}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-alpine-500 hover:text-alpine-900"
                  >
                    {show ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </div>
                {pass && <PassphraseStrength value={pass} />}
              </div>

              {p.plaintext_count > 0 && (
                <label className="flex items-start gap-2 text-alpine-700">
                  <input
                    type="checkbox"
                    checked={alsoExisting}
                    onChange={e => setAlsoExisting(e.target.checked)}
                    className="mt-0.5"
                  />
                  <span>
                    Chiffrer aussi les {p.plaintext_count} copie(s) déjà présentes.
                    Chacune est relue et vérifiée avant que la version en clair soit supprimée.
                  </span>
                </label>
              )}

              <label className="flex items-start gap-2 text-alpine-700">
                <input
                  type="checkbox"
                  checked={noted}
                  onChange={e => setNoted(e.target.checked)}
                  className="mt-0.5"
                />
                <span>
                  <strong>Je l'ai notée ailleurs que sur cet ordinateur.</strong> LedgerAlps la
                  retiendra pour vous à chaque démarrage, mais ne pourra plus vous la montrer.
                  Le jour où cette machine n'est plus là, cette phrase est la seule chose qui
                  ouvre vos sauvegardes.
                </span>
              </label>

              {save.isError && (
                <ErrorBanner message={refusalMessage(save.error, "La phrase de passe n'a pas pu être enregistrée.")} />
              )}

              <div className="flex items-center gap-2">
                <button
                  onClick={() => save.mutate()}
                  disabled={!strong || !noted || save.isPending}
                  className="btn-primary btn-sm flex items-center gap-1.5"
                >
                  {save.isPending && <Loader2 size={13} className="animate-spin" />}
                  Enregistrer
                </button>
                <button onClick={reset} className="btn-ghost btn-sm">Annuler</button>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
