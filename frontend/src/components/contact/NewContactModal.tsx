// LedgerAlps — Modal création de contact

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { contactsApi } from '@/api/client'
import { ErrorBanner } from '@/components/ui'

// Convert empty strings to undefined so optional fields don't trip validation.
const opt = <T extends z.ZodTypeAny>(s: T) =>
  z.preprocess(v => (v === '' ? undefined : v), s.optional()) as z.ZodOptional<T>

const schema = z.object({
  contact_type:      z.enum(['customer', 'supplier', 'both']),
  is_company:        z.boolean().default(false),
  name:              z.string().min(1, 'Nom requis'),
  legal_name:        opt(z.string()),
  address:           opt(z.string()),
  postal_code:       opt(z.string()),
  city:              opt(z.string()),
  country:           z.string().min(2).max(2).default('CH'),
  uid_number:        opt(z.string()),
  vat_number:        opt(z.string()),
  email:             opt(z.string().email('E-mail invalide')),
  phone:             opt(z.string()),
  payment_term_days: z.coerce.number().int().min(0).max(365).default(30),
  iban:              opt(z.string()),
  notes:             opt(z.string()),
})

type FormData = z.infer<typeof schema>

interface Props { onClose: () => void }

export function NewContactModal({ onClose }: Props) {
  const qc = useQueryClient()
  const {
    register, handleSubmit,
    formState: { errors },
  } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      contact_type:      'customer',
      is_company:        false,
      country:           'CH',
      payment_term_days: 30,
    },
  })

  const create = useMutation({
    mutationFn: (data: FormData) => contactsApi.create(data),
    onSuccess:  () => {
      qc.invalidateQueries({ queryKey: ['contacts'] })
      onClose()
    },
  })

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Overlay */}
      <div
        className="absolute inset-0 bg-alpine-900/50 backdrop-blur-sm"
        onClick={onClose}
      />

      {/* Modal */}
      <div className="relative bg-white rounded-2xl shadow-modal w-full max-w-2xl
                      max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between px-6 py-4 border-b border-alpine-100">
          <h2 className="font-display font-700 text-lg text-alpine-900">Nouveau contact</h2>
          <button onClick={onClose} className="btn-ghost p-1.5">
            <X size={18} />
          </button>
        </div>

        <form onSubmit={handleSubmit(d => create.mutate(d))} className="px-6 py-5 space-y-5">
          {create.error && (
            <ErrorBanner message={
              (create.error as any)?.response?.data?.error
                ?? 'Erreur lors de la création du contact.'
            } />
          )}

          {/* Type et nature */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Type *</label>
              <select className="select" {...register('contact_type')}>
                <option value="customer">Client</option>
                <option value="supplier">Fournisseur</option>
                <option value="both">Les deux</option>
              </select>
            </div>
            <div className="flex items-end pb-2">
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" {...register('is_company')} className="rounded border-alpine-300 accent-accent-500" />
                <span className="text-sm text-alpine-700">Entreprise</span>
              </label>
            </div>
          </div>

          {/* Nom */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">Nom / Raison sociale *</label>
              <input className={`input ${errors.name ? 'input-error' : ''}`}
                {...register('name')} />
              {errors.name && <p className="error-msg">{errors.name.message}</p>}
            </div>
            <div>
              <label className="label">Raison sociale légale</label>
              <input className="input" {...register('legal_name')} />
            </div>
          </div>

          {/* Adresse */}
          <div>
            <label className="label">Adresse</label>
            <input className="input mb-2" placeholder="Rue et numéro"
              {...register('address')} />
            <div className="grid grid-cols-3 gap-3">
              <input className="input" placeholder="NPA" {...register('postal_code')} />
              <input className="input col-span-2" placeholder="Localité" {...register('city')} />
            </div>
          </div>

          {/* Pays + Numéros légaux */}
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="label">Pays</label>
              <input className="input uppercase" maxLength={2} {...register('country')} />
            </div>
            <div>
              <label className="label">N° IDE (CHE-…)</label>
              <input className="input font-mono" placeholder="CHE-123.456.789"
                {...register('uid_number')} />
            </div>
            <div>
              <label className="label">N° TVA</label>
              <input className="input font-mono" {...register('vat_number')} />
            </div>
          </div>

          {/* Contact */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">E-mail</label>
              <input type="email" className={`input ${errors.email ? 'input-error' : ''}`}
                {...register('email')} />
            </div>
            <div>
              <label className="label">Téléphone</label>
              <input type="tel" className="input" {...register('phone')} />
            </div>
          </div>

          {/* Paiement */}
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="label">Délai paiement (jours)</label>
              <input type="number" min="0" max="365" className="input"
                {...register('payment_term_days')} />
            </div>
            <div className="col-span-2">
              <label className="label">IBAN</label>
              <input className="input font-mono" placeholder="CH…"
                {...register('iban')} />
            </div>
          </div>

          {/* Notes */}
          <div>
            <label className="label">Notes</label>
            <textarea rows={2} className="input resize-none" {...register('notes')} />
          </div>

          {/* Actions */}
          <div className="flex justify-end gap-3 pt-2 border-t border-alpine-100">
            <button type="button" onClick={onClose} className="btn-secondary">
              Annuler
            </button>
            <button type="submit" className="btn-primary" disabled={create.isPending}>
              {create.isPending ? 'Enregistrement…' : 'Créer le contact'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
