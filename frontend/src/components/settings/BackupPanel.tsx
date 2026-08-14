// LedgerAlps — Sauvegardes (onglet Paramètres)
//
// Deux verbes qui ne se comportent pas pareil, et l'interface doit le montrer :
// créer une sauvegarde est immédiat, restaurer demande de redémarrer
// l'application. Une restauration remplace toute la comptabilité, donc elle
// passe par un avertissement explicite avant d'être préparée.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Download, RotateCcw, ShieldCheck, ShieldOff, AlertTriangle, X, Loader2, RefreshCw,
  Eye, EyeOff,
} from 'lucide-react'
import { backupsApi } from '@/api/client'
import {
  SectionTitle, LoadingSpinner, ErrorBanner, EmptyState,
  PassphraseField, PassphraseStrength, passphraseIsStrong, ConfirmDialog,
} from '@/components/ui'
import { formatDate } from '@/utils'
import { useT, useFormats } from '@/i18n/useT'

// Exemple montre a l'utilisateur, deliberement non copiable : le reprendre
// tel quel en ferait la phrase de passe la plus repandue du produit, donc la
// premiere qu'un attaquant essaierait.
const PASSPHRASE_EXAMPLE = '34CryPt3DB4ckup5@26'
import { waitForShutdownThenGo } from '@/utils/restart'
import { refusalMessage } from '@/utils/refusal'
import type { BackupItem, PendingRestore, BackupPolicy } from '@/types'

function formatSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} Mo`
  if (bytes >= 1024) return `${Math.round(bytes / 1024)} Ko`
  return `${bytes} o`
}



export function BackupPanel() {
  const t = useT()
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
  if (error)     return <ErrorBanner message={t('sv.erreurLecture')} />

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
              <p className="font-medium">{t('sv.restaurationEnAttente')}</p>
              <p className="mt-1">
                {t('sv.restaurationPreparee', { nom: pending.source_name })}
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
                  {restarting ? t('sv.redemarrage') : t('sv.redemarrerMaintenant')}
                </button>
                <button
                  onClick={() => cancelRestore.mutate()}
                  disabled={cancelRestore.isPending || restart.isPending || restarting}
                  className="btn-ghost btn-sm"
                >
                  {t('sv.annulerRestauration')}
                </button>
              </div>
              {restarting && (
                <p className="text-xs mt-2">
                  {t('sv.redemarrageEnCours')}
                </p>
              )}
              {restart.isError && (
                <p className="text-xs text-danger-700 mt-2">
                  {t('sv.serveurMuet')}
                </p>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ── Créer une sauvegarde ────────────────────────────────────────── */}
      <div>
        <SectionTitle>{t('sv.creer')}</SectionTitle>
        <p className="text-sm text-alpine-600 mb-3">
          {t('sv.creerAide')}
        </p>
        <button
          onClick={() => { create.reset(); setPassphrase(''); setShowPassphrase(false); setCreating(true) }}
          className="btn-primary btn-sm flex items-center gap-1.5"
        >
          <Download size={14} />
          {t('sv.creer')}
        </button>
        {create.isSuccess && !creating && (
          <p className="text-sm text-success-700 mt-3">{t('sv.creee')}</p>
        )}
      </div>

      {/* ── Sauvegardes existantes ──────────────────────────────────────── */}
      <div>
        <SectionTitle>{t('sv.disponibles', { n: backups.length })}</SectionTitle>
        {data?.directory && (
          <p className="text-xs text-alpine-500 mb-3 font-mono break-all">{data.directory}</p>
        )}

        {backups.length === 0 ? (
          <EmptyState
            icon={<Download size={28} />}
            title={t('sv.aucune')}
            description={t('sv.aucuneAide')}
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>{t('sv.fichier')}</th><th>{t('fact.colDate')}</th>
                  <th className="text-right">{t('sv.taille')}</th>
                  <th>{t('sv.chiffrement')}</th><th></th>
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
                          <ShieldCheck size={13} /> {t('sv.chiffree')}
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-alpine-500 text-xs">
                          <ShieldOff size={13} /> {t('sv.enClair')}
                        </span>
                      )}
                    </td>
                    <td className="text-right">
                      <button
                        onClick={() => { setConfirming(b); setRestorePass('') }}
                        className="btn-ghost btn-sm flex items-center gap-1.5 ml-auto"
                      >
                        <RotateCcw size={13} /> {t('sv.restaurer')}
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
                {t('sv.chiffrerCetteSauvegarde')}
              </h2>
              <button onClick={closeCreate} className="btn-ghost btn-sm" aria-label={t('action.fermer')}>
                <X size={15} />
              </button>
            </div>

            <div className="text-sm space-y-3">
              <p className="text-alpine-600">
                {t('sv.chiffrerAide')}
              </p>

              {/* Quand une phrase de passe est enregistrée, elle s'applique
                  aussi ici. Le dire : sans cela, laisser le champ vide semble
                  vouloir dire « en clair » — c'était d'ailleurs le cas, et le
                  bouton produisait un fichier lisible sur une installation dont
                  les sauvegardes automatiques étaient chiffrées. */}
              {storedPolicy?.encrypting && (
                <p className="rounded-md bg-neutral-50 border border-neutral-200 px-3 py-2 text-alpine-700">
                  {t('sv.phraseEnregistree')}
                </p>
              )}

              <div>
                <label className="label" htmlFor="create-passphrase">
                  {t('sv.phraseChiffrement')}
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
                          && passphraseIsStrong(passphrase)) {
                        create.mutate(passphrase)
                      }
                    }}
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassphrase(v => !v)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 p-1 text-alpine-500
                               hover:text-alpine-700"
                    aria-label={showPassphrase ? t('sv.masquerPhrase') : t('sv.afficherPhrase')}
                    title={showPassphrase ? t('sv.masquer') : t('sv.afficherEnClair')}
                    tabIndex={-1}
                  >
                    {showPassphrase ? <EyeOff size={15} /> : <Eye size={15} />}
                  </button>
                </div>
                <PassphraseStrength value={passphrase} />

                <p className="text-xs text-alpine-500 mt-2">
                  {t('sv.exemplePhrase')}{' '}
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
                    aria-label={t('sv.exempleAria')}
                  >{PASSPHRASE_EXAMPLE}</code>
                  {' '}{t('sv.exemplePublique')}
                </p>

                <p className="text-xs text-alpine-500 mt-1.5">
                  {t('sv.phraseDifferente')}
                </p>
              </div>

              {create.isError && (
                <ErrorBanner message={t('sv.echecCreation')} />
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
                {t('sv.sansChiffrer')}
              </button>
              <div className="flex gap-2">
                <button onClick={closeCreate} className="btn-secondary btn-sm">
                  {t('action.annuler')}
                </button>
                <button
                  onClick={() => create.mutate(passphrase)}
                  disabled={create.isPending
                    || !(passphraseIsStrong(passphrase)
                         || (passphrase === '' && storedPolicy?.encrypting))}
                  className="btn-primary btn-sm flex items-center gap-1.5 disabled:opacity-50"
                >
                  {create.isPending && <Loader2 size={14} className="animate-spin" />}
                  {create.isPending ? t('sv.chiffrementEnCours') : t('sv.chiffrerEtSauvegarder')}
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
                {t('sv.restaurerCetteSauvegarde')}
              </h2>
              <button onClick={() => setConfirming(null)} className="btn-ghost btn-sm" aria-label={t('action.fermer')}>
                <X size={15} />
              </button>
            </div>

            <div className="text-sm space-y-3">
              <p>
                {t('sv.remplacementAvert')}
              </p>
              <p className="font-mono text-xs bg-alpine-50 rounded px-3 py-2 break-all">
                {confirming.name}
              </p>

              <div className="rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5">
                <p className="font-medium mb-1">{t('sv.devraRedemarrer')}</p>
                <p>
                  {t('sv.devraRedemarrerAide')}
                </p>
              </div>

              <p className="text-alpine-600">
                {t('sv.reversible')}
              </p>

              {confirming.encrypted && (
                <div className="rounded-md border border-alpine-200 bg-alpine-50 px-3 py-2.5">
                  <p className="flex items-center gap-1.5 font-medium mb-2">
                    <ShieldCheck size={15} className="text-success-700" />
                    {t('sv.sauvegardeChiffree')}
                  </p>
                  {/* Nommer le fichier : plusieurs sauvegardes peuvent avoir été
                      créées avec des phrases de passe différentes. */}
                  <label className="label" htmlFor="restore-passphrase">
                    {t('sv.phrasePour')} <span className="font-mono text-xs">{confirming.name}</span>
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
                    {t('sv.phraseVerifiee')}
                  </p>
                </div>
              )}

              {stage.isError && (
                <ErrorBanner message={t('sv.preparationRefusee')} />
              )}
            </div>

            <div className="flex justify-end gap-2 mt-5">
              <button onClick={() => setConfirming(null)} className="btn-secondary btn-sm">
                {t('action.annuler')}
              </button>
              <button
                onClick={() => stage.mutate()}
                disabled={stage.isPending || (confirming.encrypted && restorePass === '')}
                className="btn-primary btn-sm flex items-center gap-1.5 disabled:opacity-50"
              >
                {stage.isPending && <Loader2 size={14} className="animate-spin" />}
                {stage.isPending ? t('sv.preparationEnCours') : t('sv.preparerRestauration')}
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
  const t = useT()
  const { pluriel } = useFormats()
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [pass, setPass] = useState('')
  const [noted, setNoted] = useState(false)
  const [alsoExisting, setAlsoExisting] = useState(true)
  const [confirmingClear, setConfirmingClear] = useState(false)

  const policy = useQuery<BackupPolicy>({
    queryKey: ['backups', 'policy'],
    queryFn:  () => backupsApi.policy().then(r => r.data),
  })

  const reset = () => { setEditing(false); setPass(''); setNoted(false) }

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
  const strong = passphraseIsStrong(pass)

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
              <p className="font-medium">{t('sv.autoChiffrees')}</p>
              <p className="mt-1 text-alpine-700">
                {p.source === 'env'
                  ? t('sv.phraseEnv')
                  : <>{t('sv.phraseConservee', { mecanisme: p.mechanism })}
                      {!p.sealed && <> {t('sv.seulsDroitsFichier')}</>}</>}
              </p>
            </>
          ) : p.source === 'unavailable' ? (
            <>
              <p className="font-medium">{t('sv.phraseIllisible')}</p>
              <p className="mt-1 text-alpine-700">
                {t('sv.phraseIllisibleAide')}
              </p>
            </>
          ) : (
            <>
              <p className="font-medium">{t('sv.autoNonChiffrees')}</p>
              <p className="mt-1 text-alpine-700">
                {t('sv.autoNonChiffreesAide')}
              </p>
            </>
          )}

          {p.plaintext_count > 0 && (
            <p className="mt-1.5 text-warning-700">
              <strong>{pluriel(p.plaintext_count,
                t('sv.copieEnClairUne', { n: p.plaintext_count }),
                t('sv.copiesEnClair', { n: p.plaintext_count }))}</strong>
              {' '}{t('sv.pasRetroactif')}
            </p>
          )}

          {/* Le formulaire n'apparaît que si on le demande : sur une machine
              déjà protégée, l'afficher en permanence invite à changer une
              phrase qui marche. */}
          {!editing && p.source !== 'env' && (
            <div className="flex flex-wrap items-center gap-3 mt-3">
              <button onClick={() => setEditing(true)} className="btn-secondary btn-sm">
                {p.encrypting ? t('sv.changerPhrase') : t('sv.chiffrerLesSauvegardes')}
              </button>
              {p.encrypting && (
                <button
                  onClick={() => setConfirmingClear(true)}
                  disabled={clear.isPending}
                  className="text-xs text-alpine-600 hover:text-danger-700 underline underline-offset-2"
                >
                  {t('sv.revenirEnClairLien')}
                </button>
              )}
            </div>
          )}

          {editing && (
            <div className="mt-3 space-y-3">
              <PassphraseField
                id="autopass"
                label={t('sv.phraseSauvegardes')}
                value={pass}
                onChange={setPass}
                hint={
                  t('sv.phraseSauvegardesAide')
                }
              />

              {p.plaintext_count > 0 && (
                <label className="flex items-start gap-2 text-alpine-700">
                  <input
                    type="checkbox"
                    checked={alsoExisting}
                    onChange={e => setAlsoExisting(e.target.checked)}
                    className="mt-0.5"
                  />
                  <span>
                    {pluriel(p.plaintext_count,
                      t('sv.chiffrerAussiUne', { n: p.plaintext_count }),
                      t('sv.chiffrerAussi', { n: p.plaintext_count }))}
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
                  {t('sv.jeLaiNotee')}
                </span>
              </label>

              {save.isError && (
                <ErrorBanner message={refusalMessage(save.error, t('sv.echecEnregistrementPhrase'))} />
              )}

              <div className="flex items-center gap-2">
                <button
                  onClick={() => save.mutate()}
                  disabled={!strong || !noted || save.isPending}
                  className="btn-primary btn-sm flex items-center gap-1.5"
                >
                  {save.isPending && <Loader2 size={13} className="animate-spin" />}
                  {t('action.enregistrer')}
                </button>
                <button onClick={reset} className="btn-ghost btn-sm">{t('action.annuler')}</button>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Revenir en clair efface la phrase de passe conservée. Ce n'est pas
          seulement « les prochaines copies ne seront plus protégées » : les
          copies DÉJÀ chiffrées restent chiffrées, et si personne n'a noté la
          phrase ailleurs, elles deviennent définitivement illisibles.

          C'est la conséquence que l'utilisateur ne devine pas, donc celle que
          le dialogue met en premier. Le composant impose de fournir les
          conséquences plutôt qu'un « êtes-vous sûr » : répétée, la question
          devient un réflexe et le clic précède la lecture. */}
      <ConfirmDialog
        open={confirmingClear}
        tone="danger"
        title={t('sv.revenirEnClairTitre')}
        consequences={[
          <>
            <strong>{t('sv.consEffacee')}</strong>{' '}
            {p.encrypted_count > 0
              ? pluriel(p.encrypted_count,
                  t('sv.consChiffreeRestenteUne', { n: p.encrypted_count }),
                  t('sv.consChiffreesRestent', { n: p.encrypted_count }))
              : t('sv.consAucuneChiffree')}
          </>,
          t('sv.consProchainesEnClair'),
          t('sv.consCopieQuiVoyage'),
        ]}
        reassurance={t('sv.consRassurance')}
        confirmLabel={t('sv.revenirEnClairBouton')}
        busy={clear.isPending}
        onConfirm={() => { setConfirmingClear(false); clear.mutate() }}
        onCancel={() => setConfirmingClear(false)}
      />
    </div>
  )
}
