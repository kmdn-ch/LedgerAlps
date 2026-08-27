// LedgerAlps — Modification d'une facture / offre de prix

import { useEffect, useState } from 'react'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { Plus, Trash2, ArrowLeft, Save, UserPlus, X, AlertTriangle } from 'lucide-react'
import { invoicesApi, contactsApi, settingsApi } from '@/api/client'
import { PageHeader, ErrorBanner, LoadingSpinner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { formatCHF } from '@/utils'
import type { Contact, CompanySettings, Invoice } from '@/types'
import { useT, useTv } from '@/i18n/useT'

// ── Schéma identique à NewInvoicePage ─────────────────────────────────────────

const lineSchema = z.object({
  description: z.string().min(1, 'val.requis'),
  quantity:    z.coerce.number().positive(),
  unit:        z.string().optional(),
  unit_price:  z.coerce.number().positive('val.prixRequis'),
  discount_pct: z.coerce.number().min(0).max(100).default(0),
  vat_rate:    z.coerce.number().min(0).default(8.1),
})

const schema = z.object({
  document_type: z.enum(['invoice', 'quote', 'credit_note']).default('invoice'),
  contact_id:    z.string().min(1, 'val.contactRequis'),
  issue_date:    z.string().min(1, 'val.dateRequise'),
  due_date:      z.string().min(1, 'val.echeanceRequise'),
  notes:         z.string().optional(),
  terms:         z.string().optional(),
  lines:         z.array(lineSchema).min(1, 'val.auMoinsUneLigne'),
})

type FormData = z.infer<typeof schema>

function computeLineTotals(line: Partial<FormData['lines'][0]>) {
  const qty      = Number(line.quantity    ?? 1)
  const price    = Number(line.unit_price  ?? 0)
  const discount = Number(line.discount_pct ?? 0) / 100
  const vatRate  = Number(line.vat_rate    ?? 8.1) / 100
  const base     = qty * price * (1 - discount)
  const vat      = Math.round(base * vatRate * 20) / 20
  return {
    base:  Math.round(base * 100) / 100,
    vat:   Math.round(vat  * 100) / 100,
    total: Math.round((base + vat) * 100) / 100,
  }
}

// ── Mini-modal création rapide de contact ─────────────────────────────────────
//
// Ce contact devient toujours le débiteur d'une facture QR (contact_type
// fixé à 'customer') : l'adresse complète est donc obligatoire ici, pas
// seulement conseillée — sans elle le bulletin de versement ne s'imprime pas
// (SPC 0200 §4.2.2).

const EMPTY_CONTACT = {
  name: '', is_company: false, email: '', phone: '',
  address: '', postal_code: '', city: '', country: 'CH',
}

function NewContactModal({
  onClose,
  onCreated,
}: { onClose: () => void; onCreated: (c: Contact) => void }) {
  const t = useT()
  const qc = useQueryClient()
  const [fields, setFields] = useState(EMPTY_CONTACT)
  const [err, setErr] = useState<string | null>(null)
  const [tried, setTried] = useState(false)

  const create = useMutation({
    mutationFn: () => contactsApi.create({
      contact_type: 'customer', is_company: fields.is_company,
      name: fields.name.trim(), email: fields.email || undefined,
      phone: fields.phone || undefined,
      address: fields.address.trim(), postal_code: fields.postal_code.trim(),
      city: fields.city.trim(), country: fields.country || 'CH', payment_term_days: 30,
    }),
    onSuccess: (res) => { qc.invalidateQueries({ queryKey: ['contacts'] }); onCreated(res.data as Contact) },
    onError:   (e) => setErr(refusalMessage(e, t('nf.erreurContact'))),
  })

  const set = (k: keyof typeof EMPTY_CONTACT, v: string | boolean) =>
    setFields(f => ({ ...f, [k]: v }))

  const champsQrManquants =
    !fields.name.trim() || !fields.address.trim() || !fields.postal_code.trim() || !fields.city.trim()

  const submit = () => {
    setTried(true)
    if (champsQrManquants) return
    create.mutate()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div className="bg-white rounded-xl shadow-2xl w-full max-w-md mx-4">
        <div className="flex items-center justify-between px-5 py-4 border-b border-alpine-200">
          <h3 className="text-sm font-semibold text-alpine-900">{t('ct.nouveau')}</h3>
          <button type="button" onClick={onClose} className="btn-ghost btn-sm p-1 text-alpine-400">
            <X size={16} />
          </button>
        </div>
        <div className="px-5 py-4 space-y-3">
          {err && <p className="text-xs text-danger-700 bg-danger-100 rounded px-3 py-2">{err}</p>}
          <div className="flex items-center gap-2">
            <input id="nc_co" type="checkbox" checked={fields.is_company}
              onChange={e => set('is_company', e.target.checked)}
              className="rounded border-alpine-300 text-alpine-700" />
            <label htmlFor="nc_co" className="text-sm text-alpine-700">{t('nf.entreprise')}</label>
          </div>
          <div>
            <label className="label">{t('nf.nom')}</label>
            <input className={`input ${tried && !fields.name.trim() ? 'input-error' : ''}`}
              placeholder={fields.is_company ? t('nf.placeholderRaisonSociale') : t('nf.placeholderPrenomNom')}
              value={fields.name} onChange={e => set('name', e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div><label className="label">{t('nf.email')}</label>
              <input type="email" className="input" value={fields.email}
                onChange={e => set('email', e.target.value)} /></div>
            <div><label className="label">{t('nf.telephone')}</label>
              <input type="tel" className="input" value={fields.phone}
                onChange={e => set('phone', e.target.value)} /></div>
          </div>
          <div>
            <label className="label">{t('co.adresse')} *</label>
            <input className={`input ${tried && !fields.address.trim() ? 'input-error' : ''}`}
              placeholder={t('co.placeholderRue')}
              value={fields.address} onChange={e => set('address', e.target.value)} />
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div><label className="label">{t('pr.npa')} *</label>
              <input className={`input ${tried && !fields.postal_code.trim() ? 'input-error' : ''}`}
                value={fields.postal_code}
                onChange={e => set('postal_code', e.target.value)} /></div>
            <div className="col-span-2"><label className="label">{t('nf.ville')} *</label>
              <input className={`input ${tried && !fields.city.trim() ? 'input-error' : ''}`}
                value={fields.city}
                onChange={e => set('city', e.target.value)} /></div>
          </div>
          <div>
            <label className="label">{t('nf.pays')}</label>
            <input className="input w-24" maxLength={2} value={fields.country}
              onChange={e => set('country', e.target.value.toUpperCase())} />
          </div>
          {tried && champsQrManquants && (
            <p className="text-xs text-danger-700">{t('nf.qrIncomplet')}</p>
          )}
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-alpine-200">
          <button type="button" onClick={onClose} className="btn-secondary btn-sm">{t('action.annuler')}</button>
          <button type="button" disabled={create.isPending}
            onClick={submit} className="btn-primary btn-sm">
            {create.isPending ? t('fd.creationEnCours') : t('nf.creerContact')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Page principale ────────────────────────────────────────────────────────────

export function EditInvoicePage() {
  const t = useT()
  const tv = useTv()
  const { invoiceId }           = useParams<{ invoiceId: string }>()
  const navigate                = useNavigate()
  const qc                      = useQueryClient()
  const [showContactModal, setShowContactModal] = useState(false)

  // Load existing invoice
  const { data: invoice, isLoading, error: loadError } = useQuery<Invoice>({
    queryKey: ['invoice', invoiceId],
    queryFn:  () => invoicesApi.get(invoiceId!).then(r => r.data),
    enabled:  !!invoiceId,
  })

  // Load contacts
  const { data: contacts = [] } = useQuery<Contact[]>({
    queryKey: ['contacts'],
    queryFn:  () => contactsApi.list().then(r => r.data),
  })

  const { data: company } = useQuery<CompanySettings>({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
  })

  const {
    register, control, handleSubmit, setValue, reset,
    formState: { errors },
  } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      document_type: 'invoice',
      lines: [{ description: '', quantity: 1, unit_price: 0, discount_pct: 0, vat_rate: 8.1 }],
    },
  })

  // Pre-fill form once invoice loads
  useEffect(() => {
    if (!invoice) return
    reset({
      document_type: invoice.document_type as FormData['document_type'],
      contact_id:    invoice.contact_id,
      issue_date:    invoice.issue_date.slice(0, 10),
      due_date:      invoice.due_date ? invoice.due_date.slice(0, 10) : '',
      notes:         invoice.notes  ?? '',
      terms:         invoice.terms  ?? '',
      lines: invoice.lines.map(l => ({
        description:  l.description,
        quantity:     l.quantity,
        unit:         l.unit ?? '',
        unit_price:   l.unit_price,
        discount_pct: l.discount_pct,
        vat_rate:     l.vat_rate,
      })),
    })
  }, [invoice, reset])

  const { fields, append, remove } = useFieldArray({ control, name: 'lines' })
  const watchedLines     = useWatch({ control, name: 'lines' })
  const watchedContactId = useWatch({ control, name: 'contact_id' })
  const watchedDocType   = useWatch({ control, name: 'document_type' })

  const totals     = (watchedLines ?? []).map(computeLineTotals)

  // QR bill readiness: check what's missing so the payment slip can be
  // generated. Une offre devient une facture d'un clic : elle a besoin des
  // mêmes garanties, sinon le blocage n'apparaît qu'après acceptation.
  const selectedContact = contacts.find(c => c.id === watchedContactId)
  const applyQrGate = watchedDocType === 'invoice' || watchedDocType === 'quote'
  const qrIssues: string[] = []
  const clientAddressIncomplete = !!selectedContact &&
    (!selectedContact.address || !selectedContact.postal_code || !selectedContact.city)
  if (applyQrGate) {
    if (!company?.iban) qrIssues.push(t('nf.qrSansIban'))
    if (selectedContact) {
      if (!selectedContact.address) qrIssues.push(t('nf.qrSansAdresse'))
      if (!selectedContact.postal_code || !selectedContact.city) qrIssues.push(t('nf.qrSansNPA'))
    }
  }
  const subtotal   = totals.reduce((s, t) => s + t.base,  0)
  const totalVAT   = totals.reduce((s, t) => s + t.vat,   0)
  const grandTotal = totals.reduce((s, t) => s + t.total, 0)

  const save = useMutation({
    mutationFn: (data: FormData) => invoicesApi.update(invoiceId!, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['invoice', invoiceId] })
      qc.invalidateQueries({ queryKey: ['invoices'] })
      navigate(`/invoices/${invoiceId}`)
    },
  })

  if (isLoading) return <LoadingSpinner />
  if (loadError || !invoice) return <ErrorBanner message={t('nf.factureIntrouvable')} />

  // Block editing if payment has been recorded
  if (invoice.amount_paid > 0) {
    return (
      <div className="max-w-2xl mx-auto pt-10">
        <ErrorBanner message={t('nf.paiementEnregistre')} />
        <button onClick={() => navigate(`/invoices/${invoiceId}`)}
          className="btn-secondary mt-4 flex items-center gap-2">
          <ArrowLeft size={15} /> {t('nf.retourFacture')}
        </button>
      </div>
    )
  }

  return (
    <div>
      {showContactModal && (
        <NewContactModal
          onClose={() => setShowContactModal(false)}
          onCreated={(contact) => {
            setShowContactModal(false)
            // Poser le contact dans le cache, PUIS le selectionner apres rendu.
            //
            // Deux ecueils se referment ici. L'ordre d'origine attendait le
            // rechargement avant de selectionner : le champ restait vide le
            // temps de l'appel reseau, et pour toujours si celui-ci echouait,
            // puisque rien n'attrapait le rejet. Mais selectionner sans plus
            // attendre ne suffit pas non plus : tant que la liste n'a pas ete
            // rechargee, le <select> n'a AUCUNE option portant cet
            // identifiant, et la valeur posee ne s'affiche pas -- verifie en
            // navigateur, le champ restait sur « Selectionnez un contact ».
            //
            // On insere donc le contact dans le cache, ce qui cree l'option
            // immediatement et sans reseau. La modale a deja demande le
            // rechargement de son cote : la liste faisant foi arrive juste
            // apres et remplace celle-ci.
            //
            // La selection, elle, attend le tour de rendu suivant. setValue
            // ecrit dans l'element du DOM, et React n'a pas encore pose la
            // nouvelle option quand la ligne au-dessus rend la main : le
            // <select> refusait la valeur en silence. Le .finally donne ce
            // tour, et il s'execute que le rechargement reussisse ou non --
            // l'option existe de toute facon, elle vient du cache.
            qc.setQueryData<Contact[]>(['contacts'], (liste) =>
              liste ? [...liste, contact] : [contact])
            void qc.invalidateQueries({ queryKey: ['contacts'] })
              .finally(() => setValue('contact_id', contact.id, { shouldValidate: true }))
          }}
        />
      )}

      <PageHeader
        title={t(invoice.document_type === 'quote' ? 'nf.modifierOffre' : 'nf.modifierFacture',
          { numero: invoice.invoice_number })}
        actions={
          <button onClick={() => navigate(`/invoices/${invoiceId}`)} className="btn-secondary">
            <ArrowLeft size={15} /> {t('action.retour')}
          </button>
        }
      />

      {save.isError && (
        <ErrorBanner message={refusalMessage(save.error, t('nf.erreurSauvegarde'))} />
      )}

      <form onSubmit={handleSubmit(d => save.mutate(d))} className="space-y-5">
        {/* Infos document */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('nf.infosDocument')}</h2>
          </div>
          <div className="card-body grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <label className="label">{t('nf.type')}</label>
              <select className="select" {...register('document_type')}>
                <option value="invoice">{t('doc.facture')}</option>
                <option value="quote">{t('doc.offre')}</option>
                <option value="credit_note">{t('doc.noteDeCredit')}</option>
              </select>
            </div>
            <div className="col-span-2 md:col-span-1">
              <label className="label">{t('nf.contact')}</label>
              <div className="flex gap-2">
                <select className={`select flex-1 ${errors.contact_id ? 'input-error' : ''}`}
                  {...register('contact_id')}>
                  <option value="">{t('nf.choisirContact')}</option>
                  {contacts.map(c => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
                <button type="button" onClick={() => setShowContactModal(true)}
                  className="btn-secondary btn-sm shrink-0 flex items-center gap-1.5"
                  title={t('nf.nouveauContactInfoBulle')}>
                  <UserPlus size={14} />
                  <span className="hidden sm:inline">{t('nf.nouveau')}</span>
                </button>
              </div>
              {errors.contact_id && <p className="error-msg">{tv(errors.contact_id.message)}</p>}
            </div>
            <div>
              <label className="label">{t('nf.dateEmission')}</label>
              <input type="date"
                className={`input ${errors.issue_date ? 'input-error' : ''}`}
                {...register('issue_date')} />
            </div>
            <div>
              <label className="label">{t('nf.echeance')}</label>
              <input type="date"
                className={`input ${errors.due_date ? 'input-error' : ''}`}
                {...register('due_date')} />
              {errors.due_date && <p className="error-msg">{tv(errors.due_date.message)}</p>}
            </div>
          </div>
        </div>

        {/* QR bill readiness warning — bloquant si l'adresse du client manque,
            simple avertissement si seul l'IBAN de la société manque encore. */}
        {applyQrGate && qrIssues.length > 0 && (
          <div className={`flex items-start gap-2.5 rounded-lg border px-4 py-3 text-sm ${
            clientAddressIncomplete
              ? 'border-danger-200 bg-danger-100/70'
              : 'border-warning-100 bg-warning-100/70'
          }`}>
            <AlertTriangle size={15} className={`mt-0.5 flex-shrink-0 ${
              clientAddressIncomplete ? 'text-danger-500' : 'text-warning-500'
            }`} />
            <div>
              <p className={`font-medium ${clientAddressIncomplete ? 'text-danger-700' : 'text-warning-700'}`}>
                {t('nf.qrIncomplet')}
              </p>
              <ul className={`mt-1 space-y-0.5 list-disc list-inside text-xs ${
                clientAddressIncomplete ? 'text-danger-700' : 'text-warning-700'
              }`}>
                {qrIssues.map((issue, i) => <li key={i}>{issue}</li>)}
              </ul>
            </div>
          </div>
        )}

        {/* Lignes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('fd.lignes')}</h2>
            <button type="button" className="btn-secondary btn-sm"
              onClick={() => append({ description: '', quantity: 1, unit_price: 0, discount_pct: 0, vat_rate: 8.1 })}>
              <Plus size={14} /> {t('nf.ajouterLigne')}
            </button>
          </div>
          <div className="card-body p-0">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-alpine-50 border-b border-alpine-200">
                    {([
                      'jr.colDescription', 'fd.colQte', 'nf.colUnite', 'nf.colPrixUnit',
                      'nf.colRabaisPct', 'nf.colTVAPct', 'nf.colTotalHT',
                    ] as const).map(cle => (
                      <th key={cle} className="px-4 py-2.5 text-left text-xs font-semibold text-alpine-600 uppercase tracking-wide">
                        {t(cle)}
                      </th>
                    ))}
                    <th className="px-4 py-2.5" />
                  </tr>
                </thead>
                <tbody>
                  {fields.map((field, i) => {
                    const ligne = totals[i] ?? { base: 0, vat: 0, total: 0 }
                    return (
                      <tr key={field.id} className="border-b border-alpine-100 last:border-0">
                        <td className="px-4 py-2 w-[30%]">
                          <input
                            className={`input ${errors.lines?.[i]?.description ? 'input-error' : ''}`}
                            placeholder={t('nf.placeholderDescription')}
                            aria-label={t('nf.ariaDescription', { n: i + 1 })}
                            {...register(`lines.${i}.description`)}
                          />
                        </td>
                        <td className="px-2 py-2 w-20">
                          <input type="number" step="0.001" min="0.001"
                            aria-label={t('nf.ariaQuantite', { n: i + 1 })}
                            className="input text-right" {...register(`lines.${i}.quantity`)} />
                        </td>
                        <td className="px-2 py-2 w-16">
                          <input className="input" placeholder={t('nf.placeholderUnite')}
                            aria-label={t('nf.ariaUnite', { n: i + 1 })}
                            {...register(`lines.${i}.unit`)} />
                        </td>
                        <td className="px-2 py-2 w-28">
                          <input type="number" step="0.01" min="0"
                            aria-label={t('nf.ariaPrix', { n: i + 1 })}
                            className={`input text-right font-mono ${errors.lines?.[i]?.unit_price ? 'input-error' : ''}`}
                            {...register(`lines.${i}.unit_price`)} />
                        </td>
                        <td className="px-2 py-2 w-20">
                          <input type="number" step="0.1" min="0" max="100"
                            aria-label={t('nf.ariaRabais', { n: i + 1 })}
                            className="input text-right"
                            {...register(`lines.${i}.discount_pct`)} />
                        </td>
                        <td className="px-2 py-2 w-20">
                          <select className="select text-right" {...register(`lines.${i}.vat_rate`)}>
                            <option value="8.1">8.1%</option>
                            <option value="2.6">2.6%</option>
                            <option value="3.8">3.8%</option>
                            <option value="0">0%</option>
                          </select>
                        </td>
                        <td className="px-4 py-2 text-right font-mono text-alpine-800 whitespace-nowrap">
                          {formatCHF(ligne.base)}
                        </td>
                        <td className="px-2 py-2">
                          {fields.length > 1 && (
                            <button type="button" onClick={() => remove(i)}
                              className="btn-ghost btn-sm text-danger-500 hover:text-danger-700 p-1">
                              <Trash2 size={14} />
                            </button>
                          )}
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </div>
          <div className="card-footer flex justify-end">
            <div className="w-64 space-y-1 text-sm">
              <div className="flex justify-between text-alpine-600">
                <span>{t('fd.sousTotalHT')}</span>
                <span className="font-mono">{formatCHF(subtotal)}</span>
              </div>
              <div className="flex justify-between text-alpine-600">
                <span>{t('tva.tva')}</span>
                <span className="font-mono">{formatCHF(totalVAT)}</span>
              </div>
              <div className="flex justify-between font-semibold text-base text-alpine-900 pt-1 border-t border-alpine-200">
                <span>{t('fact.colTotal')}</span>
                <span className="font-mono">{formatCHF(grandTotal)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Notes / Conditions */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('fd.remarques')}</h2>
          </div>
          <div className="card-body grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="label">{t('nf.notesInternes')}</label>
              <textarea rows={3} className="input resize-none" {...register('notes')} />
            </div>
            <div>
              <label className="label">{t('nf.conditionsPaiement')}</label>
              <textarea rows={3} className="input resize-none"
                placeholder={t('nf.placeholderConditions')}
                {...register('terms')} />
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 pb-6">
          <button type="button" onClick={() => navigate(`/invoices/${invoiceId}`)}
            className="btn-secondary">{t('action.annuler')}</button>
          <button type="submit" className="btn-primary"
            disabled={save.isPending || (applyQrGate && clientAddressIncomplete)}>
            <Save size={15} />
            {save.isPending ? t('etat.enregistrement') : t('ach.enregistrerModifs')}
          </button>
        </div>
      </form>
    </div>
  )
}
