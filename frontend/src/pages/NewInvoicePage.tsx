// LedgerAlps — Formulaire de création de facture

import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Plus, Trash2, ArrowLeft } from 'lucide-react'
import { invoicesApi, contactsApi } from '@/api/client'
import { PageHeader, ErrorBanner } from '@/components/ui'
import { formatCHF } from '@/utils'
import type { Contact } from '@/types'

const lineSchema = z.object({
  description:      z.string().min(1, 'Requis'),
  quantity:         z.coerce.number().positive(),
  unit:             z.string().optional(),
  unit_price:       z.coerce.number().positive('Prix requis'),
  discount_percent: z.coerce.number().min(0).max(100).default(0),
  vat_rate:         z.coerce.number().min(0).default(8.1),
})

const schema = z.object({
  contact_id:   z.string().min(1, 'Sélectionnez un contact'),
  issue_date:   z.string().min(1, 'Date requise'),
  due_date:     z.string().optional(),
  notes:        z.string().optional(),
  terms:        z.string().optional(),
  lines:        z.array(lineSchema).min(1, 'Au moins une ligne'),
})

type FormData = z.infer<typeof schema>

function computeLineTotals(line: Partial<FormData['lines'][0]>) {
  const qty      = Number(line.quantity     ?? 1)
  const price    = Number(line.unit_price   ?? 0)
  const discount = Number(line.discount_percent ?? 0) / 100
  const vatRate  = Number(line.vat_rate     ?? 8.1) / 100
  const base     = qty * price * (1 - discount)
  const vat      = Math.round(base * vatRate * 20) / 20  // arrondi 5 rappen
  return { base: Math.round(base * 100) / 100, vat: Math.round(vat * 100) / 100, total: Math.round((base + vat) * 100) / 100 }
}

export function NewInvoicePage() {
  const navigate  = useNavigate()
  const qc        = useQueryClient()

  const { data: contacts = [] } = useQuery<Contact[]>({
    queryKey: ['contacts', 'customer'],
    queryFn:  () => contactsApi.list({ contact_type: 'customer' }).then(r => r.data),
  })

  const {
    register, control, handleSubmit,
    formState: { errors },
  } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      issue_date: new Date().toISOString().slice(0, 10),
      lines: [{ description: '', quantity: 1, unit_price: 0, discount_percent: 0, vat_rate: 8.1 }],
    },
  })

  const { fields, append, remove } = useFieldArray({ control, name: 'lines' })
  const watchedLines = useWatch({ control, name: 'lines' })

  const totals = (watchedLines ?? []).map(computeLineTotals)
  const subtotal   = totals.reduce((s, t) => s + t.base,  0)
  const totalVAT   = totals.reduce((s, t) => s + t.vat,   0)
  const grandTotal = totals.reduce((s, t) => s + t.total, 0)

  const create = useMutation({
    mutationFn: (data: FormData) => invoicesApi.create(data),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['invoices'] })
      navigate(`/invoices/${res.data.id}`)
    },
  })

  const today = new Date().toISOString().slice(0, 10)

  return (
    <div>
      <PageHeader
        title="Nouvelle facture"
        actions={
          <button onClick={() => navigate(-1)} className="btn-secondary">
            <ArrowLeft size={15} /> Retour
          </button>
        }
      />

      {create.error && <ErrorBanner message="Erreur lors de la création." />}

      <form onSubmit={handleSubmit(d => create.mutate(d))} className="space-y-5">
        {/* Infos document */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Informations du document</h2>
          </div>
          <div className="card-body grid grid-cols-2 md:grid-cols-4 gap-4">
            {/* Contact */}
            <div className="col-span-2">
              <label className="label">Contact *</label>
              <select
                className={`select ${errors.contact_id ? 'input-error' : ''}`}
                {...register('contact_id')}
              >
                <option value="">— Sélectionnez un contact —</option>
                {contacts.map(c => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>
              {errors.contact_id && <p className="error-msg">{errors.contact_id.message}</p>}
            </div>

            {/* Date émission */}
            <div>
              <label className="label">Date d'émission *</label>
              <input
                type="date"
                className={`input ${errors.issue_date ? 'input-error' : ''}`}
                defaultValue={today}
                {...register('issue_date')}
              />
            </div>

            {/* Échéance */}
            <div>
              <label className="label">Échéance</label>
              <input type="date" className="input" {...register('due_date')} />
            </div>

          </div>
        </div>

        {/* Lignes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Lignes de facture</h2>
            <button
              type="button"
              className="btn-secondary btn-sm"
              onClick={() => append({ description: '', quantity: 1, unit_price: 0, discount_percent: 0, vat_rate: 8.1 })}
            >
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
                            className="input text-right" {...register(`lines.${i}.discount_percent`)} />
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
          {/* Totaux */}
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
            <div className="col-span-2">
              <label className="label">Message de paiement (max 140 car.)</label>
              <input className="input" maxLength={140}
                placeholder={`Facture ${new Date().getFullYear()}-XXXX`}
                {...register('payment_info')} />
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 pb-6">
          <button type="button" onClick={() => navigate(-1)} className="btn-secondary">
            Annuler
          </button>
          <button
            type="submit"
            className="btn-primary"
            disabled={create.isPending}
          >
            <Save size={15} />
            {create.isPending ? 'Enregistrement…' : 'Enregistrer en brouillon'}
          </button>
        </div>
      </form>
    </div>
  )
}
