// LedgerAlps — Maintenance & Système (onglet Paramètres)
//
// Cet écran MONTRE, il ne répare pas. Une comptabilité incohérente se corrige
// par une écriture, pas par un bouton : réparer en silence effacerait la trace
// de ce qui s'est passé, ce que le CO art. 957a al. 2 ch. 5 interdit. Chaque
// constat dit donc ce qui ne va pas et ce qu'il faut faire.

import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ShieldCheck, AlertTriangle, XCircle, Info, RefreshCw, Loader2,
  Database, Lock, LockOpen,
} from 'lucide-react'
import { maintenanceApi } from '@/api/client'
import { AuditTrailPanel } from '@/components/settings/AuditTrailPanel'
import { CompliancePanel } from '@/components/settings/CompliancePanel'
import { SecurityPanel } from '@/components/settings/SecurityPanel'
import { NetworkSettings } from '@/components/settings/NetworkSettings'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { formatDate } from '@/utils'
import type { IntegrityReport, IntegrityFinding, SystemHealth } from '@/types'

const SEVERITY: Record<IntegrityFinding['severity'], {
  icon: typeof Info; className: string; label: string
}> = {
  error:   { icon: XCircle,       className: 'text-danger-700',  label: 'À corriger' },
  warning: { icon: AlertTriangle, className: 'text-warning-700', label: 'À vérifier' },
  info:    { icon: Info,          className: 'text-alpine-600',  label: 'Information' },
}

// Les capacités remontées par le serveur sont les mêmes que celles qui gardent
// les avis de conformité honnêtes. Les afficher évite d'avoir à croire sur
// parole ce que le produit protège.
const CAPABILITY_LABEL: Record<string, string> = {
  encrypted_backups:      'Sauvegardes chiffrables',
  encrypted_database:     'Base de données chiffrée',
  native_tls:             'HTTPS natif',
  contact_anonymisation:  'Anonymisation des contacts (nLPD)',
  period_locking:         'Verrouillage de période',
  qr_bill_sps_2026:       'QR-facture SPS 2026',
}

export function MaintenancePanel() {
  const qc = useQueryClient()
  const integrity = useQuery<IntegrityReport>({
    queryKey: ['maintenance', 'integrity'],
    queryFn:  () => maintenanceApi.integrity().then(r => r.data),
  })
  const health = useQuery<SystemHealth>({
    queryKey: ['maintenance', 'health'],
    queryFn:  () => maintenanceApi.health().then(r => r.data),
  })

  return (
    <div className="space-y-6">
      {/* ── Intégrité & données ──────────────────────────────────────────── */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <SectionTitle>Contrôle de cohérence</SectionTitle>
          <button
            onClick={() => integrity.refetch()}
            disabled={integrity.isFetching}
            className="btn-ghost btn-sm flex items-center gap-1.5"
          >
            {integrity.isFetching
              ? <Loader2 size={13} className="animate-spin" />
              : <RefreshCw size={13} />}
            Relancer
          </button>
        </div>
        <p className="text-sm text-alpine-600 mb-3">
          Vérifie l'équilibre débit/crédit, les documents et les liens entre eux.
          Ce contrôle ne modifie rien : il signale ce qui demande votre décision.
        </p>

        {integrity.isLoading && <LoadingSpinner />}
        {integrity.isError && <ErrorBanner message="Le contrôle n'a pas pu s'exécuter." />}

        {integrity.data?.clean && (
          <div className="flex items-start gap-2 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
            <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
            <span>
              Aucune incohérence détectée
              {integrity.data.checked_at && <> — contrôle du {formatDate(integrity.data.checked_at)}</>}.
            </span>
          </div>
        )}

        {integrity.data && integrity.data.findings.length > 0 && (
          <div className="space-y-2">
            {integrity.data.findings.map(f => {
              const s = SEVERITY[f.severity] ?? SEVERITY.info
              return (
                <div key={f.check} className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
                  <div className="flex items-start gap-2">
                    <s.icon size={16} className={`mt-0.5 flex-shrink-0 ${s.className}`} />
                    <div className="flex-1">
                      <p className="font-medium">
                        {f.title}
                        <span className={`ml-2 text-xs font-normal ${s.className}`}>{s.label}</span>
                      </p>
                      <p className="mt-1 text-alpine-600">{f.detail}</p>
                      {f.action && <p className="mt-1.5">{f.action}</p>}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* ── Diagnostic & état du système ─────────────────────────────────── */}
      <div>
        <SectionTitle>État du système</SectionTitle>
        {health.isLoading && <LoadingSpinner />}
        {health.isError && <ErrorBanner message="L'état du système n'a pas pu être lu." />}

        {health.data && (
          <div className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <Stat icon={Database} label="Base de données" value={health.data.database.engine} />
              <Stat icon={Info}     label="Version"         value={health.data.version} />
              <Stat
                icon={health.data.network.tls ? Lock : LockOpen}
                label="Accès réseau"
                value={
                  health.data.network.loopback ? 'Cette machine uniquement'
                    : health.data.network.tls ? 'Réseau, chiffré (HTTPS)'
                    : 'Réseau, EN CLAIR'
                }
                warn={!health.data.network.loopback && !health.data.network.tls}
              />
            </div>

            {/* Volumétrie — utile pour juger si une lenteur est normale. */}
            <div className="text-sm">
              <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">Volumétrie</p>
              <div className="flex flex-wrap gap-x-6 gap-y-1 text-alpine-700">
                {Object.entries(health.data.counts).map(([k, v]) => (
                  <span key={k}><span className="tabular-nums font-medium">{v}</span> {k.replace(/_/g, ' ')}</span>
                ))}
              </div>
            </div>

            {health.data.backups && (
              <div className="text-sm">
                <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">Sauvegardes</p>
                <p className="text-alpine-700">
                  {health.data.backups.count} copie(s), dont{' '}
                  <strong>{health.data.backups.encrypted} chiffrée(s)</strong>
                  {health.data.backups.newest && <> — la plus récente du {formatDate(health.data.backups.newest)}</>}.
                </p>
                {health.data.backups.count === 0 && (
                  <p className="text-warning-700 mt-1">
                    Aucune sauvegarde. Le CO art. 958f impose de conserver vos pièces dix ans.
                  </p>
                )}
              </div>
            )}

            {health.data.disk_encryption && (
              <div className="text-sm">
                <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">Chiffrement du disque</p>
                {health.data.disk_encryption.status === 'encrypted' ? (
                  <p className="flex items-center gap-1.5 text-success-700">
                    <ShieldCheck size={14} /> Activé
                    {health.data.disk_encryption.mechanism && <> ({health.data.disk_encryption.mechanism})</>}
                  </p>
                ) : health.data.disk_encryption.status === 'not_encrypted' ? (
                  <p className="text-warning-700">
                    <strong>Non activé.</strong> La base de données est lisible sur un poste volé.
                    Activez BitLocker : c'est la seule mesure qui protège vraiment vos données au
                    repos, et LedgerAlps ne peut pas la prendre à votre place.
                  </p>
                ) : (
                  <p className="text-alpine-600">
                    Impossible à vérifier sur ce système. Assurez-vous que le disque est chiffré
                    (BitLocker, LUKS).
                  </p>
                )}
              </div>
            )}

            {/* Ce que le produit sait faire, tel que le serveur le déclare —
                la même table qui empêche les avis de conformité de mentir. */}
            <div className="text-sm">
              <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">Protections</p>
              <ul className="space-y-0.5">
                {Object.entries(health.data.capabilities).map(([name, present]) => (
                  <li key={name} className="flex items-center gap-1.5 text-alpine-700">
                    {present
                      ? <ShieldCheck size={13} className="text-success-700 flex-shrink-0" />
                      : <XCircle size={13} className="text-alpine-400 flex-shrink-0" />}
                    <span className={present ? '' : 'text-alpine-500'}>
                      {CAPABILITY_LABEL[name] ?? name}
                    </span>
                  </li>
                ))}
              </ul>
            </div>

            {(health.data.login_lockouts ?? 0) > 0 && (
              <div className="text-sm">
                <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">Sécurité</p>
                <p className="text-alpine-700">
                  {health.data.login_lockouts} verrouillage(s) de connexion enregistré(s) —
                  des tentatives répétées ont été bloquées.
                </p>
              </div>
            )}
          </div>
        )}
      </div>

      <CompliancePanel />

      <AuditTrailPanel />

      <SecurityPanel tlsEnabled={health.data?.network.tls ?? false} />

      <NetworkSettings onSaved={() => qc.invalidateQueries({ queryKey: ['maintenance', 'health'] })} />

      <p className="text-xs text-alpine-500">
        La console de rejeu ISO 20022 et le mode bac à sable arrivent dans une
        prochaine version — voir la roadmap.
      </p>
    </div>
  )
}

function Stat({ icon: Icon, label, value, warn }: {
  icon: typeof Info; label: string; value: string; warn?: boolean
}) {
  return (
    <div className={`rounded-md border px-3 py-2.5 ${
      warn ? 'border-warning-500 bg-warning-100' : 'border-neutral-200'
    }`}>
      <p className="text-xs text-alpine-500 flex items-center gap-1.5 mb-0.5">
        <Icon size={12} /> {label}
      </p>
      <p className="text-sm font-medium">{value}</p>
    </div>
  )
}
