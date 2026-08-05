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
import { formatCHF, formatDate, isOverdue } from '@/utils'
import type { DocumentStatus, Invoice, QuoteOutcome } from '@/types'

// ─── Transitions de statut autorisées ────────────────────────────────────────

const TRANSITIONS: Record<DocumentStatus, { status: DocumentStatus; label: string; icon: typeof Send; className: string }[]> = {
  draft:     [
    { status: 'sent',      label: 'Marquer envoyée',  icon: Send,        className: 'btn-primary' },
    { status: 'cancelled', label: 'Annuler',           icon: XCircle,     className: 'btn-ghost text-danger-700' },
  ],
  sent:      [
    { status: 'paid',      label: 'Marquer payée',    icon: CheckCircle, className: 'btn-primary' },
    { status: 'cancelled', label: 'Annuler',           icon: XCircle,     className: 'btn-ghost text-danger-700' },
  ],
  paid:      [
    { status: 'archived',  label: 'Archiver',          icon: Archive,     className: 'btn-ghost' },
  ],
  cancelled: [
    { status: 'draft',    label: 'Réactiver',           icon: RotateCcw,   className: 'btn-secondary' },
    { status: 'archived', label: 'Archiver',            icon: Archive,     className: 'btn-ghost' },
  ],
  archived:  [],
}

// Une offre de prix ne peut pas être « payée » : personne ne doit rien dessus.
// On l'accepte en produisant la facture (bouton Convertir), pas en changeant
// son statut — d'où une machine à états distincte.
const QUOTE_TRANSITIONS: Record<string, { status: DocumentStatus; label: string; icon: typeof Send; className: string }[]> = {
  draft:     [
    { status: 'sent',      label: 'Marquer envoyée', icon: Send,    className: 'btn-primary' },
    { status: 'cancelled', label: 'Annuler',          icon: XCircle, className: 'btn-ghost text-danger-700' },
  ],
  sent:      [
    { status: 'cancelled', label: 'Annuler',          icon: XCircle, className: 'btn-ghost text-danger-700' },
    { status: 'archived',  label: 'Archiver',         icon: Archive, className: 'btn-ghost' },
  ],
  cancelled: [
    { status: 'draft',     label: 'Réactiver',        icon: RotateCcw, className: 'btn-secondary' },
    { status: 'archived',  label: 'Archiver',         icon: Archive,   className: 'btn-ghost' },
  ],
  archived:  [],
}

const OUTCOME_LABEL: Record<QuoteOutcome, string> = {
  accepted: 'Offre acceptée — facture établie',
  refused:  'Offre refusée par le client',
  expired:  'Offre expirée',
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

function transitionCopy(status: DocumentStatus, isQuote: boolean, number: string): ActionCopy {
  const doc = isQuote ? "l'offre" : 'la facture'
  switch (status) {
    case 'sent':
      return {
        title: `Marquer ${doc} ${number} comme envoyée ?`,
        consequences: isQuote
          ? ["L'offre passe en « envoyée » et son issue — acceptée, refusée, expirée — devient à renseigner."]
          : [
              'La facture passe en « envoyée » : elle est réputée réclamée au client.',
              'Son échéance commence à courir, et elle apparaîtra en retard une fois cette date passée.',
            ],
        reassurance: "LedgerAlps n'envoie aucun courriel : c'est vous qui transmettez le document.",
        confirmLabel: 'Marquer envoyée',
      }
    case 'paid':
      return {
        title: `Marquer la facture ${number} comme payée ?`,
        consequences: [
          "Le paiement est enregistré à la date d'aujourd'hui et passé au journal (banque / débiteurs).",
          'La facture ne sera plus modifiable : un document réglé ne se réécrit pas.',
        ],
        reassurance: "Si le montant reçu diffère, enregistrez plutôt un paiement partiel depuis l'onglet Paiements.",
        confirmLabel: 'Marquer payée',
      }
    case 'cancelled':
      return {
        title: `Annuler ${doc} ${number} ?`,
        consequences: isQuote
          ? ["L'offre est classée sans suite et ne pourra plus être convertie en facture."]
          : [
              'La facture est classée sans effet et sort de vos créances.',
              "Elle reste consultable et conservée : le CO art. 958f impose dix ans, y compris pour un document annulé.",
            ],
        reassurance: isQuote
          ? "Une offre annulée peut être réactivée tant qu'elle n'est pas archivée."
          : "Pour corriger une facture déjà transmise au client, une note de crédit est le geste comptable attendu, pas une annulation.",
        confirmLabel: 'Annuler le document',
        tone: 'danger',
      }
    case 'archived':
      return {
        title: `Archiver ${doc} ${number} ?`,
        consequences: [
          'Le document quitte les listes courantes.',
          "C'est un état terminal : il ne pourra plus être réactivé depuis l'application.",
        ],
        reassurance: 'Il reste consultable, exporté dans les archives légales et compté dans vos rapports.',
        confirmLabel: 'Archiver',
      }
    case 'draft':
      return {
        title: `Réactiver ${doc} ${number} ?`,
        consequences: ['Le document repasse en brouillon et redevient modifiable.'],
        confirmLabel: 'Réactiver',
      }
    default:
      return {
        title: 'Confirmer cette action ?',
        consequences: ["Le statut du document va changer."],
        confirmLabel: 'Confirmer',
      }
  }
}

const CONVERT_COPY = (number: string): ActionCopy => ({
  title: `Convertir l'offre ${number} en facture ?`,
  consequences: [
    'Une facture est créée avec son propre numéro, et vous y êtes emmené.',
    "L'offre est marquée acceptée. Les deux documents restent liés et se citent.",
  ],
  reassurance: "L'offre n'est pas transformée : elle est conservée telle qu'elle a été envoyée (CO art. 957a al. 2 ch. 5).",
  confirmLabel: 'Créer la facture',
})

const CREDIT_NOTE_COPY = (number: string): ActionCopy => ({
  title: `Émettre une note de crédit pour la facture ${number} ?`,
  consequences: [
    'Une note de crédit est créée pour le montant restant, avec son propre numéro.',
    "Elle porte la mention « Annule la facture " + number + " » (LTVA art. 27 al. 4).",
  ],
  reassurance: 'La facture reste inchangée et conservée. Une note de crédit corrige, elle n\'efface pas.',
  confirmLabel: 'Émettre la note de crédit',
})

const OUTCOME_COPY: Record<'refused' | 'expired', (n: string) => ActionCopy> = {
  refused: n => ({
    title: `Marquer l'offre ${n} comme refusée ?`,
    consequences: [
      'Le client a décliné : aucune facture ne sera établie depuis cette offre.',
      "L'offre ne pourra plus être convertie.",
    ],
    confirmLabel: 'Marquer refusée',
    tone: 'danger',
  }),
  expired: n => ({
    title: `Marquer l'offre ${n} comme expirée ?`,
    consequences: [
      'La validité est dépassée sans réponse du client.',
      "L'offre ne pourra plus être convertie.",
    ],
    reassurance: 'Pour la remettre au client, dupliquez-la en une nouvelle offre.',
    confirmLabel: 'Marquer expirée',
  }),
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function InvoiceDetailPage() {
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
      transitionCopy(status, invoice?.document_type === 'quote', invoice?.invoice_number ?? ''),
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
    <ErrorBanner message="Impossible de charger la facture." />
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
        confirmLabel={pending?.copy.confirmLabel ?? 'Confirmer'}
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
          subtitle={`${invoice.document_type === 'quote' ? 'Offre de prix' : 'Facture'} · émise le ${formatDate(invoice.issue_date)}`}
          actions={
            <div className="flex items-center gap-2">
              {invoice.amount_paid === 0 && (
                <Link
                  to={`/invoices/${invoiceId}/edit`}
                  className="btn-secondary btn-sm flex items-center gap-1.5"
                >
                  <Pencil size={14} />
                  Modifier
                </Link>
              )}
              {quoteOpen && (
                <>
                  <button
                    onClick={() => ask(CONVERT_COPY(invoice.invoice_number), () => convert.mutate())}
                    disabled={convert.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5"
                    title="Crée une facture à partir de cette offre. L'offre est conservée."
                  >
                    <FileText size={14} />
                    {convert.isPending ? 'Conversion…' : 'Convertir en facture'}
                  </button>
                  <button
                    onClick={() => ask(OUTCOME_COPY.refused(invoice.invoice_number), () => setOutcome.mutate('refused'))}
                    disabled={setOutcome.isPending}
                    className="btn-ghost btn-sm flex items-center gap-1.5 text-danger-700"
                  >
                    <XCircle size={14} />
                    Refusée
                  </button>
                  <button
                    onClick={() => ask(OUTCOME_COPY.expired(invoice.invoice_number), () => setOutcome.mutate('expired'))}
                    disabled={setOutcome.isPending}
                    className="btn-ghost btn-sm flex items-center gap-1.5"
                  >
                    <Clock size={14} />
                    Expirée
                  </button>
                </>
              )}
              {creditable && (
                <button
                  onClick={() => ask(CREDIT_NOTE_COPY(invoice.invoice_number), () => creditNote.mutate())}
                  disabled={creditNote.isPending || fullyCredited}
                  className="btn-secondary btn-sm flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
                  title={fullyCredited
                    ? `Facture déjà créditée en totalité (${formatCHF(invoice.credited_amount)})`
                    : "Émet une note de crédit annulant cette facture. La facture est conservée."}
                >
                  <Undo2 size={14} />
                  {creditNote.isPending ? 'Création…' : 'Note de crédit'}
                </button>
              )}
              {actions.map(t => (
                <button
                  key={t.status}
                  onClick={() => handleTransition(t.status)}
                  disabled={transition.isPending}
                  className={`${t.className} btn-sm flex items-center gap-1.5`}
                >
                  <t.icon size={14} />
                  {t.label}
                </button>
              ))}
              <button
                onClick={() => setShowPDF(v => !v)}
                className="btn-ghost btn-sm flex items-center gap-1.5"
                title={showPDF ? 'Masquer l\'aperçu PDF' : 'Aperçu PDF'}
              >
                {showPDF ? <EyeOff size={14} /> : <Eye size={14} />}
                PDF
              </button>
              <button onClick={handleDownload} className="btn-ghost btn-sm" title="Télécharger PDF">
                <Download size={14} />
              </button>
              {invoice.document_type !== 'quote' && (
                <button
                  onClick={downloadSixDossier}
                  disabled={dossierBusy}
                  className="btn-ghost btn-sm"
                  title="Payload du QR et bulletin, à déposer sur le portail de validation SIX"
                >
                  {dossierBusy ? 'Préparation…' : 'Validation SIX'}
                </button>
              )}
            </div>
          }
        />
      </div>

      {transition.isError && (
        <ErrorBanner message="Erreur lors du changement de statut." />
      )}
      {convert.isError && (
        <ErrorBanner message="La conversion a échoué. Cette offre a peut-être déjà donné lieu à une facture." />
      )}
      {creditNote.isError && (
        <ErrorBanner message="La note de crédit a été refusée : le total crédité dépasserait le montant de la facture." />
      )}

      {/* Lien entre l'offre et la facture qui en découle. Les deux documents
          existent séparément ; ce bandeau est ce qui les relie à l'écran. */}
      {invoice.quote_outcome && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          {OUTCOME_LABEL[invoice.quote_outcome]}
        </div>
      )}
      {fullyCredited && invoice.document_type === 'invoice' && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          Facture créditée en totalité ({formatCHF(invoice.credited_amount)}). La facture
          reste telle qu'elle a été émise ; la correction vit dans la note de crédit.
        </div>
      )}
      {invoice.corrects_invoice_id && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          Note de crédit annulant la{' '}
          <Link to={`/invoices/${invoice.corrects_invoice_id}`} className="underline font-medium">
            facture d'origine
          </Link>
          , conservée telle qu'elle a été émise.
        </div>
      )}
      {invoice.converted_from_id && (
        <div className="mb-6 rounded-md border border-neutral-200 bg-neutral-50 px-4 py-3 text-sm">
          Facture établie à partir d'une{' '}
          <Link to={`/invoices/${invoice.converted_from_id}`} className="underline font-medium">
            offre de prix
          </Link>
          , conservée telle qu'elle a été envoyée.
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-5 mb-6">
        {/* Résumé financier */}
        <div className="lg:col-span-2 card">
          <SectionTitle>Montants</SectionTitle>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between text-alpine-600">
              <span>Sous-total HT</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.subtotal_amount)}</span>
            </div>
            <div className="flex justify-between text-alpine-600">
              <span>TVA</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.vat_amount)}</span>
            </div>
            <div className="flex justify-between text-alpine-900 font-semibold border-t border-alpine-100 pt-2">
              <span>Total TTC</span>
              <span className="font-mono tabular-nums">{formatCHF(invoice.total_amount)}</span>
            </div>
            {invoice.amount_paid > 0 && (
              <>
                <div className="flex justify-between text-success-700">
                  <span>Déjà payé</span>
                  <span className="font-mono tabular-nums">−{formatCHF(invoice.amount_paid)}</span>
                </div>
                <div className="flex justify-between text-alpine-900 font-semibold border-t border-alpine-100 pt-2">
                  <span>Solde restant</span>
                  <span className="font-mono tabular-nums">{formatCHF(totalRemaining)}</span>
                </div>
              </>
            )}
          </div>
        </div>

        {/* Infos facture */}
        <div className="card space-y-3">
          <SectionTitle>Informations</SectionTitle>
          <dl className="text-sm space-y-2">
            <div className="flex justify-between">
              <dt className="text-alpine-500">Statut</dt>
              <dd><StatusBadge status={invoice.status} overdue={isOverdue(invoice)} /></dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-alpine-500">Date d'émission</dt>
              <dd className="text-alpine-800">{formatDate(invoice.issue_date)}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-alpine-500">Échéance</dt>
              <dd className={`${isOverdue(invoice) ? 'text-danger-700 font-medium' : 'text-alpine-800'}`}>
                {formatDate(invoice.due_date)}
              </dd>
            </div>
            {invoice.qr_reference && (
              <div className="flex justify-between">
                <dt className="text-alpine-500">QR-Ref</dt>
                <dd className="text-alpine-700 font-mono text-xs truncate max-w-[120px]">{invoice.qr_reference}</dd>
              </div>
            )}
          </dl>
        </div>
      </div>

      {/* Lignes de facture */}
      <div className="card mb-6">
        <SectionTitle>Lignes</SectionTitle>
        <div className="table-wrapper">
          <table className="table">
            <thead>
              <tr>
                <th>#</th>
                <th>Description</th>
                <th className="text-right">Qté</th>
                <th className="text-right">P.U. CHF</th>
                <th className="text-right">Rabais</th>
                <th className="text-right">TVA</th>
                <th className="text-right">Total CHF</th>
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
            <div className="card">
              <SectionTitle>Remarques</SectionTitle>
              <p className="text-sm text-alpine-600 whitespace-pre-line">{invoice.notes}</p>
            </div>
          )}
          {invoice.terms && (
            <div className="card">
              <SectionTitle>Conditions</SectionTitle>
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
