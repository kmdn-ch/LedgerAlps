// LedgerAlps — Diagnostic (Paramètres → Maintenance)
//
// Cet écran MONTRE, il ne répare pas. Une comptabilité incohérente se corrige
// par une écriture, pas par un bouton : réparer en silence effacerait la trace
// de ce qui s'est passé, ce que le CO art. 957a al. 2 ch. 5 interdit. Chaque
// constat dit donc ce qui ne va pas et ce qu'il faut faire.

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  ShieldCheck, AlertTriangle, XCircle, Info, RefreshCw, Loader2,
  Database, Lock, LockOpen,
} from 'lucide-react'
import { maintenanceApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { formatDate } from '@/utils'
import type { IntegrityReport, IntegrityFinding, SystemHealth } from '@/types'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

const SEVERITY: Record<IntegrityFinding['severity'], {
  icon: typeof Info; className: string; cle: Cle
}> = {
  error:   { icon: XCircle,       className: 'text-danger-700',  cle: 'dg.aCorriger' },
  warning: { icon: AlertTriangle, className: 'text-warning-700', cle: 'dg.aVerifier' },
  info:    { icon: Info,          className: 'text-alpine-600',  cle: 'dg.information' },
}

// Les capacités remontées par le serveur sont les mêmes que celles qui gardent
// les avis de conformité honnêtes. Les afficher évite d'avoir à croire sur
// parole ce que le produit protège.
const CAPABILITY_LABEL: Record<string, Cle> = {
  encrypted_backups:      'dg.capSauvegardes',
  encrypted_database:     'dg.capBase',
  native_tls:             'dg.capTLS',
  contact_anonymisation:  'dg.capAnonymisation',
  period_locking:         'dg.capVerrouillage',
  qr_bill_sps_2026:       'dg.capQR2026',
}

export function DiagnosticPanel() {
  const t = useT()
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
          <SectionTitle>{t('dg.controleCoherence')}</SectionTitle>
          <button
            onClick={() => integrity.refetch()}
            disabled={integrity.isFetching}
            className="btn-ghost btn-sm flex items-center gap-1.5"
          >
            {integrity.isFetching
              ? <Loader2 size={13} className="animate-spin" />
              : <RefreshCw size={13} />}
            {t('dg.relancer')}
          </button>
        </div>
        <p className="text-sm text-alpine-600 mb-3">
          {t('dg.controleAide')}
        </p>

        {integrity.isLoading && <LoadingSpinner />}
        {integrity.isError && <ErrorBanner message={t('dg.erreurControle')} />}

        {integrity.data?.clean && (
          <div className="flex items-start gap-2 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
            <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
            <span>
              {t('dg.aucuneIncoherence')}
              {integrity.data.checked_at
                && <> {t('dg.controleDu', { date: formatDate(integrity.data.checked_at) })}</>}.
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
                        <span className={`ml-2 text-xs font-normal ${s.className}`}>{t(s.cle)}</span>
                      </p>
                      <p className="mt-1 text-alpine-600">{f.detail}</p>

                      {/* Les pièces concernées, cliquables. Sans elles, un
                          constat oblige à les chercher une à une dans toute la
                          liste — et c'est là qu'on renonce à corriger. */}
                      {f.documents && f.documents.length > 0 && (
                        <ul className="mt-1.5 space-y-0.5">
                          {f.documents.map(doc => (
                            <li key={doc.id} className="flex flex-wrap items-baseline gap-x-2">
                              <Link
                                to={`/invoices/${doc.id}`}
                                className="font-mono text-accent-700 hover:text-accent-600 font-medium"
                              >
                                {doc.number}
                              </Link>
                              {doc.detail && (
                                <span className="text-alpine-500 text-xs">{doc.detail}</span>
                              )}
                            </li>
                          ))}
                          {f.count > f.documents.length && (
                            <li className="text-xs text-alpine-500">
                              {t('dg.autresPieces', { n: f.count - f.documents.length })}
                            </li>
                          )}
                        </ul>
                      )}

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
        <SectionTitle>{t('dg.etatSysteme')}</SectionTitle>
        {health.isLoading && <LoadingSpinner />}
        {health.isError && <ErrorBanner message={t('dg.erreurEtat')} />}

        {health.data && (
          <div className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <Stat icon={Database} label={t('dg.baseDeDonnees')} value={health.data.database.engine} />
              <Stat icon={Info}     label={t('dg.version')}       value={health.data.version} />
              <Stat
                icon={health.data.network.tls ? Lock : LockOpen}
                label={t('dg.accesReseau')}
                value={
                  health.data.network.loopback ? t('dg.machineUniquement')
                    : health.data.network.tls ? t('dg.reseauChiffre')
                    : t('dg.reseauEnClair')
                }
                warn={!health.data.network.loopback && !health.data.network.tls}
              />
            </div>

            {/* Volumétrie — utile pour juger si une lenteur est normale. */}
            <div className="text-sm">
              <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">{t('dg.volumetrie')}</p>
              <div className="flex flex-wrap gap-x-6 gap-y-1 text-alpine-700">
                {Object.entries(health.data.counts).map(([k, v]) => (
                  <span key={k}><span className="tabular-nums font-medium">{v}</span> {k.replace(/_/g, ' ')}</span>
                ))}
              </div>
            </div>

            {health.data.backups && (
              <div className="text-sm">
                <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">{t('dg.sauvegardes')}</p>
                <p className="text-alpine-700">
                  {t('dg.copiesDont', {
                    n: health.data.backups.count, c: health.data.backups.encrypted })}
                  {health.data.backups.newest
                    && <> {t('dg.plusRecenteDu', { date: formatDate(health.data.backups.newest) })}</>}.
                </p>
                {health.data.backups.count === 0 && (
                  <p className="text-warning-700 mt-1">
                    {t('dg.aucuneSauvegarde')}
                  </p>
                )}
              </div>
            )}

            {health.data.disk_encryption && (
              <DiskEncryption info={health.data.disk_encryption} />
            )}

            {/* Ce que le produit sait faire, tel que le serveur le déclare —
                la même table qui empêche les avis de conformité de mentir. */}
            <div className="text-sm">
              <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">{t('dg.protections')}</p>
              <ul className="space-y-0.5">
                {Object.entries(health.data.capabilities).map(([name, present]) => (
                  <li key={name} className="flex items-center gap-1.5 text-alpine-700">
                    {present
                      ? <ShieldCheck size={13} className="text-success-700 flex-shrink-0" />
                      : <XCircle size={13} className="text-alpine-400 flex-shrink-0" />}
                    <span className={present ? '' : 'text-alpine-500'}>
                      {CAPABILITY_LABEL[name] ? t(CAPABILITY_LABEL[name]) : name}
                    </span>
                  </li>
                ))}
              </ul>
            </div>

            {(health.data.login_lockouts ?? 0) > 0 && (
              <div className="text-sm">
                <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">{t('dg.securite')}</p>
                <p className="text-alpine-700">
                  {t('dg.verrouillages', { n: health.data.login_lockouts ?? 0 })}
                </p>
              </div>
            )}
          </div>
        )}
      </div>
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

// ── Chiffrement du disque ────────────────────────────────────────────────────
//
// L'ancienne version disait « Activez BitLocker » et s'arrêtait là. Sous
// Windows Famille, ce panneau n'existe pas : la fonctionnalité s'appelle
// « Chiffrement de l'appareil » et se trouve ailleurs. Un conseil que
// l'utilisateur ne peut pas suivre vaut à peine mieux que pas de conseil, et il
// abîme la confiance dans les autres.
//
// Le serveur envoie donc le nom de la fonctionnalité, l'édition, la marche à
// suivre et le lien direct. L'interface se contente de les afficher — elle ne
// devine rien sur la machine, ce qu'elle ne peut de toute façon pas faire.
function DiskEncryption({ info }: { info: NonNullable<SystemHealth['disk_encryption']> }) {
  const t = useT()
  const [open, setOpen] = useState(false)

  return (
    <div className="text-sm">
      <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">{t('dg.chiffrementDisque')}</p>

      {info.status === 'encrypted' ? (
        <p className="flex items-center gap-1.5 text-success-700">
          <ShieldCheck size={14} /> {t('dg.disqueActive')}
          {info.mechanism && <> ({info.mechanism})</>}
        </p>
      ) : info.status === 'not_encrypted' ? (
        <p className="text-warning-700">
          {t('dg.disqueNonActive', { fonction: info.feature ?? t('dg.disqueParDefaut') })}
        </p>
      ) : (
        <p className="text-alpine-600">
          {t('dg.disqueInconnu')}
        </p>
      )}

      {info.caveat && <p className="text-alpine-500 text-xs mt-1">{info.caveat}</p>}

      {/* La marche à suivre reste repliée quand elle n'est pas nécessaire : sur
          une machine déjà protégée, la dérouler serait du bruit. */}
      {info.steps && info.steps.length > 0 && (
        <div className="mt-2">
          <button
            type="button"
            onClick={() => setOpen(!open)}
            className="text-xs text-alpine-600 hover:text-alpine-900 underline underline-offset-2"
          >
            {open ? t('sv.masquer')
              : t(info.advisory ? 'dg.commentActiver' : 'dg.verifierFonction',
                  { fonction: info.feature ?? '' })}
          </button>

          {open && (
            <div className="mt-2 rounded-md bg-alpine-50 border border-alpine-200 p-3">
              {info.edition && (
                <p className="text-xs text-alpine-500 mb-2">{t('dg.systemeDetecte', { edition: info.edition })}</p>
              )}
              <ol className="list-decimal list-inside space-y-1 text-alpine-700 text-xs">
                {info.steps.map((s, i) => <li key={i}>{s}</li>)}
              </ol>
              {info.settings_uri && (
                <a
                  href={info.settings_uri}
                  className="inline-block mt-2 text-xs text-alpine-900 underline underline-offset-2"
                >
                  {t('dg.ouvrirReglage')}
                </a>
              )}
              <p className="text-alpine-500 text-xs mt-2">
                {t('dg.revenirActualiser')}
              </p>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
