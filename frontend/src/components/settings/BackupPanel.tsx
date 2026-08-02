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
} from 'lucide-react'
import { backupsApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner, EmptyState } from '@/components/ui'
import { formatDate } from '@/utils'
import type { BackupItem, PendingRestore } from '@/types'

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
    onSuccess: () => { setCreating(false); setPassphrase(''); invalidate() },
  })

  const closeCreate = () => { setCreating(false); setPassphrase(''); create.reset() }

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
  const [restarting, setRestarting] = useState(false)
  const restart = useMutation({
    mutationFn: async () => {
      await backupsApi.restart()
      setRestarting(true)
      const deadline = Date.now() + 60_000
      while (Date.now() < deadline) {
        await new Promise(r => setTimeout(r, 1000))
        try {
          const res = await fetch('/health', { cache: 'no-store' })
          if (res.ok) return
        } catch {
          // Le serveur est en train de redémarrer : c'est attendu.
        }
      }
      throw new Error('timeout')
    },
    onSuccess: () => window.location.reload(),
    onError:   () => setRestarting(false),
  })

  if (isLoading) return <LoadingSpinner />
  if (error)     return <ErrorBanner message="Impossible de lire les sauvegardes." />

  const backups = data?.items ?? []
  const pending = data?.pending_restore

  return (
    <div className="space-y-5">
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
          onClick={() => { create.reset(); setPassphrase(''); setCreating(true) }}
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

              <div>
                <label className="label" htmlFor="create-passphrase">
                  Phrase de passe de chiffrement
                </label>
                <input
                  id="create-passphrase"
                  type="password"
                  className="input w-full"
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
                <PassphraseStrength value={passphrase} />

                <p className="text-xs text-alpine-500 mt-2">
                  Exemple d'une phrase solide :{' '}
                  <code className="font-mono bg-alpine-50 px-1.5 py-0.5 rounded">{PASSPHRASE_EXAMPLE}</code>
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
                  disabled={create.isPending || !checksFor(passphrase).every(c => c.met)}
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
