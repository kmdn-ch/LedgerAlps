// LedgerAlps — Journal comptable
//
// Cette page ne fonctionnait pas, et pas à moitié : elle ne fonctionnait pas du
// tout. Trois défauts indépendants, chacun suffisant.
//
// 1. La liste ne demandait rien au serveur. Sa source était une promesse résolue
//    sur un tableau vide — un vestige de maquette. Le journal restait donc vide
//    même après une écriture réussie.
//
// 2. Le formulaire parlait une autre langue que l'API. Il envoyait
//    `debit_account` / `credit_account` / `amount` là où le serveur attend un
//    compte et un montant au débit ou au crédit, et il n'envoyait qu'une ligne
//    là où une écriture en comporte au moins deux. Chaque enregistrement
//    répondait 422.
//
// 3. Le message d'erreur accusait toujours la partie double, quelle que soit la
//    cause réelle. Un numéro de compte inexistant, une date mal formée, un
//    montant manquant : tout devenait « vérifiez la partie double ». Un
//    avertissement qui se trompe de cause est pire qu'aucun.
//
// La saisie garde la forme que le métier attend — un débit, un crédit, un
// montant sur la même ligne — et c'est le navigateur qui la traduit en lignes
// comptables. Le contrôle d'équilibre reste ENTIÈREMENT du côté serveur : ce
// qui s'affiche ici n'est qu'une aide à la saisie, jamais l'autorité.

import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Plus, Trash2, CheckCircle, ChevronRight, ChevronDown, Shield, Loader2, Info,
} from 'lucide-react'
import { journalApi, accountsApi, settingsApi } from '@/api/client'
import {
  PageHeader, LoadingSpinner, EmptyState, ErrorBanner, ConfirmDialog,
} from '@/components/ui'
import { formatDate, formatCHF } from '@/utils'
import { refusalMessage } from '@/utils/refusal'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'
import { useUnsavedGuard } from '@/hooks/useUnsavedGuard'
import type {
  Account, JournalEntry, JournalEntryDetail, JournalLineCreate,
} from '@/types'

const STATUS_CLE: Record<string, Cle> = {
  draft:    'statut.brouillon',
  posted:   'statut.comptabilisee',
  reversed: 'jr.statutContrepassee',
}
const STATUS_CLASS: Record<string, string> = {
  draft:    'badge-draft',
  posted:   'badge-paid',
  reversed: 'badge-cancelled',
}

/** Une ligne de SAISIE : un débit, un crédit, un montant. */
interface Row {
  debit: string
  credit: string
  amount: string
  label: string
}

const emptyRow = (): Row => ({ debit: '', credit: '', amount: '', label: '' })

/**
 * Traduit la saisie en lignes comptables.
 *
 * Une ligne dont les deux comptes sont remplis devient une paire équilibrée.
 * Une ligne dont un seul côté est rempli devient une ligne simple — c'est ce qui
 * permet une ventilation, par exemple une vente répartie entre produit et TVA
 * sans avoir à répéter le compte de contrepartie.
 */
function toLines(rows: Row[]): JournalLineCreate[] {
  const lines: JournalLineCreate[] = []
  rows.forEach((r, i) => {
    const amount = parseFloat(r.amount.replace(',', '.'))
    if (!isFinite(amount) || amount <= 0) return
    if (r.debit.trim()) {
      lines.push({
        account_code: r.debit.trim(), debit_amount: amount,
        description: r.label.trim() || undefined, sequence: i * 2,
      })
    }
    if (r.credit.trim()) {
      lines.push({
        account_code: r.credit.trim(), credit_amount: amount,
        description: r.label.trim() || undefined, sequence: i * 2 + 1,
      })
    }
  })
  return lines
}

export function JournalPage() {
  const t = useT()
  const peutEcrire = useCanWrite()
  const qc = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [page, setPage] = useState(1)
  const [openId, setOpenId] = useState<string | null>(null)
  const [toPost, setToPost] = useState<JournalEntry | null>(null)
  const [error, setError] = useState<string | null>(null)

  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10))
  const [description, setDescription] = useState('')
  const [rows, setRows] = useState<Row[]>([emptyRow()])

  // Le plan comptable sert de dictionnaire : il traduit un numéro en nom sous
  // le champ, et alimente la liste de choix. Sans lui, il faut connaître par
  // cœur des numéros à quatre chiffres.
  const { data: accounts = [] } = useQuery<Account[]>({
    queryKey: ['accounts'],
    queryFn:  () => accountsApi.list().then(r => r.data),
    staleTime: 5 * 60_000,
  })
  const byCode = useMemo(() => {
    const m = new Map<string, Account>()
    accounts.forEach(a => m.set(a.code, a))
    return m
  }, [accounts])

  // Le réglage de comptabilisation automatique se lit ICI parce que c'est ici
  // que son absence se voit : un journal vide après avoir envoyé des factures
  // ressemble à une panne, alors que c'est un choix — celui des installations
  // créées avant que ce réglage n'existe.
  const company = useQuery<{ auto_post_invoices?: boolean }>({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
    staleTime: 5 * 60_000,
  })

  const list = useQuery<{ items: JournalEntry[]; total: number; pages: number }>({
    queryKey: ['journal', page],
    queryFn:  () => journalApi.list({ page, page_size: 25 }).then(r => r.data),
  })

  const detail = useQuery<JournalEntryDetail>({
    queryKey: ['journal-entry', openId],
    queryFn:  () => journalApi.get(openId as string).then(r => r.data),
    enabled:  openId !== null,
  })

  const dirty = showForm && (description.trim() !== '' ||
    rows.some(r => r.debit || r.credit || r.amount || r.label))
  useUnsavedGuard(dirty)

  const resetForm = () => {
    setDescription(''); setRows([emptyRow()]); setError(null)
    setDate(new Date().toISOString().slice(0, 10))
  }

  const create = useMutation({
    mutationFn: () => journalApi.create({ date, description, lines: toLines(rows) }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['journal'] })
      qc.invalidateQueries({ queryKey: ['trial-balance'] })
      resetForm(); setShowForm(false)
    },
    // Le refus du serveur est affiché TEL QUEL. Il nomme la ligne et la cause :
    // « ligne 1 : le compte 10 n'existe pas dans le plan comptable », « écart
    // 90.00 ». Le remplacer par un message générique était le défaut d'origine.
    onError: (e) => setError(refusalMessage(e, "L'écriture n'a pas pu être enregistrée.")),
  })

  const post = useMutation({
    mutationFn: (id: string) => journalApi.post(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['journal'] })
      qc.invalidateQueries({ queryKey: ['journal-entry'] })
      qc.invalidateQueries({ queryKey: ['trial-balance'] })
      setToPost(null); setError(null)
    },
    onError: (e) => {
      setToPost(null)
      setError(refusalMessage(e, "L'écriture n'a pas pu être comptabilisée."))
    },
  })

  const lines = toLines(rows)
  const totalDebit  = lines.reduce((s, l) => s + (l.debit_amount ?? 0), 0)
  const totalCredit = lines.reduce((s, l) => s + (l.credit_amount ?? 0), 0)
  const ecart = Math.round((totalDebit - totalCredit) * 100) / 100
  const equilibree = ecart === 0 && lines.length >= 2

  // Les numéros inconnus sont signalés pendant la frappe plutôt qu'au refus :
  // le plan comptable est déjà chargé, attendre l'aller-retour n'apporte rien.
  const inconnus = rows.flatMap(r => [r.debit, r.credit])
    .map(c => c.trim()).filter(c => c !== '' && !byCode.has(c))

  const peutEnregistrer = description.trim() !== '' && equilibree &&
    inconnus.length === 0 && !create.isPending

  const setRow = (i: number, patch: Partial<Row>) =>
    setRows(rs => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))

  const items = list.data?.items ?? []

  return (
    <div>
      <PageHeader
        title={t('nav.journal')}
        aide={t('aide.journal')}
        subtitle={t('jr.sousTitre')}
        actions={
          peutEcrire ? (
          <button onClick={() => { setShowForm(v => !v); setError(null) }} className="btn-primary">
            <Plus size={15} /> {t('jr.nouvelle')}
          </button>
          ) : (
            <span className="text-xs text-alpine-500">{t(RAISON_LECTURE_SEULE)}</span>
          )
        }
      />

      {error && !showForm && <div className="mb-4"><ErrorBanner message={error} /></div>}

      {company.data && company.data.auto_post_invoices === false && (
        <div className="mb-4 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          <p className="font-medium flex items-center gap-1.5">
            <Info size={15} className="text-alpine-500" />
            {t('jn.pasAutomatique')}
          </p>
          <p className="text-alpine-600 mt-1">
            La comptabilisation automatique est éteinte sur cette installation : envoyer une
            facture ne produit aucune écriture, et il faut les saisir soi-même. C&rsquo;est le
            réglage par défaut des installations créées avant qu&rsquo;il n&rsquo;existe, pour
            ne pas compter deux fois ce qui était déjà saisi à la main.
            {' '}Pour l&rsquo;activer : Paramètres → Facturation.
          </p>
        </div>
      )}

      {showForm && (
        <div className="card mb-5">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('jn.ecritureManuelle')}</h2>
            <span className="text-xs text-alpine-400">
              Le serveur vérifie la partie double : total débit = total crédit
            </span>
          </div>

          <div className="card-body grid grid-cols-1 sm:grid-cols-3 gap-4 pb-4">
            <div className="sm:col-span-2">
              <label className="label" htmlFor="je-desc">{t('jr.description')}</label>
              <input id="je-desc" className="input" value={description}
                     onChange={e => setDescription(e.target.value)}
                     placeholder={t('jr.descExemple')} />
            </div>
            <div>
              <label className="label" htmlFor="je-date">{t('jr.date')}</label>
              <input id="je-date" type="date" className="input" value={date}
                     onChange={e => setDate(e.target.value)} />
            </div>
          </div>

          <div className="px-6 pb-4">
            <div className="flex items-center justify-between mb-2">
              <span className="label mb-0">{t('jr.lignes')}</span>
              <button type="button" className="btn-ghost btn-sm"
                      onClick={() => setRows(rs => [...rs, emptyRow()])}>
                <Plus size={13} /> {t('jn.ajouterLigne')}
              </button>
            </div>

            {/* La liste de choix vit une seule fois, hors du tableau : la
                répéter par ligne ferait autant de copies du plan comptable. */}
            <datalist id="plan-comptable">
              {accounts.map(a => (
                <option key={a.id} value={a.code}>{a.name}</option>
              ))}
            </datalist>

            <div className="border border-alpine-200 rounded-lg overflow-x-auto">
              <table className="w-full text-[13px]">
                <thead>
                  <tr className="bg-alpine-50 border-b border-alpine-200">
                    <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">{t('jr.compteDebite')}</th>
                    <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">{t('jr.compteCredite')}</th>
                    <th className="px-3 py-2 text-right text-xs font-semibold text-alpine-600">{t('jr.montantCHF')}</th>
                    <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">{t('jr.libelle')}</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r, i) => (
                    <tr key={i} className="border-t border-alpine-100 align-top">
                      <td className="px-2 py-2 w-40">
                        <input className="input font-mono" list="plan-comptable" placeholder="1000"
                               value={r.debit} onChange={e => setRow(i, { debit: e.target.value })} />
                        <AccountHint code={r.debit} byCode={byCode} />
                      </td>
                      <td className="px-2 py-2 w-40">
                        <input className="input font-mono" list="plan-comptable" placeholder="3200"
                               value={r.credit} onChange={e => setRow(i, { credit: e.target.value })} />
                        <AccountHint code={r.credit} byCode={byCode} />
                      </td>
                      <td className="px-2 py-2 w-32">
                        <input type="number" step="0.05" min="0" inputMode="decimal"
                               className="input text-right font-mono tabular-nums"
                               value={r.amount} onChange={e => setRow(i, { amount: e.target.value })} />
                      </td>
                      <td className="px-2 py-2">
                        <input className="input" placeholder={t('jr.facultatif')}
                               value={r.label} onChange={e => setRow(i, { label: e.target.value })} />
                      </td>
                      <td className="px-2 py-2 w-10">
                        {rows.length > 1 && (
                          <button type="button" onClick={() => setRows(rs => rs.filter((_, j) => j !== i))}
                                  className="btn-ghost btn-sm p-1 text-danger-500"
                                  aria-label={`Retirer la ligne ${i + 1}`}>
                            <Trash2 size={13} />
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
                <tfoot>
                  <tr className="border-t border-alpine-200 bg-alpine-50">
                    <td className="px-3 py-2 text-xs text-alpine-600" colSpan={2}>
                      {lines.length} ligne(s) comptable(s)
                    </td>
                    <td className="px-3 py-2 text-right font-mono tabular-nums text-xs">
                      <div>D {formatCHF(totalDebit)}</div>
                      <div>C {formatCHF(totalCredit)}</div>
                    </td>
                    <td className="px-3 py-2 text-xs" colSpan={2}>
                      {equilibree
                        ? <span className="text-success-700">{t('jn.equilibree')}</span>
                        : lines.length === 0
                          ? <span className="text-alpine-500">{t('jr.renseignez')}</span>
                          : <span className="text-warning-700">
                              Écart de {formatCHF(Math.abs(ecart))} — l&rsquo;écriture ne sera pas acceptée.
                            </span>}
                    </td>
                  </tr>
                </tfoot>
              </table>
            </div>

            <p className="text-xs text-alpine-500 mt-2">
              {t('jn.ventilationAide')}
            </p>

            {inconnus.length > 0 && (
              <p className="text-xs text-danger-700 mt-2">
                Numéro{inconnus.length > 1 ? 's' : ''} inconnu{inconnus.length > 1 ? 's' : ''} au
                plan comptable : {[...new Set(inconnus)].join(', ')}.
              </p>
            )}
          </div>

          {error && <div className="px-6 pb-2"><ErrorBanner message={error} /></div>}

          <div className="card-footer flex justify-end gap-3">
            <button type="button" className="btn-secondary"
                    onClick={() => { setShowForm(false); resetForm() }}>
              {t('action.annuler')}
            </button>
            <button type="button" className="btn-primary flex items-center gap-1.5"
                    disabled={!peutEnregistrer} onClick={() => { setError(null); create.mutate() }}>
              {create.isPending ? <Loader2 size={14} className="animate-spin" /> : <CheckCircle size={15} />}
              Enregistrer le brouillon
            </button>
          </div>
        </div>
      )}

      {/* Liste */}
      <div className="table-wrapper">
        <table className="table">
          <thead>
            <tr>
              <th style={{ width: '34px' }} />
              <th>{t('jr.colRef')}</th>
              <th>{t('fact.colDate')}</th>
              <th>{t('jr.colDescription')}</th>
              <th>{t('jr.colAuteur')}</th>
              <th className="text-right">{t('jr.montantCHF')}</th>
              <th>{t('fact.colStatut')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.isLoading && <tr><td colSpan={8}><LoadingSpinner /></td></tr>}
            {list.isError && (
              <tr><td colSpan={8}>
                <ErrorBanner message="Le journal n'a pas pu être lu." />
              </td></tr>
            )}
            {!list.isLoading && !list.isError && items.length === 0 && (
              <tr><td colSpan={8}>
                <EmptyState
                  title={t('jr.vide')}
                  description={t('jr.videAide')}
                />
              </td></tr>
            )}
            {items.map(e => (
              <EntryRow
                key={e.id}
                entry={e}
                open={openId === e.id}
                detail={openId === e.id ? detail.data : undefined}
                loadingDetail={openId === e.id && detail.isLoading}
                onToggle={() => setOpenId(openId === e.id ? null : e.id)}
                onPost={() => setToPost(e)}
              />
            ))}
          </tbody>
        </table>
      </div>

      {(list.data?.pages ?? 1) > 1 && (
        <div className="flex items-center justify-between mt-3 text-sm">
          <span className="text-alpine-600">
            {list.data?.total} écriture(s) — page {page} sur {list.data?.pages}
          </span>
          <div className="flex gap-2">
            <button className="btn-secondary btn-sm" disabled={page <= 1}
                    onClick={() => setPage(p => p - 1)}>{t('jr.precedente')}</button>
            <button className="btn-secondary btn-sm" disabled={page >= (list.data?.pages ?? 1)}
                    onClick={() => setPage(p => p + 1)}>{t('jr.suivante')}</button>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={toPost !== null}
        title={t('jr.confirmTitre', { ref: toPost?.reference ?? '' })}
        consequences={[
          t('jr.confirmScellee'),
          t('jr.confirmEntre'),
          t('jr.confirmCorrection'),
        ]}
        reassurance={t('jr.confirmBrouillon')}
        confirmLabel={t('ach.comptabiliser')}
        tone="danger"
        busy={post.isPending}
        onConfirm={() => toPost && post.mutate(toPost.id)}
        onCancel={() => setToPost(null)}
      />
    </div>
  )
}

/** Le nom du compte sous le champ : la seule façon de voir qu'on s'est trompé. */
function AccountHint({ code, byCode }: { code: string; byCode: Map<string, Account> }) {
  const c = code.trim()
  if (c === '') return null
  const a = byCode.get(c)
  return (
    <span className={`block text-[11px] mt-0.5 truncate ${a ? 'text-alpine-500' : 'text-danger-600'}`}>
      {a ? a.name : 'numéro inconnu'}
    </span>
  )
}

interface EntryRowProps {
  entry: JournalEntry
  open: boolean
  detail?: JournalEntryDetail
  loadingDetail: boolean
  onToggle: () => void
  onPost: () => void
}

function EntryRow({ entry, open, detail, loadingDetail, onToggle, onPost }: EntryRowProps) {
  const t = useT()
  return (
    <>
      <tr className="cursor-pointer hover:bg-alpine-50" onClick={onToggle}>
        <td className="text-alpine-400">
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </td>
        <td><span className="font-mono text-accent-700 text-xs">{entry.reference}</span></td>
        <td>{formatDate(entry.date)}</td>
        <td className="max-w-xs truncate text-alpine-700">{entry.description}</td>
        <td className="text-alpine-600 text-xs">{entry.author || '—'}</td>
        <td className="text-right font-mono tabular-nums">{formatCHF(entry.total)}</td>
        <td>
          <span className={`badge ${STATUS_CLASS[entry.status] ?? 'badge-draft'}`}>
            {t(STATUS_CLE[entry.status] ?? 'statut.brouillon')}
          </span>
        </td>
        <td className="text-right">
          {entry.status === 'draft' && (
            <button onClick={ev => { ev.stopPropagation(); onPost() }}
                    className="btn-ghost btn-sm text-success-700">
              {t('ach.comptabiliser')}
            </button>
          )}
        </td>
      </tr>

      {open && (
        <tr>
          <td colSpan={8} className="bg-alpine-50 px-5 py-3">
            {loadingDetail && <LoadingSpinner />}
            {detail && (
              <>
                <table className="w-full text-[13px]">
                  <thead>
                    <tr className="text-xs text-alpine-500">
                      <th className="text-left font-medium py-1">{t('jr.compte')}</th>
                      <th className="text-left font-medium py-1">{t('jr.libelle')}</th>
                      <th className="text-right font-medium py-1">{t('compta.debit')}</th>
                      <th className="text-right font-medium py-1">{t('compta.credit')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {detail.lines.map(l => (
                      <tr key={l.id} className="border-t border-alpine-200">
                        <td className="py-1.5">
                          <span className="font-mono text-accent-700">{l.account_code}</span>
                          <span className="text-alpine-600"> — {l.account_name}</span>
                        </td>
                        <td className="py-1.5 text-alpine-600">{l.description || '—'}</td>
                        <td className="py-1.5 text-right font-mono tabular-nums">
                          {l.debit_amount > 0 ? formatCHF(l.debit_amount) : '—'}
                        </td>
                        <td className="py-1.5 text-right font-mono tabular-nums">
                          {l.credit_amount > 0 ? formatCHF(l.credit_amount) : '—'}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>

                {/* L'empreinte est ce qui rend la traçabilité vérifiable plutôt
                    qu'affirmée : elle se cite, se compare, et son absence dit
                    qu'un brouillon n'est scellé par rien. */}
                <p className="text-xs text-alpine-500 mt-2 flex items-start gap-1.5">
                  <Shield size={12} className="mt-0.5 shrink-0" />
                  {detail.integrity_hash
                    ? <span>
                        Scellée — empreinte{' '}
                        <span className="font-mono break-all">{detail.integrity_hash}</span>
                      </span>
                    : <span>
                        {t('jn.brouillonSansEmpreinte')}
                      </span>}
                </p>
              </>
            )}
          </td>
        </tr>
      )}
    </>
  )
}
