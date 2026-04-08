// LedgerAlps — Journal comptable

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm, useFieldArray } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Plus, Trash2, CheckCircle } from 'lucide-react'
import { journalApi } from '@/api/client'
import { PageHeader, LoadingSpinner, EmptyState, ErrorBanner } from '@/components/ui'
import { formatDate, formatCHF } from '@/utils'
import type { JournalEntry } from '@/types'

const lineSchema = z.object({
  debit_account:  z.string().optional(),
  credit_account: z.string().optional(),
  amount:         z.coerce.number().positive('Montant requis'),
  currency:       z.string().default('CHF'),
  description:    z.string().optional(),
})

const schema = z.object({
  date:        z.string().min(1),
  description: z.string().min(1, 'Description requise'),
  reference:   z.string().optional(),
  lines:       z.array(lineSchema).min(1),
})

type FormData = z.infer<typeof schema>

const STATUS_LABEL: Record<string, string> = {
  draft:    'Brouillon',
  posted:   'Validé',
  reversed: 'Contrepassé',
}
const STATUS_CLASS: Record<string, string> = {
  draft:    'badge-draft',
  posted:   'badge-paid',
  reversed: 'badge-cancelled',
}

export function JournalPage() {
  const [showForm, setShowForm] = useState(false)
  const [startDate] = useState(() => new Date(new Date().getFullYear(), 0, 1).toISOString().slice(0, 10))
  const [endDate]   = useState(() => new Date().toISOString().slice(0, 10))
  const qc = useQueryClient()

  // On simule une liste — en production, il faudrait un endpoint GET /journal
  const { data: entries = [], isLoading } = useQuery<JournalEntry[]>({
    queryKey: ['journal-entries'],
    queryFn:  () => Promise.resolve([]),  // Remplacer par api.get('/journal')
  })

  const { register, control, handleSubmit, reset, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      date:  new Date().toISOString().slice(0, 10),
      lines: [
        { debit_account: '', credit_account: '', amount: 0, currency: 'CHF' },
      ],
    },
  })

  const { fields, append, remove } = useFieldArray({ control, name: 'lines' })

  const create = useMutation({
    mutationFn: (data: FormData) => journalApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['journal-entries'] })
      reset()
      setShowForm(false)
    },
  })

  return (
    <div>
      <PageHeader
        title="Journal"
        subtitle="Écritures comptables — CO art. 957a"
        actions={
          <div className="flex gap-2">
            <button onClick={() => setShowForm(!showForm)} className="btn-primary">
              <Plus size={15} /> Nouvelle écriture
            </button>
          </div>
        }
      />

      {/* Formulaire écriture manuelle */}
      {showForm && (
        <div className="card mb-5">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Écriture manuelle</h2>
            <span className="text-xs text-alpine-400">
              La partie double est vérifiée : sum(débit) = sum(crédit)
            </span>
          </div>
          <form onSubmit={handleSubmit(d => create.mutate(d))}>
            <div className="card-body grid grid-cols-3 gap-4 pb-4">
              <div className="col-span-2">
                <label className="label">Description *</label>
                <input className={`input ${errors.description ? 'input-error' : ''}`}
                  {...register('description')} />
              </div>
              <div>
                <label className="label">Date *</label>
                <input type="date" className="input" {...register('date')} />
              </div>
            </div>

            {/* Lignes */}
            <div className="px-6 pb-4">
              <div className="flex items-center justify-between mb-2">
                <label className="label mb-0">Lignes débit/crédit</label>
                <button type="button" className="btn-ghost btn-sm"
                  onClick={() => append({ debit_account: '', credit_account: '', amount: 0, currency: 'CHF' })}>
                  <Plus size={13} /> Ligne
                </button>
              </div>
              <div className="border border-alpine-200 rounded-lg overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="bg-alpine-50 border-b border-alpine-200">
                      <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">Cpt débit</th>
                      <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">Cpt crédit</th>
                      <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">Montant CHF</th>
                      <th className="px-3 py-2 text-left text-xs font-semibold text-alpine-600">Description</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {fields.map((f, i) => (
                      <tr key={f.id} className="border-t border-alpine-100">
                        <td className="px-2 py-2 w-28">
                          <input className="input font-mono" placeholder="1100"
                            {...register(`lines.${i}.debit_account`)} />
                        </td>
                        <td className="px-2 py-2 w-28">
                          <input className="input font-mono" placeholder="6100"
                            {...register(`lines.${i}.credit_account`)} />
                        </td>
                        <td className="px-2 py-2 w-32">
                          <input type="number" step="0.01" className="input text-right font-mono"
                            {...register(`lines.${i}.amount`)} />
                        </td>
                        <td className="px-2 py-2">
                          <input className="input" placeholder="Libellé"
                            {...register(`lines.${i}.description`)} />
                        </td>
                        <td className="px-2 py-2">
                          {fields.length > 1 && (
                            <button type="button" onClick={() => remove(i)}
                              className="btn-ghost btn-sm p-1 text-danger-500">
                              <Trash2 size={13} />
                            </button>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            {create.error && <div className="px-6"><ErrorBanner message="Erreur : vérifiez la partie double (débit = crédit)." /></div>}

            <div className="card-footer flex justify-end gap-3">
              <button type="button" onClick={() => setShowForm(false)} className="btn-secondary">
                Annuler
              </button>
              <button type="submit" className="btn-primary" disabled={create.isPending}>
                <CheckCircle size={15} />
                {create.isPending ? 'Enregistrement…' : 'Enregistrer'}
              </button>
            </div>
          </form>
        </div>
      )}

      {/* Liste des écritures */}
      <div className="table-wrapper">
        <table className="table">
          <thead>
            <tr>
              <th>Réf.</th>
              <th>Date</th>
              <th>Description</th>
              <th className="text-right">Montant CHF</th>
              <th>Statut</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {isLoading && <tr><td colSpan={6}><LoadingSpinner /></td></tr>}
            {!isLoading && entries.length === 0 && (
              <tr>
                <td colSpan={6}>
                  <EmptyState
                    title="Journal vide"
                    description="Les écritures apparaîtront ici après leur création."
                  />
                </td>
              </tr>
            )}
            {entries.map(e => (
              <tr key={e.id}>
                <td><span className="font-mono text-accent-700 text-xs">{e.reference}</span></td>
                <td>{formatDate(e.date)}</td>
                <td className="max-w-xs truncate text-alpine-700">{e.description}</td>
                <td className="text-right font-mono tabular-nums">
                  {formatCHF(e.lines.reduce((s, l) => s + parseFloat(l.amount_chf), 0))}
                </td>
                <td>
                  <span className={`badge ${STATUS_CLASS[e.status] ?? 'badge-draft'}`}>
                    {STATUS_LABEL[e.status] ?? e.status}
                  </span>
                </td>
                <td>
                  {e.status === 'draft' && (
                    <button
                      onClick={() => {
                        journalApi.post(e.id).then(() =>
                          qc.invalidateQueries({ queryKey: ['journal-entries'] })
                        )
                      }}
                      className="btn-ghost btn-sm text-success-700"
                    >
                      Valider
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
