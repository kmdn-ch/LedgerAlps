// LedgerAlps — Bannière d'avis de conformité
//
// Affiche les évolutions légales et normatives qui concernent l'utilisateur :
// changement du standard QR-facture, obligation nLPD, etc.
//
// Les avis proviennent du flux embarqué dans le binaire (aucun appel réseau
// externe). Un avis rejeté reste masqué localement, mais uniquement pour la
// version d'avis concernée : si le texte est mis à jour, il réapparaît — un
// « ne plus afficher » ne doit pas enterrer une obligation légale révisée.

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, Download, Info, ShieldAlert, X, ExternalLink } from 'lucide-react'
import { complianceApi, updateApi } from '@/api/client'
import { cn } from '@/utils'

export interface Advisory {
  id: string
  domain: string
  severity: 'info' | 'action_required' | 'critical'
  title: string
  body: string
  source_name: string
  source_url: string
  published_at?: string
  effective_from?: string
}

const DISMISS_KEY = 'ledgeralps.compliance.dismissed'

function loadDismissed(): Record<string, string> {
  try {
    return JSON.parse(localStorage.getItem(DISMISS_KEY) ?? '{}')
  } catch {
    return {}
  }
}

function saveDismissed(map: Record<string, string>) {
  try {
    localStorage.setItem(DISMISS_KEY, JSON.stringify(map))
  } catch {
    /* stockage indisponible : l'avis restera affiché, ce qui est le bon défaut */
  }
}

// N.B. tailwind.config.js ne définit que les nuances 100/500/700 pour danger et
// warning. Une classe comme `bg-danger-50` ne génère alors aucun CSS et la
// bannière s'afficherait sans couleur — le pire des défauts pour un avertissement.
// On s'en tient donc aux nuances réellement disponibles.
const STYLES = {
  critical: {
    wrap: 'border-danger-500 bg-danger-100 text-danger-700',
    icon: ShieldAlert,
    iconClass: 'text-danger-700',
    label: 'Action requise',
  },
  action_required: {
    wrap: 'border-warning-500 bg-warning-100 text-warning-700',
    icon: AlertTriangle,
    iconClass: 'text-warning-700',
    label: 'À vérifier',
  },
  info: {
    wrap: 'border-slate-300 bg-slate-100 text-slate-700',
    icon: Info,
    iconClass: 'text-slate-500',
    label: 'Information',
  },
} as const

function formatDate(iso?: string): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleDateString('fr-CH', { day: '2-digit', month: 'long', year: 'numeric' })
}

export function ComplianceBanner() {
  const [dismissed, setDismissed] = useState<Record<string, string>>(loadDismissed)

  const { data } = useQuery({
    queryKey: ['compliance-advisories'],
    queryFn: () => complianceApi.advisories('fr').then((r) => r.data),
    // Le flux est embarqué dans le binaire : il ne change qu'à la mise à jour
    // de l'application, inutile de le réinterroger en boucle.
    staleTime: 60 * 60 * 1000,
    retry: false,
  })

  // Dernier maillon de la chaîne de conformité : la veille prévient l'équipe,
  // l'équipe publie une version conforme, ceci dit à l'utilisateur de l'installer.
  const { data: update } = useQuery({
    queryKey: ['update-check'],
    queryFn: () => updateApi.check().then((r) => r.data),
    staleTime: 6 * 60 * 60 * 1000,
    retry: false,
  })

  const advisories: Advisory[] = data?.items ?? []

  // La clé de rejet inclut la date de publication : un avis révisé réapparaît.
  const visible = advisories.filter(
    (a) => dismissed[a.id] !== (a.published_at ?? 'v1'),
  )

  const updateVersion: string | undefined = update?.update_available
    ? update.latest_version
    : undefined
  const showUpdate = Boolean(updateVersion) && dismissed['__update__'] !== updateVersion

  if (visible.length === 0 && !showUpdate) return null

  function dismissUpdate() {
    if (!updateVersion) return
    const next = { ...dismissed, __update__: updateVersion }
    setDismissed(next)
    saveDismissed(next)
  }

  function dismiss(a: Advisory) {
    const next = { ...dismissed, [a.id]: a.published_at ?? 'v1' }
    setDismissed(next)
    saveDismissed(next)
  }

  return (
    <div className="space-y-3 mb-6">
      {showUpdate && (
        <div role="status" className="border rounded-lg px-4 py-3 flex gap-3 border-alpine-500 bg-alpine-100 text-alpine-700">
          <Download className="w-5 h-5 shrink-0 mt-0.5" aria-hidden />
          <div className="flex-1 min-w-0">
            <span className="text-xs font-semibold uppercase tracking-wide opacity-70">
              Mise à jour
            </span>
            <p className="font-semibold text-sm mt-0.5">
              La version {updateVersion} est disponible
            </p>
            <p className="text-sm mt-1 leading-relaxed">
              Les mises à jour contiennent les correctifs de conformité (QR-facture,
              TVA, obligations légales). Installer la dernière version garantit que
              vos factures restent acceptées par les banques.
            </p>
            {update?.release_url && (
              <a
                href={update.release_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs mt-2 underline opacity-80 hover:opacity-100"
              >
                Voir les nouveautés et télécharger
                <ExternalLink className="w-3 h-3" aria-hidden />
              </a>
            )}
          </div>
          <button
            type="button"
            onClick={dismissUpdate}
            aria-label="Masquer cette notification"
            title="Masquer cette notification"
            className="shrink-0 opacity-50 hover:opacity-100 transition-opacity self-start"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {visible.map((a) => {
        const style = STYLES[a.severity] ?? STYLES.info
        const Icon = style.icon
        const effective = formatDate(a.effective_from)

        return (
          <div
            key={a.id}
            role="status"
            className={cn('border rounded-lg px-4 py-3 flex gap-3', style.wrap)}
          >
            <Icon className={cn('w-5 h-5 shrink-0 mt-0.5', style.iconClass)} aria-hidden />

            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="text-xs font-semibold uppercase tracking-wide opacity-70">
                  {style.label}
                </span>
                {effective && (
                  <span className="text-xs opacity-70">· en vigueur depuis le {effective}</span>
                )}
              </div>

              <p className="font-semibold text-sm mt-0.5">{a.title}</p>
              <p className="text-sm mt-1 leading-relaxed">{a.body}</p>

              <a
                href={a.source_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs mt-2 underline opacity-80 hover:opacity-100"
              >
                {a.source_name}
                <ExternalLink className="w-3 h-3" aria-hidden />
              </a>
            </div>

            <button
              type="button"
              onClick={() => dismiss(a)}
              aria-label="Masquer cet avis"
              title="Masquer cet avis"
              className="shrink-0 opacity-50 hover:opacity-100 transition-opacity self-start"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        )
      })}
    </div>
  )
}
