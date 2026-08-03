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

const PAGE_SIZE = 25

const BREAK_LABEL: Record<ChainBreak['kind'], { icon: typeof ShieldAlert; title: string }> = {
  entry_altered:  { icon: FileWarning, title: 'Contenu modifié' },
  link_broken:    { icon: Link2Off,    title: 'Chaînage rompu' },
  sequence_gap:   { icon: Scissors,    title: 'Entrée supprimée' },
  anchor_invalid: { icon: Anchor,      title: 'Début de chaîne manquant' },
}

export function AuditTrailPanel() {
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
        <SectionTitle>Piste d'audit</SectionTitle>
        <p className="text-sm text-alpine-600 mb-3">
          Chaque comptabilisation d'écriture au journal ajoute un maillon à une chaîne
          d'empreintes SHA-256, chacune calculée à partir de la précédente
          (CO art. 957a al. 2 ch. 5). Retirer ou modifier une écriture rompt la chaîne
          de façon visible.
        </p>

        {/* ── Vérification ───────────────────────────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3">
          <div className="flex items-start justify-between gap-4">
            <div className="text-sm">
              <p className="font-medium">Vérifier l'intégrité de la chaîne</p>
              <p className="text-alpine-600 mt-0.5">
                Recalcule chaque empreinte et contrôle le chaînage ainsi que la
                continuité des numéros. Aucune donnée n'est modifiée.
              </p>
            </div>
            <button
              onClick={() => verify.mutate()}
              disabled={verify.isPending}
              className="btn-secondary btn-sm flex-shrink-0 flex items-center gap-1.5"
            >
              {verify.isPending
                ? <><Loader2 size={13} className="animate-spin" /> Vérification…</>
                : <><ShieldCheck size={13} /> Vérifier</>}
            </button>
          </div>

          {verify.isError && (
            <div className="mt-3">
              <ErrorBanner message="La vérification n'a pas pu s'exécuter." />
            </div>
          )}

          {report && report.verified && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5 text-sm">
              <ShieldCheck size={16} className="mt-0.5 flex-shrink-0 text-success-700" />
              <div>
                <p className="font-medium text-success-700">Chaîne intacte</p>
                <p className="text-alpine-600 mt-0.5">
                  {report.entries === 0
                    ? 'Aucune écriture comptabilisée à ce jour.'
                    : <>
                        {report.entries} entrée(s) vérifiée(s), numéros {report.first_sequence} à{' '}
                        {report.last_sequence}. Aucune n'a été modifiée ni supprimée.
                      </>}
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
                    Chaîne rompue — {report.breaks.length}
                    {report.truncated && '+'} anomalie(s) sur {report.entries} entrée(s)
                  </p>
                  <p className="text-alpine-700 mt-0.5">
                    Vos livres ne sont plus vérifiables au sens du CO art. 957a. N'écrasez
                    aucune sauvegarde : c'est la copie antérieure à la rupture qui permettra
                    de rétablir la comptabilité. Signalez-le avant votre prochaine clôture.
                  </p>
                </div>
              </div>
              {report.breaks.map((b, i) => {
                const meta = BREAK_LABEL[b.kind] ?? { icon: ShieldAlert, title: b.kind }
                return (
                  <div key={`${b.id}-${i}`} className="flex items-start gap-2 rounded-md border border-neutral-200 px-3 py-2 text-sm">
                    <meta.icon size={14} className="mt-0.5 flex-shrink-0 text-danger-700" />
                    <div>
                      <p className="font-medium">
                        {meta.title}
                        <span className="ml-2 text-xs font-normal text-alpine-500 tabular-nums">
                          n° {b.sequence_number} — {formatDate(b.created_at)}
                        </span>
                      </p>
                      <p className="text-alpine-600">{b.detail}</p>
                    </div>
                  </div>
                )
              })}
              {report.truncated && (
                <p className="text-xs text-alpine-500">
                  Seules les 100 premières anomalies sont listées.
                </p>
              )}
            </div>
          )}

          {report && report.legacy_entries > 0 && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5 text-sm">
              <FileWarning size={16} className="mt-0.5 flex-shrink-0 text-warning-700" />
              <div>
                <p className="font-medium text-warning-700">
                  {report.legacy_entries} entrée(s) écrite(s) avant la version 1.4.6
                </p>
                <p className="text-alpine-700 mt-0.5">
                  Leur chaînage est vérifié comme les autres — une suppression y resterait
                  visible. En revanche, leur empreinte individuelle ne peut pas être
                  recalculée : jusqu'à cette version, elle était calculée sur des valeurs
                  que LedgerAlps n'enregistrait pas. Le défaut est corrigé, mais il n'est
                  pas rattrapable pour l'existant. Nous préférons vous le dire plutôt que
                  d'afficher une garantie que nous ne pouvons pas tenir.
                </p>
              </div>
            </div>
          )}

          {report && (
            <p className="mt-3 text-xs text-alpine-500">
              Ce contrôle prouve que rien n'a été modifié ni retiré <em>entre</em> la première
              et la dernière entrée. Il ne peut pas établir qu'aucune écriture ne manque
              <em> après</em> la dernière : rien ne distingue une fin effacée d'une écriture
              jamais passée. C'est la comparaison avec vos sauvegardes qui répond à
              cette question.
            </p>
          )}
        </div>
      </div>

      {/* ── Journal ────────────────────────────────────────────────────────── */}
      <div>
        <SectionTitle>Entrées</SectionTitle>
        {logs.isLoading && <LoadingSpinner />}
        {logs.isError && <ErrorBanner message="La piste d'audit n'a pas pu être lue." />}

        {logs.data && logs.data.total === 0 && (
          <EmptyState
            icon={<ScrollText size={20} />}
            title="Aucune entrée"
            description="La piste se remplit à la comptabilisation d'une écriture au journal."
          />
        )}

        {logs.data && logs.data.total > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-neutral-200 text-left text-xs uppercase tracking-wider text-alpine-500">
                    <th className="py-2 pr-3 font-medium">N°</th>
                    <th className="py-2 pr-3 font-medium">Date</th>
                    <th className="py-2 pr-3 font-medium">Action</th>
                    <th className="py-2 pr-3 font-medium">Document</th>
                    <th className="py-2 pr-3 font-medium">Auteur</th>
                    <th className="py-2 font-medium">Empreinte</th>
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
                  Page {page + 1} sur {logs.data.pages} — {logs.data.total} entrée(s)
                </span>
                <div className="flex gap-1">
                  <button
                    onClick={() => setPage(p => Math.max(0, p - 1))}
                    disabled={page === 0}
                    className="btn-ghost btn-sm flex items-center gap-1"
                  >
                    <ChevronLeft size={13} /> Précédent
                  </button>
                  <button
                    onClick={() => setPage(p => p + 1)}
                    disabled={page + 1 >= logs.data.pages}
                    className="btn-ghost btn-sm flex items-center gap-1"
                  >
                    Suivant <ChevronRight size={13} />
                  </button>
                </div>
              </div>
            )}
          </>
        )}

        <p className="mt-3 text-xs text-alpine-500">
          Seule la comptabilisation d'une écriture au journal alimente cette piste — c'est
          la chaîne d'intégrité comptable, pas un journal d'activité. Les verrouillages de
          connexion sont enregistrés séparément : une adresse IP est une donnée personnelle
          soumise à une durée de conservation limitée (nLPD art. 6), quand une pièce
          comptable se conserve dix ans (CO art. 958f).
        </p>
      </div>
    </div>
  )
}
