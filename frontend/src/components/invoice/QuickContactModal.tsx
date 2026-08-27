// LedgerAlps — Création rapide d'un contact depuis une facture ou une offre
//
// Cette fenêtre existait en DEUX exemplaires quasi identiques, l'un dans
// `NewInvoicePage`, l'autre dans `EditInvoicePage` — et une troisième
// implémentation, différente et légitime, vit dans
// `components/contact/NewContactModal.tsx` (le formulaire complet de l'écran
// Contacts). Les deux premières ont dérivé sans que rien ne le signale : celle
// de `NewInvoicePage` envoyait un champ `currency` que la table `contacts` ne
// possède pas, et que Gin jetait donc en silence.
//
// Une seule reste. La troisième est conservée telle quelle : elle sert un
// besoin réellement différent (gérer un contact de bout en bout, avec son type,
// son IBAN, ses notes), et les fusionner ajouterait de la complexité
// conditionnelle pour peu de gain.
//
// Le contact créé ici est TOUJOURS un client (`contact_type: 'customer'`) :
// c'est le débiteur de la facture en cours de saisie. Son adresse complète est
// donc obligatoire, pas conseillée — sans elle, le bulletin de versement ne
// s'imprime pas (SPC 0200 §4.2.2), et le serveur refuse la création depuis la
// v1.5.9.

import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { contactsApi } from '@/api/client'
import type { Contact } from '@/types'
import { refusalMessage } from '@/utils/refusal'
import { useT } from '@/i18n/useT'

const EMPTY_CONTACT = {
  name: '', is_company: false, email: '', phone: '',
  address: '', postal_code: '', city: '', country: 'CH',
}

interface Props {
  onClose: () => void
  onCreated: (contact: Contact) => void
}

export function QuickContactModal({ onClose, onCreated }: Props) {
  const t = useT()
  const qc = useQueryClient()
  const [fields, setFields] = useState(EMPTY_CONTACT)
  const [err, setErr] = useState<string | null>(null)
  const [tried, setTried] = useState(false)

  const create = useMutation({
    mutationFn: () => contactsApi.create({
      contact_type:      'customer',
      is_company:        fields.is_company,
      name:              fields.name.trim(),
      email:             fields.email || undefined,
      phone:             fields.phone || undefined,
      address:           fields.address.trim(),
      postal_code:       fields.postal_code.trim(),
      city:              fields.city.trim(),
      country:           fields.country || 'CH',
      payment_term_days: 30,
    }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['contacts'] })
      onCreated(res.data as Contact)
    },
    onError: (e) => setErr(refusalMessage(e, t('nf.erreurContact'))),
  })

  const set = (key: keyof typeof EMPTY_CONTACT, value: string | boolean) =>
    setFields(f => ({ ...f, [key]: value }))

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
        {/* En-tête */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-alpine-200">
          <h3 className="text-sm font-semibold text-alpine-900">{t('ct.nouveau')}</h3>
          <button type="button" onClick={onClose} className="btn-ghost btn-sm p-1 text-alpine-400">
            <X size={16} />
          </button>
        </div>

        {/* Formulaire */}
        <div className="px-5 py-4 space-y-3">
          {err && <p className="text-xs text-danger-700 bg-danger-100 rounded px-3 py-2">{err}</p>}

          <div className="flex items-center gap-2">
            <input
              id="is_company"
              type="checkbox"
              checked={fields.is_company}
              onChange={e => set('is_company', e.target.checked)}
              className="rounded border-alpine-300 text-alpine-700"
            />
            <label htmlFor="is_company" className="text-sm text-alpine-700">{t('nf.entreprise')}</label>
          </div>

          <div>
            <label className="label">{t('nf.nom')}</label>
            <input
              className={`input ${tried && !fields.name.trim() ? 'input-error' : ''}`}
              placeholder={fields.is_company ? t('nf.placeholderRaisonSociale') : t('nf.placeholderPrenomNom')}
              value={fields.name}
              onChange={e => set('name', e.target.value)}
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="label">{t('nf.email')}</label>
              <input type="email" className="input" value={fields.email}
                onChange={e => set('email', e.target.value)} />
            </div>
            <div>
              <label className="label">{t('nf.telephone')}</label>
              <input type="tel" className="input" value={fields.phone}
                onChange={e => set('phone', e.target.value)} />
            </div>
          </div>

          {/* Adresse — obligatoire : ce contact devient le débiteur de la
              facture QR, et sans elle le bulletin de versement ne s'imprime
              pas (SPC 0200 §4.2.2). */}
          <div>
            <label className="label">{t('co.adresse')} *</label>
            <input
              className={`input ${tried && !fields.address.trim() ? 'input-error' : ''}`}
              placeholder={t('co.placeholderRue')}
              value={fields.address}
              onChange={e => set('address', e.target.value)}
            />
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="label">{t('pr.npa')} *</label>
              <input className={`input ${tried && !fields.postal_code.trim() ? 'input-error' : ''}`}
                value={fields.postal_code}
                onChange={e => set('postal_code', e.target.value)} />
            </div>
            <div className="col-span-2">
              <label className="label">{t('nf.ville')} *</label>
              <input className={`input ${tried && !fields.city.trim() ? 'input-error' : ''}`}
                value={fields.city}
                onChange={e => set('city', e.target.value)} />
            </div>
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

        {/* Actions */}
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-alpine-200">
          <button type="button" onClick={onClose} className="btn-secondary btn-sm">
            {t('action.annuler')}
          </button>
          <button
            type="button"
            disabled={create.isPending}
            onClick={submit}
            className="btn-primary btn-sm"
          >
            {create.isPending ? t('fd.creationEnCours') : t('nf.creerContact')}
          </button>
        </div>
      </div>
    </div>
  )
}
