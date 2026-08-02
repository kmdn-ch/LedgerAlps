// LedgerAlps — Bannière d'avis de conformité
//
// Affiche les évolutions légales et normatives qui concernent l'utilisateur :
// changement du standard QR-facture, obligation nLPD, mise à jour disponible.
//
// Les avis proviennent du flux embarqué dans le binaire (aucun appel externe).
//
// Repliée par défaut, et c'est délibéré : la première version affichait le
// texte complet de chaque avis en haut de CHAQUE page, ce qui consommait près
// de la moitié de l'écran et repoussait les formulaires sous la ligne de
// flottaison. Un avertissement qui gêne le travail quotidien est un
// avertissement qu'on apprend à ignorer.

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  AlertTriangle, ChevronDown, ChevronRight, Download, Info, ShieldAlert, X, ExternalLink,
} from 'lucide-react'
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

// tailwind.config.js ne définit que les nuances 100/500/700 pour danger et
// warning : une classe comme bg-danger-100 n'émettrait aucun CSS et la bannière
// s'afficherait sans couleur — le pire des défauts pour un avertissement.
const STYLES = {
  critical: {
    wrap: 'border-danger-500 bg-danger-100 text-danger-700',
    icon: ShieldAlert,
    label: 'Action requise',
  },
  action_required: {
    wrap: 'border-warning-500 bg-warning-100 text-warning-700',
    icon: AlertTriangle,
    label: 'À vérifier',
  },
  info: {
    wrap: 'border-slate-300 bg-slate-100 text-slate-700',
    icon: Info,
    label: 'Information',
  },
} as const

function formatDate(iso?: string): string | null {
  if (!iso) return null
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return null
  return d.toLocaleDateString('fr-CH', { day: '2-digit', month: 'long', year: 'numeric' })
}

/** Ligne repliable commune aux avis et à la notification de mise à jour. */
function CollapsibleNotice({
  wrap, Icon, label, title, children, onDismiss, dismissLabel,
}: {
  wrap: string
  Icon: typeof Info
  label: string
  title: string
  children: React.ReactNode
  onDismiss: () => void
  dismissLabel: string
}) {
  const [open, setOpen] = useState(false)

  return (
    <div role="status" className={cn('border rounded-lg', wrap)}>
      <div className="flex items-center gap-2 px-3 py-2">
        <Icon className="w-4 h-4 shrink-0" aria-hidden />

        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="flex-1 min-w-0 flex items-center gap-2 text-left"
        >
          <span className="text-[11px] font-semibold uppercase tracking-wide opacity-70 shrink-0">
            {label}
          </span>
          <span className="text-sm font-medium truncate">{title}</span>
          {open
            ? <ChevronDown className="w-4 h-4 shrink-0 opacity-60 ml-auto" aria-hidden />
            : <ChevronRight className="w-4 h-4 shrink-0 opacity-60 ml-auto" aria-hidden />}
        </button>

        <button
          type="button"
          onClick={onDismiss}
          aria-label={dismissLabel}
          title={dismissLabel}
          className="shrink-0 opacity-50 hover:opacity-100 transition-opacity"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {open && <div className="px-3 pb-3 pl-9 text-sm leading-relaxed">{children}</div>}
    </div>
  )
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

  function remember(key: string, value: string) {
    const next = { ...dismissed, [key]: value }
    setDismissed(next)
    saveDismissed(next)
  }

  return (
    <div className="space-y-2 mb-4">
      {showUpdate && (
        <CollapsibleNotice
          wrap="border-alpine-500 bg-alpine-100 text-alpine-700"
          Icon={Download}
          label="Mise à jour"
          title={`La version ${updateVersion} est disponible`}
          onDismiss={() => remember('__update__', updateVersion!)}
          dismissLabel="Masquer cette notification"
        >
          <p>
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
        </CollapsibleNotice>
      )}

      {visible.map((a) => {
        const style = STYLES[a.severity] ?? STYLES.info
        const effective = formatDate(a.effective_from)

        return (
          <CollapsibleNotice
            key={a.id}
            wrap={style.wrap}
            Icon={style.icon}
            label={style.label}
            title={a.title}
            onDismiss={() => remember(a.id, a.published_at ?? 'v1')}
            dismissLabel="Masquer cet avis"
          >
            {effective && (
              <p className="text-xs opacity-70 mb-1">En vigueur depuis le {effective}</p>
            )}
            <p>{a.body}</p>
            <a
              href={a.source_url}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1 text-xs mt-2 underline opacity-80 hover:opacity-100"
            >
              {a.source_name}
              <ExternalLink className="w-3 h-3" aria-hidden />
            </a>
          </CollapsibleNotice>
        )
      })}
    </div>
  )
}
