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

export function SecurityPanel({ tlsEnabled }: { tlsEnabled: boolean }) {
  const [confirming, setConfirming] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const rotate = useMutation<RotateSecretResult>({
    mutationFn: () => securityApi.rotateSecret().then(r => r.data),
    onSuccess: () => { setConfirming(false); setError(null) },
    onError: (e: any) => setError(e?.response?.data?.error ?? "La rotation a échoué."),
  })

  async function restartNow() {
    setRestarting(true)
    try {
      await backupsApi.restart()
      await waitForShutdownThenGo(targetURLAfterRestart(tlsEnabled))
    } catch {
      setRestarting(false)
      setError("Le redémarrage n'a pas pu être demandé. Fermez et rouvrez LedgerAlps.")
    }
  }

  return (
    <div>
      <SectionTitle>Sécurité</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        LedgerAlps signe vos sessions avec une clé conservée dans son fichier de
        configuration. La régénérer met fin à toutes les sessions en cours.
      </p>

      <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-2">
            <KeyRound size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div>
              <p className="font-medium">Régénérer la clé de signature</p>
              <p className="text-alpine-600 mt-0.5">
                À faire si votre fichier de configuration a pu être vu par quelqu'un
                d'autre : joint à un ticket de support, copié sur une clé USB, ou
                poussé par erreur dans un dépôt de code. Qui détient cette clé peut
                se faire passer pour n'importe quel compte{' '}
                <strong>sans connaître le mot de passe</strong>.
              </p>
            </div>
          </div>
          {!confirming && !rotate.data && (
            <button onClick={() => { setError(null); setConfirming(true) }}
                    className="btn-secondary btn-sm flex-shrink-0">
              Régénérer
            </button>
          )}
        </div>

        {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

        {confirming && !rotate.data && (
          <div className="mt-3 rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5">
            <div className="flex items-start gap-2">
              <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-warning-700" />
              <div className="flex-1">
                <p className="font-medium text-warning-700">Ce que cela fait, exactement</p>
                <ul className="mt-1.5 space-y-0.5 text-alpine-700 list-disc list-inside">
                  <li>Toutes les sessions ouvertes se ferment — vous vous reconnecterez.</li>
                  <li><strong>Et rien d'autre.</strong> Vos mots de passe restent valables.</li>
                  <li>Aucune donnée comptable n'est touchée.</li>
                  <li>
                    Vos sauvegardes restent utilisables : elles ne contiennent pas cette
                    clé, et leur chiffrement dépend de votre phrase de passe, pas d'elle.
                  </li>
                </ul>
                <p className="mt-2 text-alpine-700">
                  Cela ne remplace pas le chiffrement du disque : qui peut lire le
                  fichier de configuration peut aussi lire la base de données, posée
                  dans le même dossier.
                </p>
                <div className="mt-3 flex gap-2">
                  <button onClick={() => rotate.mutate()} disabled={rotate.isPending}
                          className="btn-primary btn-sm flex items-center gap-1.5">
                    {rotate.isPending && <Loader2 size={13} className="animate-spin" />}
                    Régénérer la clé
                  </button>
                  <button onClick={() => setConfirming(false)} className="btn-ghost btn-sm">
                    Annuler
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
                <p className="font-medium text-success-700">Nouvelle clé enregistrée</p>
                <p className="text-alpine-700 mt-0.5">
                  {rotate.data.sessions_revoked > 0 && (
                    <>{rotate.data.sessions_revoked} session(s) révoquée(s). </>
                  )}
                  La nouvelle clé prend effet au redémarrage de LedgerAlps. Jusque-là,
                  l'ancienne reste en mémoire du serveur en cours d'exécution.
                </p>
                <button onClick={restartNow} disabled={restarting}
                        className="btn-primary btn-sm mt-2 flex items-center gap-1.5">
                  {restarting
                    ? <><Loader2 size={13} className="animate-spin" /> Redémarrage…</>
                    : <><RefreshCw size={13} /> Redémarrer maintenant</>}
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
