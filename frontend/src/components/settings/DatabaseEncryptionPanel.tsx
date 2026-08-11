// LedgerAlps — Chiffrement de la base (Paramètres → Maintenance → Sécurité)
//
// Cet écran a un devoir particulier : ne pas survendre.
//
// Le chiffrement du disque (BitLocker, LUKS) couvre déjà les mêmes menaces,
// gratuitement, sans que LedgerAlps détienne quoi que ce soit. Ce que le
// chiffrement de la base ajoute est étroit mais réel : la protection suit le
// fichier. Une base copiée sur un NAS, un partage réseau ou un dossier
// synchronisé reste illisible, ce qu'un disque chiffré ne fait pas.
//
// Ce qu'il n'apporte pas est dit aussi. Un programme qui tourne sous le même
// compte Windows peut demander la clé exactement comme LedgerAlps le fait.
// Laisser croire le contraire coûterait la confiance dans tous les autres
// avertissements du logiciel.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Lock, LockOpen, AlertTriangle, Loader2, KeyRound } from 'lucide-react'
import { databaseApi } from '@/api/client'
import { SectionTitle, ErrorBanner, PassphraseField, passphraseIsStrong } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import type { DatabaseEncryption } from '@/types'
import { useT } from '@/i18n/useT'

export function DatabaseEncryptionPanel() {
  const t = useT()
  const qc = useQueryClient()
  const [mode, setMode] = useState<'idle' | 'enable' | 'recover' | 'rekey'>('idle')
  const [pass, setPass] = useState('')
  const [noted, setNoted] = useState(false)

  const status = useQuery<DatabaseEncryption>({
    queryKey: ['database', 'encryption'],
    queryFn:  () => databaseApi.encryption().then(r => r.data),
  })

  const reset = () => { setMode('idle'); setPass(''); setNoted(false) }
  const invalidate = () => qc.invalidateQueries({ queryKey: ['database', 'encryption'] })

  const enable   = useMutation({ mutationFn: () => databaseApi.enableEncryption(pass),
                                 onSuccess: () => { reset(); invalidate() } })
  const disable  = useMutation({ mutationFn: () => databaseApi.disableEncryption(), onSuccess: invalidate })
  const cancel   = useMutation({ mutationFn: () => databaseApi.cancelEncryption(),  onSuccess: invalidate })
  const recover  = useMutation({ mutationFn: () => databaseApi.recoverKey(pass),
                                 onSuccess: () => { reset(); invalidate() } })
  const rekey    = useMutation({ mutationFn: () => databaseApi.changeRecovery(pass),
                                 onSuccess: () => { reset(); invalidate() } })

  const s = status.data
  if (!s) return null
  if (!s.supported) {
    return (
      <div>
        <SectionTitle>{t('be.titre')}</SectionTitle>
        <p className="text-sm text-alpine-600">{t('be.postgres')}</p>
      </div>
    )
  }

  return (
    <div>
      <SectionTitle>{t('be.titre')}</SectionTitle>

      {/* L'état d'abord, en une phrase. Le reste l'explique. */}
      <div className={`rounded-md border px-4 py-3 text-sm ${
        s.encrypted ? 'border-neutral-200 bg-neutral-50' : 'border-neutral-200 bg-white'
      }`}>
        <div className="flex items-start gap-2">
          {s.encrypted
            ? <Lock size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
            : <LockOpen size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />}
          <div className="flex-1">
            <p className="font-medium">
              {t(s.encrypted ? 'be.estChiffree' : 'be.estEnClair')}
            </p>

            {s.encrypted ? (
              <p className="mt-1 text-alpine-700">
                {t('be.chiffreeAide', { mecanisme: s.mechanism ?? '' })}
              </p>
            ) : (
              <p className="mt-1 text-alpine-700">
                {t('be.enClairAide')}
              </p>
            )}

            {/* Ce que ça ne fait pas. Dit ici, et pas en petit ailleurs. */}
            <p className="mt-1.5 text-alpine-500 text-xs">
              {t('be.pasLaSession')}
            </p>

            {s.configured && !s.key_available && (
              <p className="mt-2 text-danger-700">
                {t('be.cleNonDescellee')}
              </p>
            )}

            {s.encrypted && !s.has_recovery && (
              <p className="mt-2 text-warning-700">
                {t('be.sansRecuperation')}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Conversion en attente : elle ne peut pas se faire pendant que le
          serveur tient le fichier ouvert, et l'interface doit le dire au lieu
          de laisser croire que le clic a fait le travail. */}
      {s.pending && (
        <div className="mt-3 rounded-md border border-warning-500 bg-warning-100 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <AlertTriangle size={16} className="mt-0.5 flex-shrink-0 text-warning-700" />
            <div className="flex-1">
              <p className="font-medium">
                {t(s.pending === 'encrypt' ? 'be.chiffrementProgramme' : 'be.clairProgramme')}
              </p>
              <p className="mt-1">
                {t('be.conversionAide')}
              </p>
              <button
                onClick={() => cancel.mutate()}
                disabled={cancel.isPending}
                className="btn-ghost btn-sm mt-2"
              >
                {t('action.annuler')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Actions ─────────────────────────────────────────────────────── */}
      {!s.pending && mode === 'idle' && (
        <div className="flex flex-wrap items-center gap-3 mt-3">
          {!s.configured && (
            <button onClick={() => setMode('enable')} className="btn-secondary btn-sm">
              {t('be.chiffrerLaBase')}
            </button>
          )}
          {s.configured && !s.key_available && (
            <button onClick={() => setMode('recover')} className="btn-primary btn-sm flex items-center gap-1.5">
              <KeyRound size={13} /> {t('be.recupererCle')}
            </button>
          )}
          {s.configured && s.key_available && (
            <>
              {/* Le chiffrement est en place : cet écran n'a plus à le proposer.
                  Il sert désormais à entretenir ce qui existe — changer la
                  phrase de récupération mal notée, ou revenir en arrière.
                  Le panneau ne disparaît pas pour autant : les installations
                  antérieures à l'assistant n'ont jamais vu la question, et
                  quelqu'un qui a décliné doit pouvoir changer d'avis. */}
              <button onClick={() => setMode('rekey')} className="btn-secondary btn-sm">
                {t('be.changerPhrase')}
              </button>
              <button
                onClick={() => disable.mutate()}
                disabled={disable.isPending}
                className="text-xs text-alpine-600 hover:text-danger-700 underline underline-offset-2"
              >
                {t('be.revenirEnClair')}
              </button>
            </>
          )}
        </div>
      )}

      {mode !== 'idle' && (
        <div className="mt-3 space-y-3 text-sm">
          <PassphraseField
            id="dbrecovery"
            label={t(mode === 'recover' ? 'be.champVotrePhrase'
              : mode === 'rekey' ? 'be.champNouvellePhrase'
              : 'be.champPhrase')}
            value={pass}
            onChange={setPass}
            autoFocus
            showStrength={mode !== 'recover'}
            hint={t(mode !== 'recover' ? 'be.aideNouvelle' : 'be.aideRecuperation')}
          />

          {mode !== 'recover' && (
            <>
              <p className="text-alpine-600 text-xs">
                {t('be.rarementDemandee')}
              </p>
              <label className="flex items-start gap-2 text-alpine-700">
                <input type="checkbox" checked={noted} onChange={e => setNoted(e.target.checked)} className="mt-0.5" />
                <span>
                  {t('be.jeLaiNotee')}
                </span>
              </label>
            </>
          )}

          {(enable.isError || recover.isError || rekey.isError) && (
            <ErrorBanner message={refusalMessage(
              enable.error ?? recover.error ?? rekey.error,
              t(mode === 'enable' ? 'be.echecActivation'
                : mode === 'rekey' ? 'be.echecChangement'
                : 'be.echecRecuperation'),
            )} />
          )}

          <div className="flex items-center gap-2">
            <button
              onClick={() => (mode === 'enable' ? enable : mode === 'rekey' ? rekey : recover).mutate()}
              disabled={
                (mode !== 'recover' && (!passphraseIsStrong(pass) || !noted)) ||
                (mode === 'recover' && pass === '') ||
                enable.isPending || recover.isPending || rekey.isPending
              }
              className="btn-primary btn-sm flex items-center gap-1.5"
            >
              {(enable.isPending || recover.isPending || rekey.isPending) && <Loader2 size={13} className="animate-spin" />}
              {t(mode === 'enable' ? 'be.programmerChiffrement'
                : mode === 'rekey' ? 'be.enregistrerNouvelle'
                : 'be.recuperer')}
            </button>
            <button onClick={reset} className="btn-ghost btn-sm">{t('action.annuler')}</button>
          </div>
        </div>
      )}
    </div>
  )
}
