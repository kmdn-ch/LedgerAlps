// LedgerAlps — Piste d'audit (Paramètres → Maintenance)
//
// Ce que cet écran montre est étroit et doit le rester : la chaîne d'empreintes
// du CO art. 957a, alimentée par la SEULE comptabilisation d'une écriture au
// journal. Ce n'est pas un journal d'activité — il n'y a ici ni connexion, ni
// consultation, ni modification de contact. Le dire est un choix : laisser
// croire que tout est tracé serait un mensonge sur lequel quelqu'un finirait
// par s'appuyer.
//
// Le bouton de vérification est la raison d'être de l'écran. Vérifier une
// entrée isolée ne détecte que la modification de son contenu ; supprimer une
// ligne laisse l'empreinte de toutes les autres parfaitement valide. Seul le
// parcours du chaînage et des numéros de séquence rend une suppression visible.

import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  ShieldCheck, ShieldAlert, Link2Off, FileWarning, Scissors, Anchor,
  Loader2, ChevronLeft, ChevronRight, ScrollText,
} from 'lucide-react'
import { auditApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner, EmptyState } from '@/components/ui'
import { formatDate } from '@/utils'
import type { AuditLogPage, ChainReport, ChainBreak } from '@/types'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'

const PAGE_SIZE = 25

const BREAK_LABEL: Record<ChainBreak['kind'], { icon: typeof ShieldAlert; cle: Cle }> = {
  entry_altered:  { icon: FileWarning, cle: 'at.contenuModifie' },
  link_broken:    { icon: Link2Off,    cle: 'at.chainageRompu' },
  sequence_gap:   { icon: Scissors,    cle: 'at.entreeSupprimee' },
  anchor_invalid: { icon: Anchor,      cle: 'at.debutManquant' },
}

/**
 * Les champs qu'une action a réellement modifiés.
 *
 * Ils voyagent dans l'état « après » du maillon, sous `champs_modifies`, et y
 * sont calculés côté serveur AVANT le masquage des données personnelles. C'est
 * ce qui permet d'afficher « l'IBAN a changé » alors que les deux IBAN sont
 * masqués et donc identiques dans la piste : sans cette liste, la modification
 * la plus sensible du produit serait la seule invisible.
 *
 * Un état illisible ne doit pas casser la ligne : la piste reste consultable
 * même si un maillon a été écrit par une version qui n'existe plus.
 */
function champsModifies(apres?: string): string[] {
  if (!apres) return []
  try {
    const etat = JSON.parse(apres) as Record<string, unknown>
    const liste = etat['champs_modifies']
    return Array.isArray(liste) ? liste.filter(v => typeof v === 'string') as string[] : []
  } catch {
    return []
  }
}

export function AuditTrailPanel() {
  const t = useT()
  const [page, setPage] = useState(0)

  const logs = useQuery<AuditLogPage>({
    queryKey: ['audit-logs', page],
    queryFn: () =>
      auditApi
        .list({ limit: PAGE_SIZE, offset: page * PAGE_SIZE, order: 'desc' })
        .then(r => r.data),
  })

  // Volontairement une mutation et non une requête : le parcours lit toute la
  // chaîne, il se déclenche donc quand on le demande, pas à l'ouverture de
  // l'onglet.
  const verify = useMutation<ChainReport>({
    mutationFn: () => auditApi.verifyChain().then(r => r.data),
  })

  const report = verify.data

  return (
    <div className="space-y-6">
      <div>
        <SectionTitle>{t('at.titre')}</SectionTitle>
        <p className="text-sm text-alpine-600 mb-3">
          {t('at.introduction')}
        </p>

        {/* ── Vérification ───────────────────────────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3">
          <div className="flex items-start justify-between gap-4">
            <div className="text-sm">
              <p className="font-medium">{t('at.verifierChaine')}</p>
              <p className="text-alpine-600 mt-0.5">
                {t('at.verifierAide')}
              </p>
            </div>
            <button
              onClick={() => verify.mutate()}
              disabled={verify.isPending}
              className="btn-secondary btn-sm flex-shrink-0 flex items-center gap-1.5"
            >
              {verify.isPending
                ? <><Loader2 size={13} className="animate-spin" /> {t('at.verificationEnCours')}</>
                : <><ShieldCheck size={13} /> {t('at.verifier')}</>}
            </button>
          </div>

          {verify.isError && (
            <div className="mt-3">
              <ErrorBanner message={t('at.echecVerification')} />
            </div>
          )}

          {report && report.verified && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5 text-sm">
              <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
              <div>
                <p className="font-medium text-success-700">{t('at.chaineIntacte')}</p>
                <p className="text-alpine-600 mt-0.5">
                  {report.entries === 0
                    ? t('at.aucuneEcriture')
                    : t('at.entreesVerifiees', {
                        n: report.entries,
                        debut: report.first_sequence,
                        fin: report.last_sequence,
                      })}
                </p>
              </div>
            </div>
          )}

          {report && !report.verified && (
            <div className="mt-3 space-y-2">
              <div className="flex items-start gap-2 rounded-md border border-danger-500 bg-danger-100 px-3 py-2.5 text-sm">
                <ShieldAlert size={16} className="mt-0.5 flex-shrink-0 text-danger-700" />
                <div>
                  <p className="font-medium text-danger-700">
                    {t('at.chaineRompue', {
                      a: report.breaks.length,
                      plus: report.truncated ? '+' : '',
                      n: report.entries,
                    })}
                  </p>
                  <p className="text-alpine-700 mt-0.5">
                    {t('at.chaineRompueAide')}
                  </p>
                </div>
              </div>
              {report.breaks.map((b, i) => {
                const meta = BREAK_LABEL[b.kind]
                return (
                  <div key={`${b.id}-${i}`} className="flex items-start gap-2 rounded-md border border-neutral-200 px-3 py-2 text-sm">
                    {(() => { const Icone = meta?.icon ?? ShieldAlert
                      return <Icone size={14} className="mt-0.5 flex-shrink-0 text-danger-700" /> })()}
                    <div>
                      <p className="font-medium">
                        {meta ? t(meta.cle) : b.kind}
                        <span className="ml-2 text-xs font-normal text-alpine-500 tabular-nums">
                          {t('at.numeroEtDate', {
                            numero: b.sequence_number, date: formatDate(b.created_at) })}
                        </span>
                      </p>
                      <p className="text-alpine-600">{b.detail}</p>
                    </div>
                  </div>
                )
              })}
              {report.truncated && (
                <p className="text-xs text-alpine-500">
                  {t('at.centPremieres')}
                </p>
              )}
            </div>
          )}

          {report && report.legacy_entries > 0 && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5 text-sm">
              <FileWarning size={16} className="mt-0.5 flex-shrink-0 text-warning-700" />
              <div>
                <p className="font-medium text-warning-700">
                  {t('at.entreesAnciennes', { n: report.legacy_entries })}
                </p>
                <p className="text-alpine-700 mt-0.5">
                  {t('at.entreesAnciennesAide')}
                </p>
              </div>
            </div>
          )}

          {report && (
            <p className="mt-3 text-xs text-alpine-500">
              {t('at.porteeDuControle')}
            </p>
          )}
        </div>
      </div>

      {/* ── Journal ────────────────────────────────────────────────────────── */}
      <div>
        <SectionTitle>{t('at.entrees')}</SectionTitle>
        {logs.isLoading && <LoadingSpinner />}
        {logs.isError && <ErrorBanner message={t('at.echecLecture')} />}

        {logs.data && logs.data.total === 0 && (
          <EmptyState
            icon={<ScrollText size={20} />}
            title={t('at.aucuneEntree')}
            description={t('at.aucuneEntreeAide')}
          />
        )}

        {logs.data && logs.data.total > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-neutral-200 text-left text-xs uppercase tracking-wider text-alpine-500">
                    <th className="py-2 pr-3 font-medium">{t('at.colNumero')}</th>
                    <th className="py-2 pr-3 font-medium">{t('fact.colDate')}</th>
                    <th className="py-2 pr-3 font-medium">{t('at.colAction')}</th>
                    <th className="py-2 pr-3 font-medium">{t('at.colDocument')}</th>
                    <th className="py-2 pr-3 font-medium">{t('at.colAuteur')}</th>
                    <th className="py-2 pr-3 font-medium">{t('at.colChamps')}</th>
                    <th className="py-2 font-medium">{t('at.colEmpreinte')}</th>
                  </tr>
                </thead>
                <tbody>
                  {logs.data.items.map(l => (
                    <tr key={l.id} className="border-b border-neutral-100">
                      <td className="py-2 pr-3 tabular-nums text-alpine-500">{l.sequence_number}</td>
                      <td className="py-2 pr-3 whitespace-nowrap">{formatDate(l.created_at, 'dd.MM.yyyy HH:mm')}</td>
                      <td className="py-2 pr-3">{l.action}</td>
                      <td className="py-2 pr-3 font-mono text-xs">{l.record_id}</td>
                      <td className="py-2 pr-3">{l.user_name || <span className="text-alpine-400">—</span>}</td>
                      <td className="py-2 pr-3">
                        {(() => {
                          const champs = champsModifies(l.after_state)
                          // Une création ne remplace rien : afficher « aucun
                          // champ modifié » y serait faux, un tiret dit
                          // « sans objet » sans rien affirmer.
                          if (champs.length === 0) {
                            return <span className="text-alpine-400">—</span>
                          }
                          return (
                            <span className="flex flex-wrap gap-1">
                              {champs.map(c => (
                                <span key={c}
                                      className="rounded bg-alpine-100 px-1.5 py-0.5 font-mono text-[11px] text-alpine-700">
                                  {c}
                                </span>
                              ))}
                            </span>
                          )
                        })()}
                      </td>
                      <td className="py-2 font-mono text-xs text-alpine-500" title={l.entry_hash}>
                        {l.entry_hash.slice(0, 12)}…
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {logs.data.pages > 1 && (
              <div className="flex items-center justify-between mt-3 text-sm">
                <span className="text-alpine-500">
                  {t('at.pagination', {
                    page: page + 1, total: logs.data.pages, n: logs.data.total })}
                </span>
                <div className="flex gap-1">
                  <button
                    onClick={() => setPage(p => Math.max(0, p - 1))}
                    disabled={page === 0}
                    className="btn-ghost btn-sm flex items-center gap-1"
                  >
                    <ChevronLeft size={13} /> {t('at.precedent')}
                  </button>
                  <button
                    onClick={() => setPage(p => p + 1)}
                    disabled={page + 1 >= logs.data.pages}
                    className="btn-ghost btn-sm flex items-center gap-1"
                  >
                    {t('at.suivant')} <ChevronRight size={13} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}

        <p className="mt-3 text-xs text-alpine-500">
          {t('at.mentionPortee')}
        </p>
      </div>
    </div>
  )
}
