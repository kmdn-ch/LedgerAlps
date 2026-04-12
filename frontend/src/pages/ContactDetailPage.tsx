// LedgerAlps — Détail et édition d'un contact

import { useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ArrowLeft, Save, Building2, User, PowerOff } from 'lucide-react'
import { contactsApi } from '@/api/client'
import { PageHeader, LoadingSpinner, ErrorBanner } from '@/components/ui'
import type { Contact } from '@/types'

// ─── Schema ───────────────────────────────────────────────────────────────────

const opt = <T extends z.ZodTypeAny>(s: T) =>
  z.preprocess(v => (v === '' ? undefined : v), s.optional())

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

// ─── Page ─────────────────────────────────────────────────────────────────────

export function ContactDetailPage() {
  const { contactId } = useParams<{ contactId: string }>()
  const navigate      = useNavigate()
  const qc            = useQueryClient()

  const { data: contact, isLoading, error } = useQuery<Contact>({
    queryKey: ['contact', contactId],
    queryFn:  () => contactsApi.get(contactId!).then(r => r.data),
    enabled:  !!contactId,
  })

  const {
    register, handleSubmit, reset,
    formState: { errors, isDirty },
  } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      contact_type:      'customer',
      is_company:        false,
      country:           'CH',
      payment_term_days: 30,
    },
  })

  // Populate form once contact loads
  useEffect(() => {
    if (!contact) return
    reset({
      contact_type:      contact.contact_type,
      is_company:        contact.is_company,
      name:              contact.name,
      legal_name:        contact.legal_name ?? '',
      address:           contact.address ?? '',
      postal_code:       contact.postal_code ?? '',
      city:              contact.city ?? '',
      country:           contact.country,
      uid_number:        contact.uid_number ?? '',
      vat_number:        contact.vat_number ?? '',
      email:             contact.email ?? '',
      phone:             contact.phone ?? '',
      payment_term_days: contact.payment_term_days,
      iban:              contact.iban ?? '',
      notes:             contact.notes ?? '',
    })
  }, [contact, reset])

  const save = useMutation({
    mutationFn: (data: FormData) => contactsApi.update(contactId!, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['contact', contactId] })
      qc.invalidateQueries({ queryKey: ['contacts'] })
    },
  })

  const toggleActive = useMutation({
    mutationFn: () => contactsApi.update(contactId!, { is_active: !contact?.is_active }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['contact', contactId] })
      qc.invalidateQueries({ queryKey: ['contacts'] })
    },
  })

  if (isLoading) return <LoadingSpinner />
  if (error || !contact) return <ErrorBanner message="Contact introuvable." />

  return (
    <div className="max-w-3xl mx-auto">
      {/* En-tête */}
      <div className="flex items-center gap-3 mb-6">
        <button onClick={() => navigate('/contacts')} className="btn-ghost btn-sm">
          <ArrowLeft size={15} />
        </button>
        <PageHeader
          title={contact.name}
          subtitle={contact.is_company ? 'Entreprise' : 'Particulier'}
          actions={
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => toggleActive.mutate()}
                disabled={toggleActive.isPending}
                className="btn-ghost btn-sm flex items-center gap-1.5 text-alpine-500"
                title={contact.is_active ? 'Désactiver' : 'Réactiver'}
              >
                <PowerOff size={14} />
                {contact.is_active ? 'Désactiver' : 'Réactiver'}
              </button>
            </div>
          }
        />
      </div>

      {save.isError && (
        <ErrorBanner message={
          (save.error as any)?.response?.data?.error ?? 'Erreur lors de la sauvegarde.'
        } />
      )}
      {save.isSuccess && !isDirty && (
        <div className="mb-4 px-4 py-2.5 rounded-lg bg-success-50 border border-success-200
                        text-sm text-success-700">
          Modifications enregistrées.
        </div>
      )}

      <form onSubmit={handleSubmit(d => save.mutate(d))} className="space-y-5">
        {/* Identité */}
        <div className="card">
          <div className="card-header">
            <div className="flex items-center gap-2">
              {contact.is_company
                ? <Building2 size={15} className="text-alpine-500" />
                : <User      size={15} className="text-alpine-500" />
              }
              <h2 className="text-sm font-semibold text-alpine-800">Identité</h2>
            </div>
          </div>
          <div className="card-body space-y-4">
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
                  <input type="checkbox" {...register('is_company')}
                    className="rounded border-alpine-300 accent-accent-500" />
                  <span className="text-sm text-alpine-700">Entreprise</span>
                </label>
              </div>
            </div>

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
          </div>
        </div>

        {/* Coordonnées */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Coordonnées</h2>
          </div>
          <div className="card-body space-y-4">
            <div>
              <label className="label">Adresse</label>
              <input className="input mb-2" placeholder="Rue et numéro" {...register('address')} />
              <div className="grid grid-cols-3 gap-3">
                <input className="input" placeholder="NPA" {...register('postal_code')} />
                <input className="input col-span-2" placeholder="Localité" {...register('city')} />
              </div>
              <p className="text-xs text-alpine-400 mt-1.5">
                Rue, NPA et localité sont requis pour inclure le débiteur dans le QR code de paiement SPC 0200.
              </p>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="label">E-mail</label>
                <input type="email"
                  className={`input ${errors.email ? 'input-error' : ''}`}
                  {...register('email')} />
                {errors.email && <p className="error-msg">{errors.email.message}</p>}
              </div>
              <div>
                <label className="label">Téléphone</label>
                <input type="tel" className="input" {...register('phone')} />
              </div>
            </div>
          </div>
        </div>

        {/* Paiement */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Paiement</h2>
          </div>
          <div className="card-body">
            <div className="grid grid-cols-3 gap-4">
              <div>
                <label className="label">Délai (jours)</label>
                <input type="number" min="0" max="365" className="input"
                  {...register('payment_term_days')} />
              </div>
              <div className="col-span-2">
                <label className="label">IBAN</label>
                <input className="input font-mono" placeholder="CH…" {...register('iban')} />
              </div>
            </div>
          </div>
        </div>

        {/* Notes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">Notes</h2>
          </div>
          <div className="card-body">
            <textarea rows={3} className="input resize-none w-full" {...register('notes')} />
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 pb-6">
          <button type="button" onClick={() => navigate('/contacts')} className="btn-secondary">
            Retour
          </button>
          <button
            type="submit"
            className="btn-primary"
            disabled={save.isPending || !isDirty}
          >
            <Save size={15} />
            {save.isPending ? 'Enregistrement…' : 'Enregistrer'}
          </button>
        </div>
      </form>
    </div>
  )
}
