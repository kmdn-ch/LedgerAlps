// LedgerAlps — Modal création de contact

import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { contactsApi } from '@/api/client'
import { ErrorBanner, ConfirmDialog } from '@/components/ui'
import { useBeforeUnload } from '@/hooks/useUnsavedGuard'
import { useT, useTv } from '@/i18n/useT'
import { refusalMessage } from '@/utils/refusal'

// Convert empty strings to undefined so optional fields don't trip validation.
const opt = <T extends z.ZodTypeAny>(s: T) =>
  z.preprocess(v => (v === '' ? undefined : v), s.optional())

const schema = z.object({
  contact_type:      z.enum(['customer', 'supplier', 'both']),
  is_company:        z.boolean().default(false),
  name:              z.string().min(1, 'val.nomRequis'),
  legal_name:        opt(z.string()),
  address:           opt(z.string()),
  postal_code:       opt(z.string()),
  city:              opt(z.string()),
  country:           z.string().min(2).max(2).default('CH'),
  uid_number:        opt(z.string()),
  vat_number:        opt(z.string()),
  email:             opt(z.string().email('val.emailInvalide')),
  phone:             opt(z.string()),
  payment_term_days: z.coerce.number().int().min(0).max(365).default(30),
  iban:              opt(z.string()),
  notes:             opt(z.string()),
}).superRefine((data, ctx) => {
  // Un client (ou un contact « les deux ») devient le débiteur d'une facture
  // QR : sans adresse structurée complète, le bulletin de versement suisse
  // ne peut pas s'imprimer. Un fournisseur pur n'est jamais débiteur d'une
  // facture émise par LedgerAlps — sa fiche reste allégée.
  if (data.contact_type === 'supplier') return
  if (!data.address) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['address'], message: 'val.adresseRequise' })
  }
  if (!data.postal_code) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['postal_code'], message: 'val.npaRequis' })
  }
  if (!data.city) {
    ctx.addIssue({ code: z.ZodIssueCode.custom, path: ['city'], message: 'val.villeRequise' })
  }
})

type FormData = z.infer<typeof schema>

interface Props { onClose: () => void }

export function NewContactModal({ onClose }: Props) {
  const t = useT()
  const tv = useTv()
  const qc = useQueryClient()
  const {
    register, handleSubmit, watch,
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

  const create = useMutation({
    mutationFn: (data: FormData) => contactsApi.create(data),
    onSuccess:  () => {
      qc.invalidateQueries({ queryKey: ['contacts'] })
      onClose()
    },
  })

  // L'adresse n'est exigée que pour un client : un fournisseur pur n'est
  // jamais débiteur d'une facture QR émise par LedgerAlps.
  const watchedType = watch('contact_type')
  const addressRequise = watchedType !== 'supplier'

  // Le voile ferme au moindre clic. C'est commode pour consulter, destructeur
  // pour une saisie : LedgerAlps n'enregistre aucun brouillon, et une fiche à
  // demi remplie disparaît entièrement. La confirmation n'apparaît que si
  // quelque chose a été saisi — la demander sur un formulaire vide apprendrait
  // à cliquer « Quitter » sans lire.
  const [confirmingClose, setConfirmingClose] = useState(false)
  const requestClose = () => (isDirty ? setConfirmingClose(true) : onClose())
  useBeforeUnload(isDirty)

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      {/* Overlay */}
      <div
        className="absolute inset-0 bg-alpine-900/50 backdrop-blur-sm"
        onClick={requestClose}
      />

      <ConfirmDialog
        open={confirmingClose}
        tone="danger"
        title={t('co.abandonnerTitre')}
        consequences={[t('co.abandonnerCons')]}
        reassurance={t('co.abandonnerRassur')}
        confirmLabel={t('co.abandonnerConfirmer')}
        onConfirm={() => { setConfirmingClose(false); onClose() }}
        onCancel={() => setConfirmingClose(false)}
      />

      {/* Modal */}
      <div className="relative bg-white rounded-2xl shadow-modal w-full max-w-2xl
                      max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between px-6 py-4 border-b border-alpine-100">
          <h2 className="font-display font-700 text-lg text-alpine-900">{t('ct.nouveau')}</h2>
          <button onClick={requestClose} className="btn-ghost p-1.5">
            <X size={18} />
          </button>
        </div>

        <form onSubmit={handleSubmit(d => create.mutate(d))} className="px-6 py-5 space-y-5">
          {create.error && (
            <ErrorBanner message={
              refusalMessage(create.error, t('co.erreurCreation'))
                ?? t('co.erreurCreation')
            } />
          )}

          {/* Type et nature */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">{t('co.type')}</label>
              <select className="select" {...register('contact_type')}>
                <option value="customer">{t('co.client')}</option>
                <option value="supplier">{t('co.fournisseur')}</option>
                <option value="both">{t('co.lesDeux')}</option>
              </select>
            </div>
            <div className="flex items-end pb-2">
              <label className="flex items-center gap-2 cursor-pointer">
                <input type="checkbox" {...register('is_company')} className="rounded border-alpine-300 accent-accent-500" />
                <span className="text-sm text-alpine-700">{t('co.entreprise')}</span>
              </label>
            </div>
          </div>

          {/* Nom */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">{t('co.nomRaisonSociale')}</label>
              <input className={`input ${errors.name ? 'input-error' : ''}`}
                {...register('name')} />
              {errors.name && <p className="error-msg">{tv(errors.name.message)}</p>}
            </div>
            <div>
              <label className="label">{t('co.raisonSocialeLegale')}</label>
              <input className="input" {...register('legal_name')} />
            </div>
          </div>

          {/* Adresse */}
          <div>
            <label className="label">{t('co.adresse')}{addressRequise && ' *'}</label>
            <input className={`input mb-2 ${errors.address ? 'input-error' : ''}`}
              placeholder={t('co.placeholderRue')}
              {...register('address')} />
            {errors.address && <p className="error-msg mb-2">{tv(errors.address.message)}</p>}
            <div className="grid grid-cols-3 gap-3">
              <div>
                <input className={`input ${errors.postal_code ? 'input-error' : ''}`}
                  placeholder={addressRequise ? `${t('pr.npa')} *` : t('pr.npa')}
                  {...register('postal_code')} />
                {errors.postal_code && <p className="error-msg">{tv(errors.postal_code.message)}</p>}
              </div>
              <div className="col-span-2">
                <input className={`input w-full ${errors.city ? 'input-error' : ''}`}
                  placeholder={addressRequise ? `${t('co.placeholderLocalite')} *` : t('co.placeholderLocalite')}
                  {...register('city')} />
                {errors.city && <p className="error-msg">{tv(errors.city.message)}</p>}
              </div>
            </div>
          </div>

          {/* Pays + Numéros légaux */}
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="label">{t('co.pays')}</label>
              <input className="input uppercase" maxLength={2} {...register('country')} />
            </div>
            <div>
              <label className="label">{t('co.ide')}</label>
              <input className="input font-mono" placeholder="CHE-123.456.789"
                {...register('uid_number')} />
            </div>
            <div>
              <label className="label">{t('co.numeroTVA')}</label>
              <input className="input font-mono" {...register('vat_number')} />
            </div>
          </div>

          {/* Contact */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="label">{t('co.email')}</label>
              <input type="email" className={`input ${errors.email ? 'input-error' : ''}`}
                {...register('email')} />
            </div>
            <div>
              <label className="label">{t('co.telephone')}</label>
              <input type="tel" className="input" {...register('phone')} />
            </div>
          </div>

          {/* Paiement */}
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="label">{t('co.delaiPaiementJours')}</label>
              <input type="number" min="0" max="365" className="input"
                {...register('payment_term_days')} />
            </div>
            <div className="col-span-2">
              <label className="label">{t('paiement.iban')}</label>
              <input className="input font-mono" placeholder="CH…"
                {...register('iban')} />
            </div>
          </div>

          {/* Notes */}
          <div>
            <label className="label">{t('co.notes')}</label>
            <textarea rows={2} className="input resize-none" {...register('notes')} />
          </div>

          {/* Actions */}
          <div className="flex justify-end gap-3 pt-2 border-t border-alpine-100">
            <button type="button" onClick={requestClose} className="btn-secondary">
              {t('action.annuler')}
            </button>
            <button type="submit" className="btn-primary" disabled={create.isPending}>
              {create.isPending ? t('etat.enregistrement') : t('co.creerContact')}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
