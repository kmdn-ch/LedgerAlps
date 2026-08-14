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
import { useT, useLangue } from '@/i18n/useT'
import { localeIntl } from '@/i18n'
import type { Cle } from '@/i18n'

// Deux minutes est le plancher accepté par le serveur : en dessous, la
// déconnexion tombe pendant la lecture d'un document, et comme aucun brouillon
// n'est enregistré, la saisie est perdue.
const IDLE_CHOICES: { v: number; cle: Cle }[] = [
  { v: 0,  cle: 'ss.jamais' },
  { v: 5,  cle: 'ss.cinqMinutes' },
  { v: 10, cle: 'ss.dixMinutes' },
  { v: 20, cle: 'ss.vingtMinutes' },
  { v: 60, cle: 'ss.uneHeure' },
]

const ROTATION_CHOICES: { v: number; cle: Cle }[] = [
  { v: 0,  cle: 'ss.jamais' },
  { v: 1,  cle: 'ss.chaqueJour' },
  { v: 7,  cle: 'ss.chaqueSemaine' },
  { v: 30, cle: 'ss.chaqueMois' },
]

// La locale suit la langue choisie : « 09.02.2026 14:30 » est suisse, un
// Britannique lit « 09/02/2026, 14:30 ». Le figer en fr-CH affichait une date
// française sur une interface anglaise.
function formatDateTime(iso: string | undefined, loc: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString(loc, {
    dateStyle: 'short', timeStyle: 'short',
  })
}

export function SessionSecurityPanel() {
  const t = useT()
  const loc = localeIntl(useLangue())
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
    onError:   (e) => setError(refusalMessage(e, t('ss.echecReglage'))),
  })

  const s = settings.data
  if (!s) return null

  return (
    <div className="mt-6">
      <SectionTitle>{t('ss.titre')}</SectionTitle>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}

      <div className="space-y-3">
        {/* ── Déconnexion sur inactivité ──────────────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <Clock size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div className="flex-1">
              <p className="font-medium">{t('ss.deconnexionTitre')}</p>
              <p className="text-alpine-600 mt-0.5">
                {t('ss.deconnexionAide')}
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
                    {t(c.cle)}
                  </button>
                ))}
                {save.isPending && <Loader2 size={13} className="animate-spin text-alpine-500" />}
              </div>
              {s.idle_logout_minutes === 0 && (
                <p className="mt-2 text-warning-700 text-xs">
                  {t('ss.deconnexionDesactivee')}
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
              <p className="font-medium">{t('ss.rotationTitre')}</p>
              <p className="text-alpine-600 mt-0.5">
                {t('ss.rotationAide')}
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
                    {t(c.cle)}
                  </button>
                ))}
              </div>
              <p className="mt-2 text-xs text-alpine-500">
                {t('ss.derniereRegeneration', { date: formatDateTime(s.rotation.rotated_at, loc) })}
                {s.rotation.next_at
                  && t('ss.prochaineAu', { date: formatDateTime(s.rotation.next_at, loc) })}
              </p>
              {s.rotation.max_age_days === 0 && (
                <p className="mt-1 text-warning-700 text-xs">
                  {t('ss.rotationDesactivee')}
                </p>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
