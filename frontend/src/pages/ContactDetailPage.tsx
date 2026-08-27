// LedgerAlps — Détail et édition d'un contact

import { useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ArrowLeft, Save, Building2, User } from 'lucide-react'
import { contactsApi } from '@/api/client'
import { ContactDocuments } from '@/components/contacts/ContactDocuments'
import { PageHeader, LoadingSpinner, ErrorBanner } from '@/components/ui'
import type { Contact } from '@/types'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { useT, useTv } from '@/i18n/useT'
import { refusalMessage } from '@/utils/refusal'

// ─── Schema ───────────────────────────────────────────────────────────────────

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
  qr_iban:           opt(z.string()),
  notes:             opt(z.string()),
}).superRefine((data, ctx) => {
  // Symétrique de NewContactModal : la règle d'adresse QR doit tenir à la
  // MODIFICATION comme à la création. Elle ne l'était pas — cet écran laissait
  // vider l'adresse d'un client complet sans le moindre avertissement, et le
  // serveur ne la revérifiait pas non plus.
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

// ─── Page ─────────────────────────────────────────────────────────────────────

export function ContactDetailPage() {
  const t = useT()
  const tv = useTv()
  const { contactId } = useParams<{ contactId: string }>()
  const peutEcrire    = useCanWrite()
  const navigate      = useNavigate()
  const qc            = useQueryClient()

  const { data: contact, isLoading, error } = useQuery<Contact>({
    queryKey: ['contact', contactId],
    queryFn:  () => contactsApi.get(contactId!).then(r => r.data),
    enabled:  !!contactId,
  })

  const {
    register, handleSubmit, reset, watch,
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
      qr_iban:           contact.qr_iban ?? '',
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

  // L'adresse n'est exigée que si le contact peut être débiteur d'une facture
  // QR. On suit le type SÉLECTIONNÉ, pas celui stocké : l'astérisque doit
  // apparaître au moment où l'utilisateur bascule le menu, pas après
  // l'enregistrement.
  const adresseRequise = watch('contact_type') !== 'supplier'

  if (isLoading) return <LoadingSpinner />
  if (error || !contact) return <ErrorBanner message={t('co.introuvable')} />

  return (
    <div className="max-w-3xl mx-auto">
      {/* En-tête */}
      <div className="flex items-center gap-3 mb-6">
        <button onClick={() => navigate('/contacts')} className="btn-ghost btn-sm">
          <ArrowLeft size={15} />
        </button>
        <PageHeader
          title={contact.name}
          subtitle={t(contact.is_company ? 'co.entreprise' : 'co.particulier')}
          /* La désactivation manuelle a été retirée : elle n'apportait rien
             qu'on ne fasse mieux autrement. Un contact qu'on ne veut plus voir
             s'anonymise (nLPD art. 6 al. 4), ce qui l'écarte des listes ET
             efface ses données personnelles — un geste qui dit ce qu'il fait,
             au lieu d'un interrupteur dont l'effet n'était visible nulle part.

             La colonne is_active reste : c'est l'anonymisation qui la pose. */
        />
      </div>

      {save.isError && (
        <ErrorBanner message={
          refusalMessage(save.error, t('co.erreurSauvegarde'))
        } />
      )}
      {save.isSuccess && !isDirty && (
        <div className="mb-4 px-4 py-2.5 rounded-lg bg-success-100 border border-success-100
                        text-sm text-success-700">
          {t('co.enregistre')}
        </div>
      )}

      {!peutEcrire && (
        <div className="mb-4 px-4 py-2.5 rounded-lg bg-alpine-50 border border-alpine-200
                        text-sm text-alpine-600">
          {t(RAISON_LECTURE_SEULE)}
        </div>
      )}

      <form onSubmit={handleSubmit(d => save.mutate(d))} className="space-y-5">
      {/* Un `fieldset` désactivé neutralise NATIVEMENT chaque champ et bouton
          qu'il contient, y compris ceux qu'on y ajoutera plus tard. Désactiver
          champ par champ marche le jour où on l'écrit, puis se périme au
          premier champ ajouté sans y penser — c'est le motif qui a déjà laissé
          passer des fonctions non gardées dans ce produit. */}
      <fieldset disabled={!peutEcrire} className="contents">
        {/* Identité */}
        <div className="card">
          <div className="card-header">
            <div className="flex items-center gap-2">
              {contact.is_company
                ? <Building2 size={15} className="text-alpine-500" />
                : <User      size={15} className="text-alpine-500" />
              }
              <h2 className="text-sm font-semibold text-alpine-800">{t('co.identite')}</h2>
            </div>
          </div>
          <div className="card-body space-y-4">
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
                  <input type="checkbox" {...register('is_company')}
                    className="rounded border-alpine-300 accent-accent-500" />
                  <span className="text-sm text-alpine-700">{t('co.entreprise')}</span>
                </label>
              </div>
            </div>

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
          </div>
        </div>

        {/* Coordonnées */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('co.coordonnees')}</h2>
          </div>
          <div className="card-body space-y-4">
            <div>
              <label className="label">{t('co.adresse')}{adresseRequise && ' *'}</label>
              <input className={`input mb-2 ${errors.address ? 'input-error' : ''}`}
                placeholder={t('co.placeholderRue')} {...register('address')} />
              {errors.address && <p className="error-msg mb-2">{tv(errors.address.message)}</p>}
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <input className={`input ${errors.postal_code ? 'input-error' : ''}`}
                    placeholder={adresseRequise ? `${t('pr.npa')} *` : t('pr.npa')}
                    {...register('postal_code')} />
                  {errors.postal_code && <p className="error-msg">{tv(errors.postal_code.message)}</p>}
                </div>
                <div className="col-span-2">
                  <input className={`input w-full ${errors.city ? 'input-error' : ''}`}
                    placeholder={adresseRequise ? `${t('co.placeholderLocalite')} *` : t('co.placeholderLocalite')}
                    {...register('city')} />
                  {errors.city && <p className="error-msg">{tv(errors.city.message)}</p>}
                </div>
              </div>
              <p className="text-xs text-alpine-400 mt-1.5">
                {t('co.adresseQRAide')}
              </p>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="label">{t('co.email')}</label>
                <input type="email"
                  className={`input ${errors.email ? 'input-error' : ''}`}
                  {...register('email')} />
                {errors.email && <p className="error-msg">{tv(errors.email.message)}</p>}
              </div>
              <div>
                <label className="label">{t('co.telephone')}</label>
                <input type="tel" className="input" {...register('phone')} />
              </div>
            </div>
          </div>
        </div>

        {/* Paiement */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('co.paiement')}</h2>
          </div>
          <div className="card-body">
            <div className="grid grid-cols-3 gap-4">
              <div>
                <label className="label">{t('co.delaiJours')}</label>
                <input type="number" min="0" max="365" className="input"
                  {...register('payment_term_days')} />
              </div>
              <div className="col-span-2">
                <label className="label">{t('paiement.iban')}</label>
                <input className="input font-mono" placeholder="CH…" {...register('iban')} />
              </div>
            </div>
            {/* Le QR-IBAN se range à part parce qu'il ne se substitue pas à
                l'IBAN : une référence QR n'est acceptée qu'avec lui, une
                référence Creditor Reference qu'avec un IBAN ordinaire
                (SIX IG v2.4 §4.2.2). Les confondre fait rejeter le virement.
                Lu sur une facture, il était enregistré ici sans jamais être
                montré — ce qui est invisible ne se corrige pas. */}
            <div className="mt-4">
              <label className="label">{t('paiement.qrIban')}</label>
              <input className="input font-mono" placeholder="CH…"
                {...register('qr_iban')} />
              <p className="text-xs text-alpine-500 mt-1">
                {t('co.qrIbanAide')}
              </p>
            </div>
          </div>
        </div>

        {/* Notes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('co.notes')}</h2>
          </div>
          <div className="card-body">
            <textarea rows={3} className="input resize-none w-full" {...register('notes')} />
          </div>
        </div>

        </fieldset>

        {/* « Retour » vit hors du fieldset : naviguer n'écrit rien, et un
            lecteur doit pouvoir repartir. */}
        <div className="flex justify-end gap-3 pb-6">
          <button type="button" onClick={() => navigate('/contacts')} className="btn-secondary">
            {t('action.retour')}
          </button>
          {peutEcrire && (
            <button
              type="submit"
              className="btn-primary"
              disabled={save.isPending || !isDirty}
            >
              <Save size={15} />
              {save.isPending ? t('etat.enregistrement') : t('action.enregistrer')}
            </button>
          )}
        </div>
      </form>

      {/* Pièces du contact — factures, offres et notes de crédit.
          Le filtre est appliqué côté serveur : filtrer la page affichée
          manquerait les pièces des pages suivantes. */}
      <ContactDocuments contactId={contactId!} contactName={contact?.name ?? ''} />
    </div>
  )
}
