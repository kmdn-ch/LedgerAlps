// Durée de vie d'une session : rotation de la clé, déconnexion sur inactivité.
//
// Les deux réglages vivent ensemble parce qu'ils bornent la même chose par deux
// chemins — combien de temps une session vaut quelque chose. Les séparer conduit
// à en durcir un et à laisser l'autre grand ouvert, ce qui donne l'illusion
// d'une protection sans la protection.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Clock, RefreshCw, Loader2 } from 'lucide-react'
import { securityApi } from '@/api/client'
import { SectionTitle, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import type { SecuritySettings } from '@/types'

// Deux minutes est le plancher accepté par le serveur : en dessous, la
// déconnexion tombe pendant la lecture d'un document, et comme aucun brouillon
// n'est enregistré, la saisie est perdue.
const IDLE_CHOICES = [
  { v: 0,  label: 'Jamais' },
  { v: 5,  label: '5 minutes' },
  { v: 10, label: '10 minutes' },
  { v: 20, label: '20 minutes' },
  { v: 60, label: '1 heure' },
]

const ROTATION_CHOICES = [
  { v: 0,  label: 'Jamais' },
  { v: 1,  label: 'Chaque jour' },
  { v: 7,  label: 'Chaque semaine' },
  { v: 30, label: 'Chaque mois' },
]

function formatDateTime(iso?: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('fr-CH', {
    dateStyle: 'short', timeStyle: 'short',
  })
}

export function SessionSecurityPanel() {
  const qc = useQueryClient()
  const [error, setError] = useState<string | null>(null)

  const settings = useQuery<SecuritySettings>({
    queryKey: ['settings', 'security'],
    queryFn:  () => securityApi.settings().then(r => r.data),
  })

  const save = useMutation({
    mutationFn: (body: { rotation_days?: number; idle_logout_minutes?: number }) =>
      securityApi.saveSettings(body),
    onSuccess: () => { setError(null); qc.invalidateQueries({ queryKey: ['settings', 'security'] }) },
    onError:   (e) => setError(refusalMessage(e, "Le réglage n'a pas pu être enregistré.")),
  })

  const s = settings.data
  if (!s) return null

  return (
    <div className="mt-6">
      <SectionTitle>Durée de vie des sessions</SectionTitle>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}

      <div className="space-y-3">
        {/* ── Déconnexion sur inactivité ──────────────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <Clock size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div className="flex-1">
              <p className="font-medium">Déconnexion après inactivité</p>
              <p className="text-alpine-600 mt-0.5">
                Protège l'écran laissé ouvert — un bureau partagé, un portable en salle
                d'attente. Un avertissement d'une minute précède la coupure, avec un bouton
                pour rester : LedgerAlps ne conserve pas de brouillon automatique, et une
                saisie en cours ne doit pas disparaître sans qu'on ait pu l'arrêter.
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {IDLE_CHOICES.map(c => (
                  <button
                    key={c.v}
                    onClick={() => save.mutate({ idle_logout_minutes: c.v })}
                    disabled={save.isPending}
                    className={`btn-sm rounded-md border px-2.5 py-1 ${
                      s.idle_logout_minutes === c.v
                        ? 'border-accent-700 bg-accent-700 text-white'
                        : 'border-neutral-200 hover:border-alpine-500'
                    }`}
                  >
                    {c.label}
                  </button>
                ))}
                {save.isPending && <Loader2 size={13} className="animate-spin text-alpine-500" />}
              </div>
              {s.idle_logout_minutes === 0 && (
                <p className="mt-2 text-warning-700 text-xs">
                  Désactivée : une session ouverte le reste tant que la fenêtre l'est.
                </p>
              )}
            </div>
          </div>
        </div>

        {/* ── Rotation de la clé de signature ─────────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <RefreshCw size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div className="flex-1">
              <p className="font-medium">Régénération automatique de la clé de signature</p>
              <p className="text-alpine-600 mt-0.5">
                Borne la valeur d'une fuite passée : un fichier de configuration parti dans
                un ticket de support la semaine dernière ne permet plus rien aujourd'hui.
                La clé est régénérée <strong>au démarrage</strong>, jamais en cours de
                session — vous aurez simplement à vous reconnecter.
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {ROTATION_CHOICES.map(c => (
                  <button
                    key={c.v}
                    onClick={() => save.mutate({ rotation_days: c.v })}
                    disabled={save.isPending}
                    className={`btn-sm rounded-md border px-2.5 py-1 ${
                      s.rotation.max_age_days === c.v
                        ? 'border-accent-700 bg-accent-700 text-white'
                        : 'border-neutral-200 hover:border-alpine-500'
                    }`}
                  >
                    {c.label}
                  </button>
                ))}
              </div>
              <p className="mt-2 text-xs text-alpine-500">
                Dernière régénération : {formatDateTime(s.rotation.rotated_at)}
                {s.rotation.next_at && <> · prochaine au démarrage suivant le {formatDateTime(s.rotation.next_at)}</>}
              </p>
              {s.rotation.max_age_days === 0 && (
                <p className="mt-1 text-warning-700 text-xs">
                  Désactivée : la clé ne changera plus que si vous la régénérez vous-même.
                </p>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
