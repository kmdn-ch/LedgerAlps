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
import { Lock, LockOpen, AlertTriangle, Loader2, KeyRound, Eye, EyeOff } from 'lucide-react'
import { databaseApi } from '@/api/client'
import { SectionTitle, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import type { DatabaseEncryption } from '@/types'

const MIN_LEN = 16

function strongEnough(p: string): boolean {
  return [...p].length >= MIN_LEN &&
    /\p{Ll}/u.test(p) && /\p{Lu}/u.test(p) && /\p{Nd}/u.test(p)
}

export function DatabaseEncryptionPanel() {
  const qc = useQueryClient()
  const [mode, setMode] = useState<'idle' | 'enable' | 'recover'>('idle')
  const [pass, setPass] = useState('')
  const [show, setShow] = useState(false)
  const [noted, setNoted] = useState(false)

  const status = useQuery<DatabaseEncryption>({
    queryKey: ['database', 'encryption'],
    queryFn:  () => databaseApi.encryption().then(r => r.data),
  })

  const reset = () => { setMode('idle'); setPass(''); setNoted(false); setShow(false) }
  const invalidate = () => qc.invalidateQueries({ queryKey: ['database', 'encryption'] })

  const enable   = useMutation({ mutationFn: () => databaseApi.enableEncryption(pass),
                                 onSuccess: () => { reset(); invalidate() } })
  const disable  = useMutation({ mutationFn: () => databaseApi.disableEncryption(), onSuccess: invalidate })
  const cancel   = useMutation({ mutationFn: () => databaseApi.cancelEncryption(),  onSuccess: invalidate })
  const recover  = useMutation({ mutationFn: () => databaseApi.recoverKey(pass),
                                 onSuccess: () => { reset(); invalidate() } })

  const s = status.data
  if (!s) return null
  if (!s.supported) {
    return (
      <div>
        <SectionTitle>Chiffrement de la base</SectionTitle>
        <p className="text-sm text-alpine-600">
          Cette installation utilise PostgreSQL : le chiffrement au repos s'y règle côté serveur
          de base de données.
        </p>
      </div>
    )
  }

  return (
    <div>
      <SectionTitle>Chiffrement de la base</SectionTitle>

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
              {s.encrypted ? 'La base de données est chiffrée' : 'La base de données est en clair'}
            </p>

            {s.encrypted ? (
              <p className="mt-1 text-alpine-700">
                Le fichier reste illisible s'il est copié ailleurs — NAS, partage réseau, dossier
                synchronisé. Clé conservée par LedgerAlps ({s.mechanism}) ; rien à saisir au
                démarrage.
              </p>
            ) : (
              <p className="mt-1 text-alpine-700">
                C'est le réglage normal, et il suffit à la plupart des installations : le
                chiffrement du disque protège déjà contre le vol du poste. Chiffrer la base
                n'ajoute qu'une chose, mais elle est réelle — la protection <em>suit le fichier</em>.
              </p>
            )}

            {/* Ce que ça ne fait pas. Dit ici, et pas en petit ailleurs. */}
            <p className="mt-1.5 text-alpine-500 text-xs">
              Dans les deux cas, un programme lancé sous votre compte Windows accède à vos données :
              il peut demander la clé comme LedgerAlps le fait. Ce chiffrement protège le fichier,
              pas la session.
            </p>

            {s.configured && !s.key_available && (
              <p className="mt-2 text-danger-700">
                <strong>La clé ne se descelle pas sur ce compte.</strong> Cette base vient d'un autre
                compte Windows ou d'une autre machine. Utilisez la phrase de récupération.
              </p>
            )}

            {s.encrypted && !s.has_recovery && (
              <p className="mt-2 text-warning-700">
                <strong>Aucune phrase de récupération n'est enregistrée.</strong> Si ce compte
                Windows disparaît, cette base ne s'ouvrira plus.
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
                {s.pending === 'encrypt'
                  ? 'Chiffrement programmé au prochain démarrage'
                  : 'Retour en clair programmé au prochain démarrage'}
              </p>
              <p className="mt-1">
                La conversion remplace le fichier que le serveur a ouvert : elle ne peut pas se
                faire maintenant. <strong>Fermez puis rouvrez LedgerAlps.</strong>
              </p>
              <button
                onClick={() => cancel.mutate()}
                disabled={cancel.isPending}
                className="btn-ghost btn-sm mt-2"
              >
                Annuler
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
              Chiffrer la base de données
            </button>
          )}
          {s.configured && !s.key_available && (
            <button onClick={() => setMode('recover')} className="btn-primary btn-sm flex items-center gap-1.5">
              <KeyRound size={13} /> Récupérer la clé
            </button>
          )}
          {s.configured && s.key_available && (
            <button
              onClick={() => disable.mutate()}
              disabled={disable.isPending}
              className="text-xs text-alpine-600 hover:text-danger-700 underline underline-offset-2"
            >
              Revenir à une base en clair
            </button>
          )}
        </div>
      )}

      {(mode === 'enable' || mode === 'recover') && (
        <div className="mt-3 space-y-3 text-sm">
          <div>
            <label className="label" htmlFor="dbrecovery">
              {mode === 'enable' ? 'Phrase de récupération' : 'Votre phrase de récupération'}
            </label>
            <div className="relative">
              <input
                id="dbrecovery"
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
                aria-label={show ? 'Masquer' : 'Afficher'}
                className="absolute right-2 top-1/2 -translate-y-1/2 text-alpine-500 hover:text-alpine-900"
              >
                {show ? <EyeOff size={16} /> : <Eye size={16} />}
              </button>
            </div>
          </div>

          {mode === 'enable' && (
            <>
              <p className="text-alpine-600 text-xs">
                Elle n'est demandée qu'en cas de changement de machine ou de compte Windows.
                Au quotidien, LedgerAlps s'ouvre sans rien réclamer.
              </p>
              <label className="flex items-start gap-2 text-alpine-700">
                <input type="checkbox" checked={noted} onChange={e => setNoted(e.target.checked)} className="mt-0.5" />
                <span>
                  <strong>Je l'ai notée ailleurs que sur cet ordinateur.</strong> Sans elle, une
                  réinstallation de Windows ou une panne de cette machine rend la base
                  définitivement illisible — y compris les dix ans de pièces que le CO art. 958f
                  impose de conserver.
                </span>
              </label>
            </>
          )}

          {(enable.isError || recover.isError) && (
            <ErrorBanner message={refusalMessage(
              enable.error ?? recover.error,
              mode === 'enable' ? "Le chiffrement n'a pas pu être activé." : 'La clé n\'a pas pu être récupérée.',
            )} />
          )}

          <div className="flex items-center gap-2">
            <button
              onClick={() => (mode === 'enable' ? enable : recover).mutate()}
              disabled={
                (mode === 'enable' && (!strongEnough(pass) || !noted)) ||
                (mode === 'recover' && pass === '') ||
                enable.isPending || recover.isPending
              }
              className="btn-primary btn-sm flex items-center gap-1.5"
            >
              {(enable.isPending || recover.isPending) && <Loader2 size={13} className="animate-spin" />}
              {mode === 'enable' ? 'Programmer le chiffrement' : 'Récupérer'}
            </button>
            <button onClick={reset} className="btn-ghost btn-sm">Annuler</button>
          </div>
        </div>
      )}
    </div>
  )
}
