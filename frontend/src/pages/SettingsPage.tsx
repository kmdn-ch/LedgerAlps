// LedgerAlps — Paramètres de la société

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useState } from 'react'
import { Save, Building2, CreditCard, FileText, Shield } from 'lucide-react'
import { PageHeader } from '@/components/ui'

const schema = z.object({
  // Identité
  name:         z.string().min(1, 'Requis'),
  legal_name:   z.string().optional(),
  uid_number:   z.string().optional(),
  vat_number:   z.string().optional(),
  // Adresse
  address_line1: z.string().optional(),
  postal_code:   z.string().optional(),
  city:          z.string().optional(),
  country:       z.string().length(2).default('CH'),
  // Contact
  email:  z.string().email().optional().or(z.literal('')),
  phone:  z.string().optional(),
  website: z.string().url().optional().or(z.literal('')),
  // Comptabilité
  currency:           z.string().length(3).default('CHF'),
  fiscal_year_start:  z.string().default('01-01'),
  vat_method:         z.enum(['effective', 'tdfn']).default('effective'),
  default_vat_rate:   z.coerce.number().default(8.1),
  payment_term_days:  z.coerce.number().int().min(0).default(30),
  // Bancaire
  iban:     z.string().optional(),
  qr_iban:  z.string().optional(),
  bic:      z.string().optional(),
  bank_name: z.string().optional(),
  // Facturation
  invoice_prefix:  z.string().default('FA'),
  invoice_footer:  z.string().optional(),
  invoice_terms:   z.string().optional(),
})

type FormData = z.infer<typeof schema>

const TABS = [
  { key: 'identity',  label: 'Identité',    icon: Building2  },
  { key: 'banking',   label: 'Banque',       icon: CreditCard },
  { key: 'invoicing', label: 'Facturation',  icon: FileText   },
  { key: 'legal',     label: 'Légal / CO',   icon: Shield     },
]

export function SettingsPage() {
  const [tab,   setTab]   = useState('identity')
  const [saved, setSaved] = useState(false)

  const { register, handleSubmit, formState: { errors, isDirty } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      country:          'CH',
      currency:         'CHF',
      vat_method:       'effective',
      default_vat_rate: 8.1,
      payment_term_days: 30,
      invoice_prefix:   'FA',
      fiscal_year_start: '01-01',
    },
  })

  const onSubmit = async (data: FormData) => {
    // TODO Phase 5 : persister via API /api/v1/settings
    console.log('Paramètres sauvegardés :', data)
    setSaved(true)
    setTimeout(() => setSaved(false), 3000)
  }

  return (
    <div>
      <PageHeader
        title="Paramètres"
        subtitle="Configuration de votre société"
        actions={
          <button
            form="settings-form"
            type="submit"
            className={`btn-primary ${!isDirty ? 'opacity-50' : ''}`}
            disabled={!isDirty}
          >
            <Save size={15} />
            {saved ? 'Sauvegardé ✓' : 'Enregistrer'}
          </button>
        }
      />

      <div className="flex gap-6">
        {/* Nav latérale */}
        <nav className="w-44 flex-shrink-0">
          <div className="space-y-0.5">
            {TABS.map(t => (
              <button
                key={t.key}
                onClick={() => setTab(t.key)}
                className={`w-full flex items-center gap-2.5 px-3 py-2.5 rounded-lg
                            text-sm text-left transition-all ${
                  tab === t.key
                    ? 'bg-alpine-800 text-white font-medium'
                    : 'text-alpine-600 hover:bg-alpine-100 hover:text-alpine-900'
                }`}
              >
                <t.icon size={15} className="flex-shrink-0" />
                {t.label}
              </button>
            ))}
          </div>
        </nav>

        {/* Formulaire */}
        <form id="settings-form" onSubmit={handleSubmit(onSubmit)} className="flex-1">

          {/* ─── Identité ─────────────────────────────────────────────── */}
          {tab === 'identity' && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-sm font-semibold text-alpine-800">Identité de la société</h2>
              </div>
              <div className="card-body grid grid-cols-2 gap-4">
                <div className="col-span-2">
                  <label className="label">Nom commercial *</label>
                  <input className={`input ${errors.name ? 'input-error' : ''}`}
                    placeholder="Acme SA" {...register('name')} />
                </div>
                <div>
                  <label className="label">Raison sociale légale</label>
                  <input className="input" {...register('legal_name')} />
                </div>
                <div>
                  <label className="label">N° IDE suisse (CHE-…)</label>
                  <input className="input font-mono" placeholder="CHE-123.456.789"
                    {...register('uid_number')} />
                </div>
                <div className="col-span-2">
                  <label className="label">Adresse</label>
                  <input className="input mb-2" placeholder="Rue et numéro"
                    {...register('address_line1')} />
                  <div className="grid grid-cols-4 gap-3">
                    <input className="input" placeholder="NPA" {...register('postal_code')} />
                    <input className="input col-span-2" placeholder="Localité" {...register('city')} />
                    <input className="input uppercase" placeholder="CH" maxLength={2}
                      {...register('country')} />
                  </div>
                </div>
                <div>
                  <label className="label">E-mail</label>
                  <input type="email" className="input" {...register('email')} />
                </div>
                <div>
                  <label className="label">Téléphone</label>
                  <input type="tel" className="input" {...register('phone')} />
                </div>
                <div className="col-span-2">
                  <label className="label">Site web</label>
                  <input type="url" className="input" placeholder="https://…" {...register('website')} />
                </div>
              </div>
            </div>
          )}

          {/* ─── Banque ───────────────────────────────────────────────── */}
          {tab === 'banking' && (
            <div className="space-y-4">
              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">Coordonnées bancaires</h2>
                </div>
                <div className="card-body grid grid-cols-2 gap-4">
                  <div className="col-span-2">
                    <label className="label">IBAN (compte courant)</label>
                    <input className="input font-mono" placeholder="CH56 0483 5012 3456 7800 9"
                      {...register('iban')} />
                    <p className="text-xs text-alpine-400 mt-1">
                      Utilisé pour les virements reçus et les exports ISO 20022.
                    </p>
                  </div>
                  <div className="col-span-2">
                    <label className="label">QR-IBAN</label>
                    <input className="input font-mono" placeholder="CH44 3100 0000 0012 3456 7"
                      {...register('qr_iban')} />
                    <p className="text-xs text-alpine-400 mt-1">
                      IID entre 30000–31999. Obligatoire pour les QR-factures avec référence QRR.
                    </p>
                  </div>
                  <div>
                    <label className="label">BIC / SWIFT</label>
                    <input className="input font-mono" placeholder="POFICHBEXXX"
                      {...register('bic')} />
                  </div>
                  <div>
                    <label className="label">Nom de la banque</label>
                    <input className="input" placeholder="PostFinance AG"
                      {...register('bank_name')} />
                  </div>
                </div>
              </div>

              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">Paramètres TVA</h2>
                </div>
                <div className="card-body grid grid-cols-3 gap-4">
                  <div>
                    <label className="label">N° TVA AFC</label>
                    <input className="input font-mono" placeholder="CHE-123.456.789 MWST"
                      {...register('vat_number')} />
                  </div>
                  <div>
                    <label className="label">Méthode TVA</label>
                    <select className="select" {...register('vat_method')}>
                      <option value="effective">Méthode effective</option>
                      <option value="tdfn">TDFN (taux dette fiscale nette)</option>
                    </select>
                  </div>
                  <div>
                    <label className="label">Taux par défaut (%)</label>
                    <select className="select" {...register('default_vat_rate')}>
                      <option value={8.1}>8.1% — Normal</option>
                      <option value={2.6}>2.6% — Réduit</option>
                      <option value={3.8}>3.8% — Hébergement</option>
                      <option value={0}>0% — Exonéré</option>
                    </select>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* ─── Facturation ──────────────────────────────────────────── */}
          {tab === 'invoicing' && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-sm font-semibold text-alpine-800">Paramètres de facturation</h2>
              </div>
              <div className="card-body grid grid-cols-2 gap-4">
                <div>
                  <label className="label">Préfixe factures</label>
                  <input className="input font-mono" placeholder="FA"
                    {...register('invoice_prefix')} />
                  <p className="text-xs text-alpine-400 mt-1">
                    Ex: FA → FA2025-0001
                  </p>
                </div>
                <div>
                  <label className="label">Délai de paiement (jours)</label>
                  <input type="number" min="0" max="365" className="input"
                    {...register('payment_term_days')} />
                </div>
                <div className="col-span-2">
                  <label className="label">Conditions de paiement (texte par défaut)</label>
                  <textarea rows={3} className="input resize-none"
                    placeholder="Paiement à 30 jours. En cas de retard, des intérêts de 5% l'an seront facturés."
                    {...register('invoice_terms')} />
                </div>
                <div className="col-span-2">
                  <label className="label">Pied de page des factures</label>
                  <textarea rows={2} className="input resize-none"
                    placeholder="Merci de votre confiance."
                    {...register('invoice_footer')} />
                </div>
              </div>
            </div>
          )}

          {/* ─── Légal ────────────────────────────────────────────────── */}
          {tab === 'legal' && (
            <div className="space-y-4">
              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">
                    Exercice comptable — CO art. 957
                  </h2>
                </div>
                <div className="card-body grid grid-cols-2 gap-4">
                  <div>
                    <label className="label">Début de l'exercice (JJ-MM)</label>
                    <input className="input font-mono" placeholder="01-01"
                      {...register('fiscal_year_start')} />
                  </div>
                  <div>
                    <label className="label">Devise principale</label>
                    <select className="select" {...register('currency')}>
                      <option value="CHF">CHF — Franc suisse</option>
                      <option value="EUR">EUR — Euro</option>
                    </select>
                  </div>
                </div>
              </div>

              <div className="card border-warning-200 bg-warning-50/30">
                <div className="card-body">
                  <h3 className="text-sm font-semibold text-warning-700 mb-2">
                    Obligations légales
                  </h3>
                  <ul className="text-sm text-alpine-700 space-y-1.5">
                    <li className="flex items-start gap-2">
                      <span className="text-success-600 mt-0.5">✓</span>
                      <span><strong>CO art. 958f</strong> — Conservation des documents : 10 ans.</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-success-600 mt-0.5">✓</span>
                      <span><strong>nLPD</strong> — Données stockées localement, chiffrement pgcrypto.</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-success-600 mt-0.5">✓</span>
                      <span><strong>TVA CH</strong> — Déclaration AFC : trimestrielle ou semestrielle.</span>
                    </li>
                    <li className="flex items-start gap-2">
                      <span className="text-warning-600 mt-0.5">⚠</span>
                      <span>Ce logiciel ne remplace pas un expert fiduciaire agréé.</span>
                    </li>
                  </ul>
                </div>
              </div>
            </div>
          )}
        </form>
      </div>
    </div>
  )
}
