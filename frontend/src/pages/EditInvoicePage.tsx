// LedgerAlps — Modification d'une facture / offre de prix

import { useEffect, useState } from 'react'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { Plus, Trash2, ArrowLeft, Save, UserPlus, X } from 'lucide-react'
import { invoicesApi, contactsApi } from '@/api/client'
import { PageHeader, ErrorBanner, LoadingSpinner } from '@/components/ui'
import { formatCHF } from '@/utils'
import type { Contact, Invoice } from '@/types'

// ── Schéma identique à NewInvoicePage ─────────────────────────────────────────

const lineSchema = z.object({
  description: z.string().min(1, 'Requis'),
  quantity:    z.coerce.number().positive(),
  unit:        z.string().optional(),
  unit_price:  z.coerce.number().positive('Prix requis'),
  discount_pct: z.coerce.number().min(0).max(100).default(0),
  vat_rate:    z.coerce.number().min(0).default(8.1),
})

const schema = z.object({
  document_type: z.enum(['invoice', 'quote', 'credit_note']).default('invoice'),
  contact_id:    z.string().min(1, 'Sélectionnez un contact'),
  issue_date:    z.string().min(1, 'Date requise'),
  due_date:      z.string().min(1, 'Échéance requise'),
  notes:         z.string().optional(),
  terms:         z.string().optional(),
  lines:         z.array(lineSchema).min(1, 'Au moins une ligne'),
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

const EMPTY_CONTACT = { name: '', is_company: false, email: '', phone: '', city: '', country: 'CH' }

function NewContactModal({
  onClose,
  onCreated,
}: { onClose: () => void; onCreated: (c: Contact) => void }) {
  const qc = useQueryClient()
  const [fields, setFields] = useState(EMPTY_CONTACT)
  const [err, setErr] = useState<string | null>(null)

  const create = useMutation({
    mutationFn: () => contactsApi.create({
      contact_type: 'customer', is_company: fields.is_company,
      name: fields.name.trim(), email: fields.email || undefined,
      phone: fields.phone || undefined, city: fields.city || undefined,
      country: fields.country || 'CH', payment_term_days: 30,
    }),
    onSuccess: (res) => { qc.invalidateQueries({ queryKey: ['contacts'] }); onCreated(res.data as Contact) },
    onError:   () => setErr('Erreur lors de la création du contact.'),
  })

  const set = (k: keyof typeof EMPTY_CONTACT, v: string | boolean) =>
    setFields(f => ({ ...f, [k]: v }))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div className="bg-white rounded-xl shadow-2xl w-full max-w-md mx-4">
        <div className="flex items-center justify-between px-5 py-4 border-b border-alpine-200">
          <h3 className="text-sm font-semibold text-alpine-900">Nouveau contact</h3>
          <button type="button" onClick={onClose} className="btn-ghost btn-sm p-1 text-alpine-400">
            <X size={16} />
          </button>
        </div>
        <div className="px-5 py-4 space-y-3">
          {err && <p className="text-xs text-danger-600 bg-danger-50 rounded px-3 py-2">{err}</p>}
          <div className="flex items-center gap-2">
            <input id="nc_co" type="checkbox" checked={fields.is_company}
              onChange={e => set('is_company', e.target.checked)}
              className="rounded border-alpine-300 text-alpine-700" />
            <label htmlFor="nc_co" className="text-sm text-alpine-700">Entreprise</label>
          </div>
          <div>
            <label className="label">Nom *</label>
            <input className="input" placeholder={fields.is_company ? 'Raison sociale' : 'Prénom Nom'}
              value={fields.name} onChange={e => set('name', e.target.value)} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div><label className="label">E-mail</label>
              <input type="email" className="input" value={fields.email}
                onChange={e => set('email', e.target.value)} /></div>
            <div><label className="label">Téléphone</label>
              <input type="tel" className="input" value={fields.phone}
                onChange={e => set('phone', e.target.value)} /></div>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2"><label className="label">Ville</label>
              <input className="input" value={fields.city}
                onChange={e => set('city', e.target.value)} /></div>
            <div><label className="label">Pays</label>
              <input className="input" maxLength={2} value={fields.country}
                onChange={e => set('country', e.target.value.toUpperCase())} /></div>
          </div>
        </div>
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-alpine-200">
          <button type="button" onClick={onClose} className="btn-secondary btn-sm">Annuler</button>
          <button type="button" disabled={!fields.name.trim() || create.isPending}
            onClick={() => create.mutate()} className="btn-primary btn-sm">
            {create.isPending ? 'Création…' : 'Créer le contact'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Page principale ────────────────────────────────────────────────────────────

export function EditInvoicePage() {
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
  const watchedLines = useWatch({ control, name: 'lines' })

  const totals     = (watchedLines ?? []).map(computeLineTotals)
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
  if (loadError || !invoice) return <ErrorBanner message="Facture introuvable." />

  // Block editing if payment has been recorded
  if (invoice.amount_paid > 0) {
    return (
      <div className="max-w-2xl mx-auto pt-10">
        <ErrorBanner message="Cette facture a un paiement enregistré et ne peut plus être modifiée." />
        <button onClick={() => navigate(`/invoices/${invoiceId}`)}
          className="btn-secondary mt-4 flex items-center gap-2">
          <ArrowLeft size={15} /> Retour à la facture
        </button>
      </div>
    )
  }

  const docLabel = invoice.document_type === 'quote' ? 'offre' : 'facture'

  return (
    <div>
      {showContactModal && (
        <NewContactModal
          onClose={() => setShowContactModal(false)}
          onCreated={(contact) => {
            setShowContactModal(false)
            qc.invalidateQueries({ queryKey: ['contacts'] }).then(() => {
              setValue('contact_id', contact.id, { shouldValidate: true })
            })
          }}
        />
      )}

      <PageHeader
        title={`Modifier ${docLabel} ${invoice.invoice_number}`}
        actions={
          <button onClick={() => navigate(`/invoices/${invoiceId}`)} className="btn-secondary">
            <ArrowLeft size={15} /> Retour
          </button>
        }
      />

      {save.isError && (
        <ErrorBanner message={
          (save.error as any)?.response?.data?.error ?? 'Erreur lors de la sauvegarde.'
        } />
      )}

      <form onSubmit={handleSubmit(d => save.mutate(d))} className="space-y-5">
        {/* Infos document */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Informations du document</h2>
          </div>
          <div className="card-body grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <label className="label">Type *</label>
              <select className="select" {...register('document_type')}>
                <option value="invoice">Facture</option>
                <option value="quote">Offre de prix</option>
                <option value="credit_note">Note de crédit</option>
              </select>
            </div>
            <div className="col-span-2 md:col-span-1">
              <label className="label">Contact *</label>
              <div className="flex gap-2">
                <select className={`select flex-1 ${errors.contact_id ? 'input-error' : ''}`}
                  {...register('contact_id')}>
                  <option value="">— Sélectionnez un contact —</option>
                  {contacts.map(c => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
                <button type="button" onClick={() => setShowContactModal(true)}
                  className="btn-secondary btn-sm shrink-0 flex items-center gap-1.5"
                  title="Créer un nouveau contact">
                  <UserPlus size={14} />
                  <span className="hidden sm:inline">Nouveau</span>
                </button>
              </div>
              {errors.contact_id && <p className="error-msg">{errors.contact_id.message}</p>}
            </div>
            <div>
              <label className="label">Date d'émission *</label>
              <input type="date"
                className={`input ${errors.issue_date ? 'input-error' : ''}`}
                {...register('issue_date')} />
            </div>
            <div>
              <label className="label">Échéance *</label>
              <input type="date"
                className={`input ${errors.due_date ? 'input-error' : ''}`}
                {...register('due_date')} />
              {errors.due_date && <p className="error-msg">{errors.due_date.message}</p>}
            </div>
          </div>
        </div>

        {/* Lignes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Lignes</h2>
            <button type="button" className="btn-secondary btn-sm"
              onClick={() => append({ description: '', quantity: 1, unit_price: 0, discount_pct: 0, vat_rate: 8.1 })}>
              <Plus size={14} /> Ajouter une ligne
            </button>
          </div>
          <div className="card-body p-0">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-alpine-50 border-b border-alpine-200">
                    {['Description', 'Qté', 'Unité', 'Prix unit.', 'Rabais %', 'TVA %', 'Total HT', ''].map(h => (
                      <th key={h} className="px-4 py-2.5 text-left text-xs font-semibold text-alpine-600 uppercase tracking-wide">
                        {h}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {fields.map((field, i) => {
                    const t = totals[i] ?? { base: 0, vat: 0, total: 0 }
                    return (
                      <tr key={field.id} className="border-b border-alpine-100 last:border-0">
                        <td className="px-4 py-2 w-[30%]">
                          <input
                            className={`input ${errors.lines?.[i]?.description ? 'input-error' : ''}`}
                            placeholder="Description du service ou produit"
                            {...register(`lines.${i}.description`)}
                          />
                        </td>
                        <td className="px-2 py-2 w-20">
                          <input type="number" step="0.001" min="0.001"
                            className="input text-right" {...register(`lines.${i}.quantity`)} />
                        </td>
                        <td className="px-2 py-2 w-16">
                          <input className="input" placeholder="h, pce…"
                            {...register(`lines.${i}.unit`)} />
                        </td>
                        <td className="px-2 py-2 w-28">
                          <input type="number" step="0.01" min="0"
                            className={`input text-right font-mono ${errors.lines?.[i]?.unit_price ? 'input-error' : ''}`}
                            {...register(`lines.${i}.unit_price`)} />
                        </td>
                        <td className="px-2 py-2 w-20">
                          <input type="number" step="0.1" min="0" max="100"
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
                          {formatCHF(t.base)}
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
                <span>Sous-total HT</span>
                <span className="font-mono">{formatCHF(subtotal)}</span>
              </div>
              <div className="flex justify-between text-alpine-600">
                <span>TVA</span>
                <span className="font-mono">{formatCHF(totalVAT)}</span>
              </div>
              <div className="flex justify-between font-semibold text-base text-alpine-900 pt-1 border-t border-alpine-200">
                <span>Total CHF</span>
                <span className="font-mono">{formatCHF(grandTotal)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Notes / Conditions */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Remarques</h2>
          </div>
          <div className="card-body grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="label">Notes internes</label>
              <textarea rows={3} className="input resize-none" {...register('notes')} />
            </div>
            <div>
              <label className="label">Conditions de paiement</label>
              <textarea rows={3} className="input resize-none"
                placeholder="Paiement à 30 jours. IBAN : CH…"
                {...register('terms')} />
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 pb-6">
          <button type="button" onClick={() => navigate(`/invoices/${invoiceId}`)}
            className="btn-secondary">Annuler</button>
          <button type="submit" className="btn-primary" disabled={save.isPending}>
            <Save size={15} />
            {save.isPending ? 'Enregistrement…' : 'Enregistrer les modifications'}
          </button>
        </div>
      </form>
    </div>
  )
}
