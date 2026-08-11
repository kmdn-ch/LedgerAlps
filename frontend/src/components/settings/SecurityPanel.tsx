// LedgerAlps — Sécurité (Paramètres → Maintenance)
//
// Point 6 de la roadmap. Le secret de signature vit en clair dans config.json.
// Qui l'obtient forge un jeton valide pour n'importe quel compte, administrateur
// compris, SANS connaître aucun mot de passe — et jusqu'ici la seule réponse
// possible était d'éditer le fichier à la main.
//
// La confirmation énonce la portée EXACTE plutôt qu'un avertissement vague.
// « Êtes-vous sûr ? » n'aide personne à décider ; « cela déconnecte les sessions
// et rien d'autre » permet d'agir sans craindre de perdre sa comptabilité.

import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { KeyRound, Loader2, ShieldCheck, AlertTriangle, RefreshCw } from 'lucide-react'
import { securityApi, backupsApi } from '@/api/client'
import { SectionTitle, ErrorBanner } from '@/components/ui'
import { targetURLAfterRestart, waitForShutdownThenGo } from '@/utils/restart'
import type { RotateSecretResult } from '@/types'
import { DatabaseEncryptionPanel } from './DatabaseEncryptionPanel'
import { SessionSecurityPanel } from './SessionSecurityPanel'
import { UsersPanel } from './UsersPanel'
import { useAuthStore } from '@/store/auth'
import { useT } from '@/i18n/useT'

export function SecurityPanel({ tlsEnabled }: { tlsEnabled: boolean }) {
  const t = useT()
  const isAdmin = useAuthStore(st => st.role) === 'admin'
  const [confirming, setConfirming] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const rotate = useMutation<RotateSecretResult>({
    mutationFn: () => securityApi.rotateSecret().then(r => r.data),
    onSuccess: () => { setConfirming(false); setError(null) },
    onError: (e: any) => setError(e?.response?.data?.error ?? t('sp.echecRotation')),
  })

  async function restartNow() {
    setRestarting(true)
    try {
      await backupsApi.restart()
      await waitForShutdownThenGo(targetURLAfterRestart(tlsEnabled))
    } catch {
      setRestarting(false)
      setError(t('sp.echecRedemarrage'))
    }
  }

  return (
    <div>
      <DatabaseEncryptionPanel />

      <SectionTitle>{t('sp.titre')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        {t('sp.introduction')}
      </p>

      <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-2">
            <KeyRound size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div>
              <p className="font-medium">{t('sp.regenererCle')}</p>
              <p className="text-alpine-600 mt-0.5">
                {t('sp.regenererAide')}
              </p>
            </div>
          </div>
          {!confirming && !rotate.data && (
            <button onClick={() => { setError(null); setConfirming(true) }}
                    className="btn-secondary btn-sm flex-shrink-0">
              {t('sp.regenerer')}
            </button>
          )}
        </div>

        {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

        {confirming && !rotate.data && (
          <div className="mt-3 rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5">
            <div className="flex items-start gap-2">
              <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-warning-700" />
              <div className="flex-1">
                <p className="font-medium text-warning-700">{t('sp.ceQueCelaFait')}</p>
                <ul className="mt-1.5 space-y-0.5 text-alpine-700 list-disc list-inside">
                  <li>{t('sp.effet1')}</li>
                  <li>{t('sp.effet2')}</li>
                  <li>{t('sp.effet3')}</li>
                  <li>{t('sp.effet4')}</li>
                </ul>
                <p className="mt-2 text-alpine-700">
                  {t('sp.pasLeDisque')}
                </p>
                <div className="mt-3 flex gap-2">
                  <button onClick={() => rotate.mutate()} disabled={rotate.isPending}
                          className="btn-primary btn-sm flex items-center gap-1.5">
                    {rotate.isPending && <Loader2 size={13} className="animate-spin" />}
                    {t('sp.regenererLaCle')}
                  </button>
                  <button onClick={() => setConfirming(false)} className="btn-ghost btn-sm">
                    {t('action.annuler')}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {rotate.data && (
          <div className="mt-3 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5">
            <div className="flex items-start gap-2">
              <ShieldCheck size={15} className="mt-0.5 flex-shrink-0 text-success-700" />
              <div className="flex-1">
                <p className="font-medium text-success-700">{t('sp.nouvelleCle')}</p>
                <p className="text-alpine-700 mt-0.5">
                  {rotate.data.sessions_revoked > 0 && (
                    <>{t('sp.sessionsRevoquees', { n: rotate.data.sessions_revoked })} </>
                  )}
                  {t('sp.prendEffet')}
                </p>
                <button onClick={restartNow} disabled={restarting}
                        className="btn-primary btn-sm mt-2 flex items-center gap-1.5">
                  {restarting
                    ? <><Loader2 size={13} className="animate-spin" /> {t('sv.redemarrage')}</>
                    : <><RefreshCw size={13} /> {t('rs.redemarrerMaintenant')}</>}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
      {/* Le second facteur a quitté cet écran : il appartient au COMPTE de
          celui qui le lit, pas à l'administration du logiciel. Le laisser ici
          l'aurait rendu inatteignable pour un comptable, qui doit pourtant
          inscrire le sien — il vit désormais dans l'onglet « Mon compte ». */}
      <SessionSecurityPanel />
      {isAdmin && <UsersPanel />}
    </div>
  )
}
