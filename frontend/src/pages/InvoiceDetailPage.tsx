// LedgerAlps — Détail d'une facture

import { useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft, Download, Eye, EyeOff, Send, CheckCircle,
  XCircle, Archive, Pencil, RotateCcw, FileText, Clock, Undo2,
} from 'lucide-react'
import { invoicesApi, downloadBlob } from '@/api/client'
import {
  PageHeader, StatusBadge, LoadingSpinner, ErrorBanner,
  SectionTitle, PDFPreview, ConfirmDialog,
} from '@/components/ui'
import { useCanWrite } from '@/hooks/usePermissions'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'
import { formatCHF, formatDate, isOverdue } from '@/utils'
import type { DocumentStatus, Invoice, QuoteOutcome } from '@/types'

// La fonction de traduction telle que `useT()` la rend.
//
// Les tables et les textes de confirmation ci-dessous sont construits au
// chargement du module, avant qu'aucun composant n'existe : `useT()` n'y est
// pas appelable. Ils portent donc des CLÉS, et les fonctions reçoivent `t` en
// premier argument — la traduction se fait au moment de l'affichage, dans la
// langue du moment.
type T = ReturnType<typeof useT>

// ─── Transitions de statut autorisées ────────────────────────────────────────

type Transition = { status: DocumentStatus; cle: Cle; icon: typeof Send; className: string }

const TRANSITIONS: Record<DocumentStatus, Transition[]> = {
  draft:     [
    { status: 'sent',      cle: 'fd.marquerEnvoyee', icon: Send,        className: 'btn-primary' },
    { status: 'cancelled', cle: 'action.annuler',    icon: XCircle,     className: 'btn-ghost text-danger-700' },
  ],
  sent:      [
    { status: 'paid',      cle: 'fd.marquerPayee',   icon: CheckCircle, className: 'btn-primary' },
    { status: 'cancelled', cle: 'action.annuler',    icon: XCircle,     className: 'btn-ghost text-danger-700' },
  ],
  paid:      [
    { status: 'archived',  cle: 'fd.archiver',       icon: Archive,     className: 'btn-ghost' },
  ],
  cancelled: [
    { status: 'draft',    cle: 'fd.reactiver',       icon: RotateCcw,   className: 'btn-secondary' },
    { status: 'archived', cle: 'fd.archiver',        icon: Archive,     className: 'btn-ghost' },
  ],
  archived:  [],
}

// Une offre de prix ne peut pas être « payée » : personne ne doit rien dessus.
// On l'accepte en produisant la facture (bouton Convertir), pas en changeant
// son statut — d'où une machine à états distincte.
const QUOTE_TRANSITIONS: Record<string, Transition[]> = {
  draft:     [
    { status: 'sent',      cle: 'fd.marquerEnvoyee', icon: Send,    className: 'btn-primary' },
    { status: 'cancelled', cle: 'action.annuler',    icon: XCircle, className: 'btn-ghost text-danger-700' },
  ],
  sent:      [
    { status: 'cancelled', cle: 'action.annuler',    icon: XCircle, className: 'btn-ghost text-danger-700' },
    { status: 'archived',  cle: 'fd.archiver',       icon: Archive, className: 'btn-ghost' },
  ],
  cancelled: [
    { status: 'draft',     cle: 'fd.reactiver',      icon: RotateCcw, className: 'btn-secondary' },
    { status: 'archived',  cle: 'fd.archiver',       icon: Archive,   className: 'btn-ghost' },
  ],
  archived:  [],
}

const OUTCOME_LABEL: Record<QuoteOutcome, Cle> = {
  accepted: 'fd.issueAcceptee',
  refused:  'fd.issueRefusee',
  expired:  'fd.issueExpiree',
}


// ─── Ce que chaque action fait vraiment ──────────────────────────────────────
//
// Une confirmation qui demande « êtes-vous sûr ? » ne protège personne : elle
// ne dit ni ce qui va se passer, ni ce qui reste récupérable, et répétée elle
// devient un réflexe. Chaque action décrit donc ses conséquences.
//
// Le vocabulaire suit le document : une offre de prix n'est pas une facture, et
// « annuler » ne veut pas dire la même chose sur l'une et sur l'autre.

type ActionCopy = {
  title: string
  consequences: string[]
  reassurance?: string
  confirmLabel: string
  tone?: 'danger' | 'normal'
}

function transitionCopy(t: T, status: DocumentStatus, isQuote: boolean, number: string): ActionCopy {
  const n = { numero: number }
  switch (status) {
    case 'sent':
      return {
        title: t(isQuote ? 'fd.titreEnvoyeeOffre' : 'fd.titreEnvoyeeFacture', n),
        consequences: isQuote
          ? [t('fd.consEnvoyeeOffre')]
          : [t('fd.consEnvoyeeFacture1'), t('fd.consEnvoyeeFacture2')],
        reassurance: t('fd.rassurEnvoyee'),
        confirmLabel: t('fd.marquerEnvoyee'),
      }
    case 'paid':
      return {
        title: t('fd.titrePayee', n),
        consequences: [t('fd.consPayee1'), t('fd.consPayee2')],
        reassurance: t('fd.rassurPayee'),
        confirmLabel: t('fd.marquerPayee'),
      }
    case 'cancelled':
      return {
        title: t(isQuote ? 'fd.titreAnnulerOffre' : 'fd.titreAnnulerFacture', n),
        consequences: isQuote
          ? [t('fd.consAnnulerOffre')]
          : [t('fd.consAnnulerFacture1'), t('fd.consAnnulerFacture2')],
        reassurance: t(isQuote ? 'fd.rassurAnnulerOffre' : 'fd.rassurAnnulerFacture'),
        confirmLabel: t('fd.confirmerAnnuler'),
        tone: 'danger',
      }
    case 'archived':
      return {
        title: t(isQuote ? 'fd.titreArchiverOffre' : 'fd.titreArchiverFacture', n),
        consequences: [t('fd.consArchiver1'), t('fd.consArchiver2')],
        reassurance: t('fd.rassurArchiver'),
        confirmLabel: t('fd.archiver'),
      }
    case 'draft':
      return {
        title: t(isQuote ? 'fd.titreReactiverOffre' : 'fd.titreReactiverFacture', n),
        consequences: [t('fd.consReactiver')],
        confirmLabel: t('fd.reactiver'),
      }
    default:
      return {
        title: t('fd.titreDefaut'),
        consequences: [t('fd.consDefaut')],
        confirmLabel: t('action.confirmer'),
      }
  }
}

const CONVERT_COPY = (t: T, number: string): ActionCopy => ({
  title: t('fd.titreConvertir', { numero: number }),
  consequences: [t('fd.consConvertir1'), t('fd.consConvertir2')],
  reassurance: t('fd.rassurConvertir'),
  confirmLabel: t('fd.confirmerConvertir'),
})

const CREDIT_NOTE_COPY = (t: T, number: string): ActionCopy => ({
  title: t('fd.titreNoteCredit', { numero: number }),
  consequences: [
    t('fd.consNoteCredit1'),
    t('fd.consNoteCredit2', { numero: number }),
  ],
  reassurance: t('fd.rassurNoteCredit'),
  confirmLabel: t('fd.confirmerNoteCredit'),
})

const OUTCOME_COPY: Record<'refused' | 'expired', (t: T, n: string) => ActionCopy> = {
  refused: (t, n) => ({
    title: t('fd.titreRefusee', { numero: n }),
    consequences: [t('fd.consRefusee1'), t('fd.consPlusConvertible')],
    confirmLabel: t('fd.confirmerRefusee'),
    tone: 'danger',
  }),
  expired: (t, n) => ({
    title: t('fd.titreExpiree', { numero: n }),
    consequences: [t('fd.consExpiree1'), t('fd.consPlusConvertible')],
    reassurance: t('fd.rassurExpiree'),
    confirmLabel: t('fd.confirmerExpiree'),
  }),
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function InvoiceDetailPage() {
  const t = useT()
  const peutEcrire = useCanWrite()
  const { invoiceId } = useParams<{ invoiceId: string }>()
  const navigate      = useNavigate()
  const qc            = useQueryClient()
  const [showPDF, setShowPDF] = useState(false)

  // L'action en attente de confirmation. Une seule à la fois : le dialogue est
  // modal, et deux confirmations superposées seraient illisibles.
  const [pending, setPending] = useState<
    { copy: ActionCopy; run: () => void } | null
  >(null)
  const ask = (copy: ActionCopy, run: () => void) => setPending({ copy, run })

  const { data: invoice, isLoading, error } = useQuery<Invoice>({
    queryKey: ['invoice', invoiceId],
    queryFn:  () => invoicesApi.get(invoiceId!).then(r => r.data),
    enabled:  !!invoiceId,
  })

  const transition = useMutation({
    mutationFn: ({ status }: { status: DocumentStatus; paymentDate?: string }) =>
      invoicesApi.updateStatus(invoiceId!, status),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['invoice', invoiceId] })
      qc.invalidateQueries({ queryKey: ['invoices'] })
    },
  })

  const convert = useMutation({
    mutationFn: () => invoicesApi.convertQuote(invoiceId!),
    onSuccess: (resp) => {
      qc.invalidateQueries({ queryKey: ['invoices'] })
      qc.invalidateQueries({ queryKey: ['invoice', invoiceId] })
      // On ouvre la facture créée ; l'offre reste consultable à son adresse.
      navigate(`/invoices/${resp.data.id}`)
    },
  })

  const setOutcome = useMutation({
    mutationFn: (outcome: 'refused' | 'expired') => invoicesApi.setQuoteOutcome(invoiceId!, outcome),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['invoice', invoiceId] })
      qc.invalidateQueries({ queryKey: ['invoices'] })
    },
  })

  const creditNote = useMutation({
    mutationFn: () => invoicesApi.createCreditNote(invoiceId!),
    onSuccess: (resp) => {
      qc.invalidateQueries({ queryKey: ['invoices'] })
      qc.invalidateQueries({ queryKey: ['invoice', invoiceId] })
      navigate(`/invoices/${resp.data.id}`)
    },
  })

  const handleTransition = (status: DocumentStatus) => {
    const extra = status === 'paid'
      ? { paymentDate: new Date().toISOString().slice(0, 10) }
      : {}
    ask(
      transitionCopy(t, status, invoice?.document_type === 'quote', invoice?.invoice_number ?? ''),
      () => transition.mutate({ status, ...extra }),
    )
  }

  const handleDownload = async () => {
    if (!invoice) return
    const resp = await invoicesApi.downloadPDF(invoiceId!)
    const prefix = invoice.document_type === 'quote' ? 'offre'
      : invoice.document_type === 'credit_note' ? 'note_credit' : 'facture'
    downloadBlob(resp.data, `${prefix}_${invoice.invoice_number}.pdf`)
  }

  // Dossier de validation SIX : le payload exact du QR et le bulletin, à
  // déposer sur le portail des Swiss Payment Standards. C'est la seule
  // vérification qui fasse autorité — nos tests vérifient notre lecture de la
  // spécification, pas la conformité du bulletin produit.
  const [dossierBusy, setDossierBusy] = useState(false)
  const downloadSixDossier = async () => {
    if (!invoice) return
    setDossierBusy(true)
    try {
      const resp = await invoicesApi.sixValidation(invoice.id)
      downloadBlob(resp.data, `validation-six_${invoice.invoice_number}.zip`)
    } finally {
      setDossierBusy(false)
    }
  }

  if (isLoading) return <LoadingSpinner />
  if (error || !invoice) return (
    <ErrorBanner message={t('fd.erreurChargement')} />
  )

  const isQuote = invoice.document_type === 'quote'
  const actions = (isQuote ? QUOTE_TRANSITIONS[invoice.status] : TRANSITIONS[invoice.status]) ?? []
  // Une offre envoyée dont l'issue n'est pas encore connue reste actionnable.
  const quoteOpen = isQuote && invoice.status === 'sent' && !invoice.quote_outcome
  // Une facture émise — envoyée ou payée — peut être corrigée. Un brouillon n'a
  // jamais été réclamé, une facture annulée est déjà sans effet.
  // Une facture entièrement créditée ne doit plus proposer le bouton : le
  // serveur refuserait (409), et offrir une action vouée à l'échec est pire
  // que ne pas l'offrir. Un centime de tolérance absorbe l'arrondi à 5 ct.
  const fullyCredited = invoice.credited_amount >= invoice.total_amount - 0.01
  const creditable = invoice.document_type === 'invoice'
    && (invoice.status === 'sent' || invoice.status === 'paid')
  const totalRemaining = invoice.total_amount - invoice.amount_paid

  const busy = transition.isPending || convert.isPending
    || setOutcome.isPending || creditNote.isPending

  return (
    <div className="max-w-4xl mx-auto">
      <ConfirmDialog
        open={pending !== null}
        title={pending?.copy.title ?? ''}
        consequences={pending?.copy.consequences ?? []}
        reassurance={pending?.copy.reassurance}
        confirmLabel={pending?.copy.confirmLabel ?? t('action.confirmer')}
        tone={pending?.copy.tone}
        busy={busy}
        onConfirm={() => { pending?.run(); setPending(null) }}
        onCancel={() => setPending(null)}
      />

      {/* En-tête */}
      <div className="flex items-center gap-3 mb-6">
        <button onClick={() => navigate(-1)} className="btn-ghost btn-sm">
          <ArrowLeft size={15} />
        </button>
        <PageHeader
          title={invoice.invoice_number}
          subtitle={t(invoice.document_type === 'quote' ? 'fd.sousTitreOffre' : 'fd.sousTitreFacture',
            { date: formatDate(invoice.issue_date) })}
          actions={
            // Modifier, convertir, envoyer, annuler, créer une note de crédit :
            // toutes ces commandes changent une pièce comptable. L'aperçu et le
            // téléchargement du PDF, plus bas, restent ouverts à tous — c'est
            // exactement ce qu'une fiduciaire vient chercher.
            <div className="flex items-center gap-2">
              {peutEcrire && invoice.amount_paid === 0 && (
                <Link
                  to={`/invoices/${invoiceId}/edit`}
                  className="btn-secondary btn-sm flex items-center gap-1.5"
                >
                  <Pencil size={14} />
                  {t('action.modifier')}
                </Link>
              )}
              {peutEcrire && quoteOpen && (
                <>
                  <button
                    onClick={() => ask(CONVERT_COPY(t, invoice.invoice_number), () => convert.mutate())}
                    disabled={convert.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5"
                    title={t('fd.infoBulleConvertir')}
                  >
                    <FileText size={14} />
                    {convert.isPending ? t('fd.conversionEnCours') : t('fd.convertirEnFacture')}
                  </button>
                  <button
                    onClick={() => ask(OUTCOME_COPY.refused(t, invoice.invoice_number), () => setOutcome.mutate('refused'))}
                    disabled={setOutcome.isPending}
                    className="btn-ghost btn-sm flex items-center gap-1.5 text-danger-700"
                  >
                    <XCircle size={14} />
                    {t('fd.refusee')}
                  </button>
                  <button
                    onClick={() => ask(OUTCOME_COPY.expired(t, invoice.invoice_number), () => setOutcome.mutate('expired'))}
                    disabled={setOutcome.isPending}
                    className="btn-ghost btn-sm flex items-center gap-1.5"
                  >
                    <Clock size={14} />
                    {t('fd.expiree')}
                  </button>
                </>
              )}
              {peutEcrire && creditable && (
                <button
                  onClick={() => ask(CREDIT_NOTE_COPY(t, invoice.invoice_number), () => creditNote.mutate())}
                  disabled={creditNote.isPending || fullyCredited}
                  className="btn-secondary btn-sm flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
                  title={fullyCredited
                    ? t('fd.dejaCreditee', { montant: formatCHF(invoice.credited_amount) })
                    : t('fd.infoBulleNoteCredit')}
                >
                  <Undo2 size={14} />
                  {creditNote.isPending ? t('fd.creationEnCours') : t('fd.noteDeCredit')}
                </button>
              )}
              {peutEcrire && actions.map(action => (
                <button
                  key={action.status}
                  onClick={() => handleTransition(action.status)}
                  disabled={transition.isPending}
                  className={`${action.className} btn-sm flex items-center gap-1.5`}
                >
                  <action.icon size={14} />
                  {t(action.cle)}
                </button>
              ))}
              <button
                onClick={() => setShowPDF(v => !v)}
                className="btn-ghost btn-sm flex items-center gap-1.5"
                title={showPDF ? t('fd.masquerApercu') : t('fd.apercuPDF')}
              >
                {showPDF ? <EyeOff size={14} /> : <Eye size={14} />}
                PDF
              </button>
              <button onClick={handleDownload} className="btn-ghost btn-sm" title={t('fact.telechargerPDF')}>
                <Download size={14} />
              </button>
              {invoice.document_type !== 'quote' && (
                <button
                  onClick={downloadSixDossier}
                  disabled={dossierBusy}
                  className="btn-ghost btn-sm"
                  title={t('fd.infoBulleSix')}
                >
                  {dossierBusy ? t('sv.preparationEnCours') : t('fd.validationSix')}
                </button>
              )}
            </div>
          }
        />
      </div>

      {transition.isError && (
        <ErrorBanner message={t('fd.erreurStatut')} />
      )}
      {convert.isError && (
        <ErrorBanner message={t('fd.erreurConversion')} />
      )}
      {creditNote.isError && (
        <ErrorBanner message={t('fd.erreurNoteCredit')} />
      )}

      {/* Lien entre l'offre et la facture qui en découle. Les deux documents
          existent séparément ; ce bandeau est ce qui les relie à l'écran. */}
      {invoice.quote_outcome && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          {t(OUTCOME_LABEL[invoice.quote_outcome])}
        </div>
      )}
      {fullyCredited && invoice.document_type === 'invoice' && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          {t('fd.crediteeEnTotalite', { montant: formatCHF(invoice.credited_amount) })}
        </div>
      )}
      {invoice.corrects_invoice_id && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          {t('fd.noteCreditAnnulantAvant')}{' '}
          <Link to={`/invoices/${invoice.corrects_invoice_id}`} className="underline font-medium">
            {t('fd.factureOrigine')}
          </Link>
          {t('fd.noteCreditAnnulantApres')}
        </div>
      )}
      {invoice.converted_from_id && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          {t('fd.etablieDepuisAvant')}{' '}
          <Link to={`/invoices/${invoice.converted_from_id}`} className="underline font-medium">
            {t('fd.offreDePrix')}
          </Link>
          {t('fd.etablieDepuisApres')}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-5 mb-6">
        {/* Résumé financier */}
        <div className="lg:col-span-2 card card-pad">
          <SectionTitle>{t('fd.montants')}</SectionTitle>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between text-alpine-600">
              <span>{t('fd.sousTotalHT')}</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.subtotal_amount)}</span>
            </div>
            <div className="flex justify-between text-alpine-600">
              <span>{t('tva.tva')}</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.vat_amount)}</span>
            </div>
            <div className="flex justify-between text-alpine-900 font-semibold border-t border-alpine-100 pt-2">
              <span>{t('fd.totalTTC')}</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.total_amount)}</span>
            </div>
            {invoice.amount_paid > 0 && (
              <>
                <div className="flex justify-between text-success-700">
                  <span>{t('fd.dejaPaye')}</span>
                  <span className="font-mono tabular-nums">−{formatCHF(invoice.amount_paid)}</span>
                </div>
                <div className="flex justify-between text-alpine-900 font-semibold border-t border-alpine-100 pt-2">
                  <span>{t('fd.soldeRestant')}</span>
                  <span className="font-mono tabular-nums">{formatCHF(totalRemaining)}</span>
                </div>
              </>
            )}
          </div>
        </div>

        {/* Infos facture */}
        <div className="card card-pad space-y-3">
          <SectionTitle>{t('fd.informations')}</SectionTitle>
          <dl className="text-sm space-y-2">
            <div className="flex justify-between">
              <dt className="text-alpine-500">{t('fact.colStatut')}</dt>
              <dd><StatusBadge status={invoice.status} overdue={isOverdue(invoice)} /></dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-alpine-500">{t('fd.dateEmission')}</dt>
              <dd className="text-alpine-800">{formatDate(invoice.issue_date)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-alpine-500">{t('doc.echeance')}</dt>
              <dd className={`${isOverdue(invoice) ? 'text-danger-700 font-medium' : 'text-alpine-800'}`}>
                {formatDate(invoice.due_date)}
              </dd>
            </div>
            {invoice.qr_reference && (
              <div className="flex justify-between">
                <dt className="text-alpine-500">{t('fd.qrRef')}</dt>
                <dd className="text-alpine-700 font-mono text-xs truncate max-w-[120px]">{invoice.qr_reference}</dd>
              </div>
            )}
          </dl>
        </div>
      </div>

      {/* Lignes de facture */}
      <div className="card card-pad mb-6">
        <SectionTitle>{t('fd.lignes')}</SectionTitle>
        <div className="table-wrapper">
          <table className="table">
            <thead>
              <tr>
                <th>#</th>
                <th>{t('jr.colDescription')}</th>
                <th className="text-right">{t('fd.colQte')}</th>
                <th className="text-right">{t('fd.colPU')}</th>
                <th className="text-right">{t('fd.colRabais')}</th>
                <th className="text-right">{t('tva.tva')}</th>
                <th className="text-right">{t('fact.colTotal')}</th>
              </tr>
            </thead>
            <tbody>
              {invoice.lines.map(line => (
                <tr key={line.id}>
                  <td className="text-alpine-400 w-8">{line.sequence}</td>
                  <td className="text-alpine-800">{line.description}</td>
                  <td className="text-right tabular-nums">
                    {line.quantity}{line.unit ? ` ${line.unit}` : ''}
                  </td>
                  <td className="text-right tabular-nums font-mono">{formatCHF(line.unit_price)}</td>
                  <td className="text-right tabular-nums text-alpine-500">
                    {line.discount_pct > 0 ? `${line.discount_pct}%` : '—'}
                  </td>
                  <td className="text-right tabular-nums text-alpine-500">{line.vat_rate}%</td>
                  <td className="text-right tabular-nums font-mono font-medium">{formatCHF(line.line_total)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Notes / conditions */}
      {(invoice.notes || invoice.terms) && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
          {invoice.notes && (
            <div className="card card-pad">
              <SectionTitle>{t('fd.remarques')}</SectionTitle>
              <p className="text-sm text-alpine-600 whitespace-pre-line">{invoice.notes}</p>
            </div>
          )}
          {invoice.terms && (
            <div className="card card-pad">
              <SectionTitle>{t('fd.conditions')}</SectionTitle>
              <p className="text-sm text-alpine-600 whitespace-pre-line">{invoice.terms}</p>
            </div>
          )}
        </div>
      )}

      {/* Aperçu PDF inline */}
      {showPDF && (
        <PDFPreview
          fetchPDF={() => invoicesApi.downloadPDF(invoiceId!).then(r => r.data as Blob)}
          filename={`facture_${invoice.invoice_number}.pdf`}
          className="mb-6"
          onClose={() => setShowPDF(false)}
        />
      )}
    </div>
  )
}
