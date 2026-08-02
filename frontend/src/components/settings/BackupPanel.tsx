// LedgerAlps — Sauvegardes (onglet Paramètres)
//
// Deux verbes qui ne se comportent pas pareil, et l'interface doit le montrer :
// créer une sauvegarde est immédiat, restaurer demande de redémarrer
// l'application. Une restauration remplace toute la comptabilité, donc elle
// passe par un avertissement explicite avant d'être préparée.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Download, RotateCcw, ShieldCheck, ShieldOff, AlertTriangle, X, Loader2,
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

export function BackupPanel() {
  const qc = useQueryClient()
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
    mutationFn: () => backupsApi.create(passphrase),
    onSuccess: () => { setPassphrase(''); invalidate() },
  })

  const stage = useMutation({
    mutationFn: () => backupsApi.stageRestore(confirming!.name, restorePass),
    onSuccess: () => { setConfirming(null); setRestorePass(''); invalidate() },
  })

  const cancelRestore = useMutation({
    mutationFn: () => backupsApi.cancelRestore(),
    onSuccess: invalidate,
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
              <button
                onClick={() => cancelRestore.mutate()}
                disabled={cancelRestore.isPending}
                className="btn-ghost btn-sm mt-2"
              >
                Annuler cette restauration
              </button>
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

        <label className="label" htmlFor="backup-passphrase">
          Phrase de passe de chiffrement <span className="text-alpine-400">(facultative)</span>
        </label>
        <input
          id="backup-passphrase"
          type="password"
          className="input w-full max-w-md"
          placeholder="Laisser vide pour une sauvegarde non chiffrée"
          value={passphrase}
          onChange={e => setPassphrase(e.target.value)}
          autoComplete="new-password"
        />
        <p className="text-xs text-alpine-500 mt-1.5 max-w-md">
          Choisissez-la <strong>différente de votre mot de passe de connexion</strong> : sinon,
          perdre cet ordinateur revient à perdre aussi vos sauvegardes. Notez-la ailleurs
          que sur cette machine — <strong>sans elle, la sauvegarde est définitivement
          illisible</strong>, y compris pour vous.
        </p>

        <button
          onClick={() => create.mutate()}
          disabled={create.isPending}
          className="btn-primary btn-sm mt-3 flex items-center gap-1.5"
        >
          {create.isPending ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
          {create.isPending ? 'Sauvegarde en cours…' : 'Créer une sauvegarde'}
        </button>

        {create.isError && (
          <div className="mt-3">
            <ErrorBanner message="La sauvegarde a échoué. Rien n'a été modifié." />
          </div>
        )}
        {create.isSuccess && (
          <p className="text-sm text-success-700 mt-3">
            Sauvegarde créée et vérifiée.
          </p>
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
                <AlertTriangle size={18} className="text-warning-600" />
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
                <div>
                  <label className="label" htmlFor="restore-passphrase">
                    Phrase de passe de cette sauvegarde
                  </label>
                  <input
                    id="restore-passphrase"
                    type="password"
                    className="input w-full"
                    value={restorePass}
                    onChange={e => setRestorePass(e.target.value)}
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
