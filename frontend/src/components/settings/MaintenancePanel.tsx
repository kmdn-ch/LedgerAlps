// LedgerAlps — Maintenance & Système (onglet Paramètres)
//
// L'onglet réunit tout ce qui ne relève ni de la saisie ni de la présentation :
// contrôler, prouver, effacer, sécuriser. C'est beaucoup, et l'empiler dans un
// seul défilement obligeait à parcourir la conformité pour atteindre le réseau.
//
// D'où cette navigation en cinq entrées, découpées par la QUESTION à laquelle
// chacune répond, pas par le module qui les implémente :
//
//   Diagnostic          — « est-ce que quelque chose ne va pas ? »
//   Conformité          — « puis-je le prouver à un tiers ? »
//   Piste d'audit       — « quelqu'un a-t-il modifié mes livres ? »
//   Données personnelles— « que sait LedgerAlps sur mes clients, et pour combien de temps ? »
//   Sécurité & réseau   — « qui peut atteindre cette installation ? »
//
// Chaque écran reste consultatif : il montre et il agit sur demande explicite,
// mais aucun ne répare en silence — une comptabilité incohérente se corrige par
// une écriture, pas par un bouton (CO art. 957a al. 2 ch. 5).

import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Stethoscope, ScrollText, ShieldCheck, UserRoundX, Network,
} from 'lucide-react'
import { maintenanceApi } from '@/api/client'
import { DiagnosticPanel } from '@/components/settings/DiagnosticPanel'
import { CompliancePanel } from '@/components/settings/CompliancePanel'
import { AuditTrailPanel } from '@/components/settings/AuditTrailPanel'
import { PersonalDataPanel } from '@/components/settings/PersonalDataPanel'
import { SecurityPanel } from '@/components/settings/SecurityPanel'
import { NetworkSettings } from '@/components/settings/NetworkSettings'
import type { IntegrityReport, SystemHealth } from '@/types'

type SectionKey = 'diagnostic' | 'compliance' | 'audit' | 'personal' | 'security'

const SECTIONS: {
  key: SectionKey
  label: string
  icon: typeof Stethoscope
  hint: string
}[] = [
  { key: 'diagnostic', label: 'Diagnostic',          icon: Stethoscope, hint: 'Cohérence des données et état du système' },
  { key: 'compliance', label: 'Conformité',          icon: ShieldCheck, hint: 'Exercices, clôture, attestation et archives' },
  { key: 'audit',      label: "Piste d'audit",       icon: ScrollText,  hint: "Chaîne d'intégrité des écritures (CO art. 957a)" },
  { key: 'personal',   label: 'Données personnelles', icon: UserRoundX, hint: 'Rétention et anonymisation (nLPD)' },
  { key: 'security',   label: 'Sécurité & réseau',   icon: Network,     hint: "Clé de signature et adresse d'écoute" },
]

export function MaintenancePanel() {
  const qc = useQueryClient()
  const [section, setSection] = useState<SectionKey>('diagnostic')

  // L'état de santé sert à deux endroits : la pastille du Diagnostic et le
  // réglage TLS de la section Sécurité. Une seule requête, partagée par la clé.
  const health = useQuery<SystemHealth>({
    queryKey: ['maintenance', 'health'],
    queryFn:  () => maintenanceApi.health().then(r => r.data),
  })
  // Le nombre d'anomalies est affiché sur l'entrée Diagnostic : sans cela,
  // rien n'inciterait à l'ouvrir, et un contrôle qu'on n'ouvre pas ne sert à rien.
  const integrity = useQuery<IntegrityReport>({
    queryKey: ['maintenance', 'integrity'],
    queryFn:  () => maintenanceApi.integrity().then(r => r.data),
  })

  const errors = integrity.data?.findings.filter(f => f.severity === 'error').length ?? 0
  const warnings = integrity.data?.findings.filter(f => f.severity === 'warning').length ?? 0

  const current = SECTIONS.find(s => s.key === section) ?? SECTIONS[0]

  return (
    <div>
      {/* ── Navigation ───────────────────────────────────────────────────── */}
      <div className="flex flex-wrap gap-1 border-b border-neutral-200 mb-4">
        {SECTIONS.map(s => {
          const active = s.key === section
          const badge = s.key === 'diagnostic' && (errors > 0 || warnings > 0)
          return (
            <button
              key={s.key}
              onClick={() => setSection(s.key)}
              title={s.hint}
              className={`relative flex items-center gap-1.5 px-3 py-2 text-sm font-medium
                border-b-2 -mb-px transition-colors ${
                active
                  ? 'border-alpine-700 text-alpine-900'
                  : 'border-transparent text-alpine-500 hover:text-alpine-700'
              }`}
            >
              <s.icon size={14} />
              {s.label}
              {badge && (
                <span className={`ml-0.5 rounded-full px-1.5 py-0.5 text-xs tabular-nums ${
                  errors > 0 ? 'bg-danger-100 text-danger-700' : 'bg-warning-100 text-warning-700'
                }`}>
                  {errors > 0 ? errors : warnings}
                </span>
              )}
            </button>
          )
        })}
      </div>

      {/* Une ligne qui rappelle à quoi sert la section ouverte. Les titres seuls
          — « Conformité », « Diagnostic » — ne disent pas ce qu'on y fait. */}
      <p className="text-sm text-alpine-500 mb-4">{current.hint}</p>

      {/* ── Contenu ──────────────────────────────────────────────────────── */}
      {section === 'diagnostic' && <DiagnosticPanel />}
      {section === 'compliance' && <CompliancePanel />}
      {section === 'audit' && <AuditTrailPanel />}
      {section === 'personal' && <PersonalDataPanel />}
      {section === 'security' && (
        <div className="space-y-6">
          <SecurityPanel tlsEnabled={health.data?.network.tls ?? false} />
          <NetworkSettings
            onSaved={() => qc.invalidateQueries({ queryKey: ['maintenance', 'health'] })}
          />
        </div>
      )}

      <p className="mt-6 text-xs text-alpine-500">
        La console de rejeu ISO 20022 et le mode bac à sable arrivent dans une
        prochaine version — voir la roadmap.
      </p>
    </div>
  )
}
