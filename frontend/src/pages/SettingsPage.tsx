// LedgerAlps — Paramètres de la société

import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Save, Building2, CreditCard, FileText, Shield,
  Upload, Trash2, ImageOff, Loader2, AlertTriangle,
} from 'lucide-react'
import { settingsApi } from '@/api/client'
import { PageHeader, ErrorBanner } from '@/components/ui'

const schema = z.object({
  company_name:          z.string().min(1, 'Requis'),
  legal_form:            z.string().default(''),
  che_number:            z.string().default(''),
  vat_number:            z.string().default(''),
  address_street:        z.string().default(''),
  address_postal_code:   z.string().default(''),
  address_city:          z.string().default(''),
  address_country:       z.string().length(2).default('CH'),
  iban:                  z.string().default(''),
  fiscal_year_start_month: z.coerce.number().int().min(1).max(12).default(1),
  currency:              z.string().length(3).default('CHF'),
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
  const qc = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Load existing settings
  const { data: company, isLoading } = useQuery({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
  })

  const { register, handleSubmit, reset, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      company_name: '',
      legal_form: '',
      che_number: '',
      vat_number: '',
      address_street: '',
      address_postal_code: '',
      address_city: '',
      address_country: 'CH',
      iban: '',
      fiscal_year_start_month: 1,
      currency: 'CHF',
    },
  })

  // Pre-fill form when settings load
  useEffect(() => {
    if (company) {
      reset({
        company_name:          company.company_name           ?? '',
        legal_form:            company.legal_form             ?? '',
        che_number:            company.che_number             ?? '',
        vat_number:            company.vat_number             ?? '',
        address_street:        company.address_street         ?? '',
        address_postal_code:   company.address_postal_code   ?? '',
        address_city:          company.address_city           ?? '',
        address_country:       company.address_country        ?? 'CH',
        iban:                  company.iban                   ?? '',
        fiscal_year_start_month: company.fiscal_year_start_month ?? 1,
        currency:              company.currency               ?? 'CHF',
      })
    }
  }, [company, reset])

  const save = useMutation({
    mutationFn: (data: FormData) => settingsApi.putCompany(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['company-settings'] })
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    },
  })

  const uploadLogo = useMutation({
    mutationFn: (file: File) => settingsApi.uploadLogo(file),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['company-settings'] }),
  })

  const deleteLogo = useMutation({
    mutationFn: () => settingsApi.deleteLogo(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['company-settings'] }),
  })

  const handleLogoFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    uploadLogo.mutate(file)
    // Reset so same file can be re-selected
    e.target.value = ''
  }

  if (isLoading) return null

  return (
    <div>
      <PageHeader
        title="Paramètres"
        subtitle="Configuration de votre société"
        actions={
          <button
            form="settings-form"
            type="submit"
            className="btn-primary flex items-center gap-1.5"
            disabled={save.isPending}
          >
            {save.isPending ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
            {saved ? 'Sauvegardé ✓' : 'Enregistrer'}
          </button>
        }
      />

      {save.isError && (
        <ErrorBanner message={(save.error as any)?.response?.data?.error ?? 'Erreur lors de la sauvegarde.'} />
      )}

      {/* IBAN missing warning — shown until an IBAN is saved */}
      {company && !company.iban && (
        <div className="mb-4 flex items-start gap-2.5 rounded-lg border border-warning-200 bg-warning-50 px-4 py-3 text-sm text-warning-800">
          <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-warning-500" />
          <span>
            Aucun IBAN configuré. Sans IBAN, les factures PDF ne contiendront pas de QR code de paiement suisse.
            Configurez-le dans l'onglet <strong>Banque</strong>.
          </span>
        </div>
      )}

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
        <form id="settings-form" onSubmit={handleSubmit(d => save.mutate(d))} className="flex-1 space-y-5">

          {/* ─── Identité ─────────────────────────────────────────────── */}
          {tab === 'identity' && (
            <>
              {/* Logo */}
              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">Logo de la société</h2>
                </div>
                <div className="card-body">
                  <div className="flex items-center gap-5">
                    {/* Preview */}
                    <div className="w-24 h-20 border border-alpine-200 rounded-lg bg-alpine-50
                                    flex items-center justify-center overflow-hidden flex-shrink-0">
                      {company?.logo_data ? (
                        <img
                          src={company.logo_data}
                          alt="Logo société"
                          className="w-full h-full object-contain p-1"
                        />
                      ) : (
                        <ImageOff size={28} className="text-alpine-300" />
                      )}
                    </div>

                    {/* Actions */}
                    <div className="space-y-2">
                      <p className="text-xs text-alpine-500">
                        Format PNG ou JPEG, max 2 Mo. Affiché dans la barre de navigation et sur les factures PDF.
                      </p>
                      <div className="flex items-center gap-2">
                        <button
                          type="button"
                          onClick={() => fileInputRef.current?.click()}
                          disabled={uploadLogo.isPending}
                          className="btn-secondary btn-sm flex items-center gap-1.5"
                        >
                          {uploadLogo.isPending
                            ? <Loader2 size={13} className="animate-spin" />
                            : <Upload size={13} />
                          }
                          {company?.logo_data ? 'Remplacer' : 'Télécharger'}
                        </button>
                        {company?.logo_data && (
                          <button
                            type="button"
                            onClick={() => deleteLogo.mutate()}
                            disabled={deleteLogo.isPending}
                            className="btn-ghost btn-sm text-danger-600 flex items-center gap-1.5"
                          >
                            {deleteLogo.isPending
                              ? <Loader2 size={13} className="animate-spin" />
                              : <Trash2 size={13} />
                            }
                            Supprimer
                          </button>
                        )}
                      </div>
                      {uploadLogo.isError && (
                        <p className="text-xs text-danger-600">
                          {(uploadLogo.error as any)?.response?.data?.error ?? 'Erreur lors du téléchargement.'}
                        </p>
                      )}
                    </div>

                    {/* Hidden file input */}
                    <input
                      ref={fileInputRef}
                      type="file"
                      accept="image/png,image/jpeg"
                      className="hidden"
                      onChange={handleLogoFile}
                    />
                  </div>
                </div>
              </div>

              {/* Identity fields */}
              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">Identité de la société</h2>
                </div>
                <div className="card-body grid grid-cols-2 gap-4">
                  <div className="col-span-2">
                    <label className="label">Nom commercial *</label>
                    <input
                      className={`input ${errors.company_name ? 'input-error' : ''}`}
                      placeholder="Acme SA"
                      {...register('company_name')}
                    />
                    {errors.company_name && (
                      <p className="text-xs text-danger-600 mt-1">{errors.company_name.message}</p>
                    )}
                  </div>
                  <div>
                    <label className="label">Forme juridique</label>
                    <input className="input" placeholder="SA, Sàrl, raison individuelle…" {...register('legal_form')} />
                  </div>
                  <div>
                    <label className="label">N° IDE suisse (CHE-…)</label>
                    <input className="input font-mono" placeholder="CHE-123.456.789" {...register('che_number')} />
                  </div>
                  <div className="col-span-2">
                    <label className="label">Adresse</label>
                    <input className="input mb-2" placeholder="Rue et numéro" {...register('address_street')} />
                    <div className="grid grid-cols-4 gap-3">
                      <input className="input" placeholder="NPA"      {...register('address_postal_code')} />
                      <input className="input col-span-2" placeholder="Localité" {...register('address_city')} />
                      <input className="input uppercase" placeholder="CH" maxLength={2} {...register('address_country')} />
                    </div>
                  </div>
                </div>
              </div>
            </>
          )}

          {/* ─── Banque ───────────────────────────────────────────────── */}
          {tab === 'banking' && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-sm font-semibold text-alpine-800">Coordonnées bancaires et TVA</h2>
              </div>
              <div className="card-body grid grid-cols-2 gap-4">
                <div className="col-span-2">
                  <label className="label">IBAN <span className="text-warning-600 font-normal">(requis pour le QR code de paiement)</span></label>
                  <input className="input font-mono" placeholder="CH56 0483 5012 3456 7800 9" {...register('iban')} />
                  <p className="text-xs text-alpine-400 mt-1">
                    Requis pour le QR code de paiement SPC 0200 sur les factures PDF. Format : CH + 19 chiffres.
                  </p>
                </div>
                <div className="col-span-2">
                  <label className="label">N° TVA AFC</label>
                  <input className="input font-mono" placeholder="CHE-123.456.789 MWST" {...register('vat_number')} />
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
                  <label className="label">Devise principale</label>
                  <select className="select" {...register('currency')}>
                    <option value="CHF">CHF — Franc suisse</option>
                    <option value="EUR">EUR — Euro</option>
                  </select>
                </div>
                <div>
                  <label className="label">Début d'exercice (mois)</label>
                  <select className="select" {...register('fiscal_year_start_month')}>
                    {[
                      [1,'Janvier'],[2,'Février'],[3,'Mars'],[4,'Avril'],
                      [5,'Mai'],[6,'Juin'],[7,'Juillet'],[8,'Août'],
                      [9,'Septembre'],[10,'Octobre'],[11,'Novembre'],[12,'Décembre'],
                    ].map(([v, l]) => (
                      <option key={v} value={v}>{l}</option>
                    ))}
                  </select>
                </div>
              </div>
            </div>
          )}

          {/* ─── Légal ────────────────────────────────────────────────── */}
          {tab === 'legal' && (
            <div className="space-y-4">
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
