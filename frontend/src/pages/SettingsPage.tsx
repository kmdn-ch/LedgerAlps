// LedgerAlps — Paramètres de la société

import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { useForm, type UseFormRegister } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Save, Building2, CreditCard, FileText,
  Upload, Trash2, ImageOff, Loader2, AlertTriangle, Database, Wrench, UserCog,
} from 'lucide-react'
import { settingsApi } from '@/api/client'
import { PageHeader, ErrorBanner } from '@/components/ui'
import { estQRIBAN } from '@/utils'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { useT, useTv } from '@/i18n/useT'
import type { Cle } from '@/i18n'

const schema = z.object({
  company_name:          z.string().min(1, 'val.requis'),
  legal_form:            z.string().default(''),
  che_number:            z.string().default(''),
  vat_number:            z.string().default(''),
  // '' = non déclaré. Ce n'est pas un défaut technique mais une réponse
  // manquante : la distinguer de « non assujetti » est tout l'intérêt du champ.
  vat_status:            z.enum(['', 'liable', 'exempt']).default(''),
  phone:                 z.string().default(''),
  bank_name:             z.string().default(''),
  bank_address:          z.string().default(''),
  bank_bic:              z.string().default(''),
  auto_post_invoices:    z.boolean().default(false),
  email:                 z.string().default(''),
  address_street:        z.string().default(''),
  address_postal_code:   z.string().default(''),
  address_city:          z.string().default(''),
  address_country:       z.string().length(2).default('CH'),
  iban:                  z.string().default(''),
  fiscal_year_start_month: z.coerce.number().int().min(1).max(12).default(1),
  currency:              z.string().length(3).default('CHF'),
})

type FormData = z.infer<typeof schema>

// Les onglets réservés à l'administration ne sont pas seulement masqués : ils
// ne sont pas rendus, leur contenu n'est pas monté, et leurs routes répondent
// 403 côté serveur — le rôle y étant relu dans la base à chaque requête.
//
// Masquer sans interdire ne protège de rien : l'adresse reste tapable et
// l'appel réseau reste faisable. Interdire sans masquer use la confiance dans
// l'interface. Il faut les deux, et c'est le serveur qui décide.
const ADMIN_ONLY = new Set(['backups', 'maintenance'])

const TABS = [
  { key: 'identity',  cle: 'pr.ongletIdentite'    as Cle, icon: Building2  },
  { key: 'banking',   cle: 'pr.ongletBanque'      as Cle, icon: CreditCard },
  { key: 'invoicing', cle: 'pr.ongletFacturation' as Cle, icon: FileText   },
  // Il y a eu ici un onglet « Légal ». Il montrait quatre phrases immuables sur
  // le CO — les mêmes pour toutes les entreprises, jamais fonction de ce que
  // contenait la base. Un onglet qui n'affiche rien de l'installation qu'on
  // consulte apprend à ne pas être ouvert, et fait douter des voisins qui, eux,
  // disent quelque chose. Ce que le logiciel a réellement à dire sur la
  // conformité vit dans Maintenance → Conformité, qui LIT les livres.
  { key: 'backups',   cle: 'pr.ongletSauvegardes' as Cle, icon: Database   },
  { key: 'maintenance', cle: 'pr.ongletMaintenance' as Cle, icon: Wrench   },
  // Ce que chacun règle pour LUI-MÊME : son second facteur, ses ordinateurs de
  // confiance. Visible de tous les rôles — y compris en lecture seule, qui peut
  // protéger son compte même sans y être obligé.
  { key: 'account',   cle: 'pr.ongletMonCompte'   as Cle, icon: UserCog    },
]

import { BackupPanel } from '@/components/settings/BackupPanel'
import { MFAPanel } from '@/components/settings/MFAPanel'
import { LanguagePanel } from '@/components/settings/LanguagePanel'
import { MaintenancePanel } from '@/components/settings/MaintenancePanel'
import { ReconciliationPanel } from '@/components/settings/ReconciliationPanel'
import { refusalMessage } from '@/utils/refusal'

export function SettingsPage() {
  const t = useT()
  const tv = useTv()
  const peutEcrire = useCanWrite()
  // L'onglet ouvert peut être désigné par l'adresse : `/settings#banking`.
  //
  // Sans cela, « Votre IBAN de paiement » dans la liste de mise en route
  // conduisait à Paramètres… sur l'onglet Identité, à charge pour l'utilisateur
  // de deviner lequel des sept portait le champ. Envoyer quelqu'un au bon écran
  // et le laisser chercher dedans, c'est ne pas l'avoir envoyé.
  const ancre = useLocation().hash.replace('#', '')
  const [tab,   setTab]   = useState(
    () => TABS.some(t => t.key === ancre) ? ancre : 'identity')
  const [saved, setSaved] = useState(false)
  const qc = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)

  // Load existing settings
  const { data: company, isLoading } = useQuery({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
  })

  const { register, handleSubmit, reset, watch, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      company_name: '',
      legal_form: '',
      che_number: '',
      vat_number: '',
      vat_status: '',
      phone: '',
      bank_name: '',
      bank_address: '',
      bank_bic: '',
      auto_post_invoices: false,
      email: '',
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
        vat_status:            (company.vat_status as FormData['vat_status']) ?? '',
        phone:                 company.phone                  ?? '',
        bank_name:             company.bank_name              ?? '',
        bank_address:          company.bank_address           ?? '',
        bank_bic:              company.bank_bic               ?? '',
        auto_post_invoices:    company.auto_post_invoices     ?? false,
        email:                 company.email                  ?? '',
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
      // La liste de mise en route relit ces mêmes champs. Sans cette ligne,
      // quelqu'un saisit sa localité, revient au tableau de bord et y lit
      // encore « il manque la localité » pendant trente secondes — le temps
      // que le cache expire. Une liste qui ment sur ce qu'on vient de faire
      // est pire que pas de liste.
      qc.invalidateQueries({ queryKey: ['onboarding'] })
      setSaved(true)
      setTimeout(() => setSaved(false), 3000)
    },
  })

  // Ce que le SERVEUR a retenu, pas ce que le navigateur a envoyé. Les deux
  // réduisent l'image ; c'est la base qui fait foi, et l'écran le dit.
  const [logoRetenu, setLogoRetenu] =
    useState<{ l: number; h: number; reduit: boolean } | null>(null)

  const uploadLogo = useMutation({
    mutationFn: (file: File) => settingsApi.uploadLogo(file),
    onSuccess: (res) => {
      const d = res.data as { width?: number; height?: number; resized?: boolean }
      if (d.width && d.height) {
        setLogoRetenu({ l: d.width, h: d.height, reduit: d.resized === true })
      }
      qc.invalidateQueries({ queryKey: ['company-settings'] })
    },
  })

  const deleteLogo = useMutation({
    mutationFn: () => settingsApi.deleteLogo(),
    onSuccess: () => {
      setLogoRetenu(null)
      qc.invalidateQueries({ queryKey: ['company-settings'] })
    },
  })

  const handleLogoFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    uploadLogo.mutate(file)
    // Reset so same file can be re-selected
    e.target.value = ''
  }

  // Le rôle se lit AVANT tout retour anticipé.
  //
  // React identifie un hook par son rang d'appel. Ce `useAuthStore` était placé
  // après `if (isLoading) return null` : au premier rendu la page sortait avant
  // lui, au second elle l'appelait — un hook de plus que la fois précédente,
  // ce que React refuse (erreur #310, « Rendered more hooks than during the
  // previous render »). L'écran cassait alors entièrement, avec une trace
  // minifiée illisible.
  // `useCanWrite` porte exactement cette regle (admin ou comptable) et la
  // tient a jour avec le serveur : la recalculer ici la faisait diverger en
  // silence le jour ou un role serait ajoute.
  const canManage = useCanWrite()

  if (isLoading) return null

  // La lecture seule ne voit ni Sauvegardes ni Maintenance : ces écrans exposent
  // le dossier de sauvegarde, l'état du chiffrement et la santé du système —
  // les consulter est déjà sensible.
  //
  // Le comptable les voit : contrôler l'intégrité des livres et en prendre une
  // copie fait partie de son métier. Ce qui reste réservé à l'administrateur est
  // à l'intérieur de ces écrans — restauration, chiffrement, réseau, comptes —
  // et le serveur le refuse indépendamment de ce qui s'affiche.
  const visibleTabs = TABS.filter(t => canManage || !ADMIN_ONLY.has(t.key))

  // Un onglet réservé sélectionné par un lien ou un rechargement ne doit pas
  // rester actif : sans ce recalage, la page afficherait un panneau vide dont
  // chaque appel répondrait 403.
  const effectiveTab = visibleTabs.some(t => t.key === tab) ? tab : visibleTabs[0].key

  return (
    <div>
      <PageHeader
        title={t('nav.parametres')}
        subtitle={t('pr.sousTitre')}
        actions={
          // L'onglet Sauvegardes n'a rien à enregistrer : ses actions lui sont propres.
          // Un compte en lecture seule n'a rien à enregistrer : le bouton
          // partirait vers un 403, après avoir laissé croire que la saisie
          // comptait.
          !peutEcrire || effectiveTab === 'backups' || effectiveTab === 'maintenance'
            || effectiveTab === 'account' ? null : (
            <button
              form="settings-form"
              type="submit"
              className="btn-primary flex items-center gap-1.5"
              disabled={save.isPending}
            >
              {save.isPending ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
              {saved ? t('pr.sauvegarde') : t('action.enregistrer')}
            </button>
          )
        }
      />

      {save.isError && (
        <ErrorBanner message={refusalMessage(save.error, t('pr.erreurSauvegarde'))} />
      )}

      {/* IBAN missing warning — shown until an IBAN is saved */}
      {company && !company.iban && (
        <div className="mb-4 flex items-start gap-2.5 rounded-lg border border-warning-100 bg-warning-100 px-4 py-3 text-sm text-warning-700">
          <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-warning-500" />
          <span>
            {t('pr.aucunIban')}
          </span>
        </div>
      )}

      <div className="flex gap-6">
        {/* Nav latérale */}
        <nav className="w-44 flex-shrink-0">
          <div className="space-y-0.5">
            {visibleTabs.map(onglet => (
              <button
                key={onglet.key}
                onClick={() => setTab(onglet.key)}
                className={`w-full flex items-center gap-2.5 px-3 py-2.5 rounded-lg
                            text-sm text-left transition-all ${
                  effectiveTab === onglet.key
                    ? 'bg-alpine-800 text-white font-medium'
                    : 'text-alpine-600 hover:bg-alpine-100 hover:text-alpine-900'
                }`}
              >
                <onglet.icon size={15} className="flex-shrink-0" />
                {t(onglet.cle)}
              </button>
            ))}
          </div>
        </nav>

        {effectiveTab === 'backups' ? (
          <div className="flex-1"><BackupPanel /></div>
        ) : effectiveTab === 'maintenance' ? (
          /* `tab` et non `effectiveTab` ici : un onglet interdit sélectionné par
             un lien affichait quand même son panneau, dont chaque appel
             répondait 403. Le recalage ne servait à rien tant que le rendu
             lisait la valeur brute. */
          <div className="flex-1"><MaintenancePanel /></div>
        ) : effectiveTab === 'account' ? (
          <div className="flex-1"><LanguagePanel /><MFAPanel /></div>
        ) : (
        /* Formulaire */
        <form id="settings-form" onSubmit={handleSubmit(d => save.mutate(d))} className="flex-1 space-y-5">
        {/* Un `fieldset` désactivé neutralise NATIVEMENT chaque champ, liste et
            bouton qu'il contient — y compris ceux qu'on y ajoutera plus tard.
            Désactiver champ par champ marche le jour où on l'écrit, puis se
            périme au premier champ ajouté sans y penser : c'est exactement le
            motif qui a déjà laissé passer des fonctions non gardées. */}
        <fieldset disabled={!peutEcrire} className="contents">

          {/* ─── Identité ─────────────────────────────────────────────── */}
          {effectiveTab === 'identity' && (
            <>
              {/* Logo */}
              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">{t('pr.logoSociete')}</h2>
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

                    {/* Actions — déposer ou retirer un logo modifie ce que
                        portent toutes les factures PDF émises ensuite. Un compte
                        en lecture seule voit donc le logo et rien pour y
                        toucher : ni bouton, ni champ de fichier dans la page. */}
                    <div className="space-y-2">
                      <p className="text-xs text-alpine-500">
                        {t('pr.logoAide')}
                      </p>
                      {logoRetenu && (
                        <p className="text-xs text-success-700">
                          {t(logoRetenu.reduit ? 'pr.logoReduit' : 'pr.logoEnregistre',
                             { l: logoRetenu.l, h: logoRetenu.h })}
                        </p>
                      )}
                      {!peutEcrire && (
                        <p className="text-xs text-alpine-500">{t(RAISON_LECTURE_SEULE)}</p>
                      )}
                      {peutEcrire && (
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
                          {company?.logo_data ? t('pr.remplacer') : t('action.telecharger')}
                        </button>
                        {company?.logo_data && (
                          <button
                            type="button"
                            onClick={() => deleteLogo.mutate()}
                            disabled={deleteLogo.isPending}
                            className="btn-ghost btn-sm text-danger-700 flex items-center gap-1.5"
                          >
                            {deleteLogo.isPending
                              ? <Loader2 size={13} className="animate-spin" />
                              : <Trash2 size={13} />
                            }
                            Supprimer
                          </button>
                        )}
                      </div>
                      )}
                      {uploadLogo.isError && (
                        <p className="text-xs text-danger-700">
                          {refusalMessage(uploadLogo.error, t('pr.erreurTelechargement'))}
                        </p>
                      )}
                    </div>

                    {/* Le champ de fichier n'est pas seulement caché : il est
                        absent du document quand le compte ne peut pas écrire. */}
                    {peutEcrire && (
                      <input
                        ref={fileInputRef}
                        type="file"
                        accept="image/png,image/jpeg"
                        className="hidden"
                        onChange={handleLogoFile}
                      />
                    )}
                  </div>
                </div>
              </div>

              {/* Identity fields */}
              <div className="card">
                <div className="card-header">
                  <h2 className="text-sm font-semibold text-alpine-800">{t('pr.identiteSociete')}</h2>
                </div>
                <div className="card-body grid grid-cols-2 gap-4">
                  <div className="col-span-2">
                    <label className="label">{t('pr.nomCommercial')}</label>
                    <input
                      className={`input ${errors.company_name ? 'input-error' : ''}`}
                      placeholder={t('pr.nomExemple')}
                      {...register('company_name')}
                    />
                    {errors.company_name && (
                      <p className="text-xs text-danger-700 mt-1">{tv(errors.company_name.message)}</p>
                    )}
                  </div>
                  <div>
                    <label className="label">{t('pr.formeJuridique')}</label>
                    <input className="input" placeholder={t('pr.formeExemple')} {...register('legal_form')} />
                  </div>
                  <div>
                    <label className="label">{t('pr.ideSuisse')}</label>
                    <input className="input font-mono" placeholder="CHE-123.456.789" {...register('che_number')} />
                  </div>
                  <div>
                    <label className="label">{t('pr.telephone')}</label>
                    <input className="input" placeholder={t('pr.telExemple')} {...register('phone')} />
                  </div>
                  <div>
                    <label className="label">{t('pr.courriel')}</label>
                    <input className="input" type="email" placeholder={t('pr.mailExemple')} {...register('email')} />
                  </div>
                  {/* Téléphone et courriel apparaissent sur la facture. La LTVA
                      art. 26 ne les exige pas ; une facture qu'on ne peut pas
                      contester facilement se paie tard, ou pas. */}
                  <div className="col-span-2">
                    <label className="label">{t('pr.adresse')}</label>
                    <input className="input mb-2" placeholder={t('pr.rueNumero')} {...register('address_street')} />
                    <div className="grid grid-cols-4 gap-3">
                      <input className="input" placeholder={t('pr.npa')}      {...register('address_postal_code')} />
                      <input className="input col-span-2" placeholder={t('pr.localite')} {...register('address_city')} />
                      <input className="input uppercase" placeholder="CH" maxLength={2} {...register('address_country')} />
                    </div>
                  </div>
                </div>
              </div>
            </>
          )}

          {/* ─── Banque ───────────────────────────────────────────────── */}
          {effectiveTab === 'banking' && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-sm font-semibold text-alpine-800">{t('pr.coordonneesBancaires')}</h2>
              </div>
              <div className="card-body grid grid-cols-2 gap-4">
                <div className="col-span-2">
                  <label className="label">{t('paiement.iban')} <span className="text-warning-700 font-normal">{t('pr.ibanRequis')}</span></label>
                  <input className="input font-mono" placeholder="CH56 0483 5012 3456 7800 9" {...register('iban')} />
                  {/* Un QR-IBAN change ce que porte le bulletin : il impose une
                      référence QRR, là où un IBAN ordinaire impose SCOR ou NON
                      (SIX IG v2.4 §4.2.2). LedgerAlps le reconnaît seul, mais le
                      dire ici évite de se demander pourquoi les références
                      changent d'aspect d'une installation à l'autre. */}
                  {estQRIBAN(watch('iban') ?? '') ? (
                    <p className="text-xs text-alpine-500 mt-1">
                      {t('pr.ibanEstQR')}
                    </p>
                  ) : (
                    <p className="text-xs text-alpine-400 mt-1">
                      {t('pr.ibanAide')}
                    </p>
                  )}
                </div>
                {/* ── Le statut TVA ──────────────────────────────────────────
                    Une DÉCISION, posée avant le numéro, parce que c'est elle
                    qui détermine s'il y a un numéro à saisir. Tant qu'elle
                    n'est pas prise, LedgerAlps applique 8.1 % par défaut puis
                    refuse la facture : le mur arrive après le travail. */}
                <div className="col-span-2 pt-2 border-t border-neutral-200">
                  <label className="label">{t('pr.statutTVA')}</label>
                  <div className="space-y-2 mt-1">
                    <Statut valeur="liable" titre={t('pr.tvaAssujetti')}
                            aide={t('pr.tvaAssujettiAide')} register={register} />
                    <Statut valeur="exempt" titre={t('pr.tvaNonAssujetti')}
                            aide={t('pr.tvaNonAssujettiAide')} register={register} />
                  </div>
                  {watch('vat_status') === '' && (
                    <p className="text-xs text-warning-700 mt-2">{t('pr.tvaNonDeclare')}</p>
                  )}
                </div>

                {/* Le numéro n'a de sens qu'en étant assujetti. Le montrer à un
                    non-assujetti inviterait à saisir ce que la LTVA art. 27
                    al. 1 lui interdit de faire figurer — et le serveur efface
                    ce champ quand « non assujetti » est enregistré, parce que
                    ce numéro s'imprime sur la facture. */}
                {watch('vat_status') !== 'exempt' && (
                  <div className="col-span-2">
                    <label className="label">{t('pr.tvaAFC')}</label>
                    <input className="input font-mono" placeholder="CHE-123.456.789 MWST" {...register('vat_number')} />
                    <p className="text-xs text-alpine-400 mt-1">{t('pr.tvaAFCAide')}</p>
                  </div>
                )}
                {watch('vat_status') === 'exempt' && (
                  <div className="col-span-2">
                    <p className="text-xs text-alpine-500">{t('pr.tvaNumeroEfface')}</p>
                  </div>
                )}

                {/* Coordonnées de virement. La QR-facture suffit à un paiement
                    en Suisse ; un virement depuis l'étranger demande le nom de
                    la banque et le BIC. Facultatif, donc présenté comme tel. */}
                <div className="col-span-2 pt-2 border-t border-neutral-200">
                  <h3 className="text-sm font-medium text-alpine-800 mb-1">
                    {t('pr.paiementVirement')} <span className="font-normal text-alpine-400">{t('pr.facultatif')}</span>
                  </h3>
                  <p className="text-xs text-alpine-400 mb-3">{t('pr.virementAide')}</p>
                </div>
                <div>
                  <label className="label">{t('pr.nomBanque')}</label>
                  <input className="input" placeholder={t('pr.banqueExemple')} {...register('bank_name')} />
                </div>
                <div>
                  <label className="label">{t('pr.bic')}</label>
                  <input className="input font-mono uppercase" placeholder="BCVLCH2LXXX" {...register('bank_bic')} />
                </div>
                <div className="col-span-2">
                  <label className="label">{t('pr.adresseBanque')}</label>
                  <input className="input" placeholder={t('pr.adresseBanqueExemple')} {...register('bank_address')} />
                </div>
              </div>
            </div>
          )}

          {/* Le rapprochement vit dans l'onglet Banque : c'est là qu'on vient
              quand on a le relevé sous les yeux. */}
          {effectiveTab === 'banking' && (
            <div className="card card-pad mt-5">
              <ReconciliationPanel />
            </div>
          )}

          {/* ─── Facturation ──────────────────────────────────────────── */}
          {effectiveTab === 'invoicing' && (
            <div className="card">
              <div className="card-header">
                <h2 className="text-sm font-semibold text-alpine-800">{t('pr.parametresFacturation')}</h2>
              </div>
              <div className="card-body grid grid-cols-2 gap-4">
                {/* Comptabilisation automatique. Éteinte sur les installations
                    antérieures à ce réglage : l'allumer d'office y doublerait
                    les écritures déjà saisies à la main. */}
                <div className="col-span-2 rounded-md border border-neutral-200 px-3 py-2.5">
                  <label className="flex items-start gap-2 text-sm">
                    <input type="checkbox" className="mt-0.5" {...register('auto_post_invoices')} />
                    <span>
                      <span className="font-medium">{t('pr.comptabiliserEnvoi')}</span>
                      <span className="block text-alpine-600 text-xs mt-0.5">
                        {t('pr.comptabiliserAide')}
                      </span>
                      <span className="block text-alpine-600 text-xs mt-1">
                        {t('pr.comptabiliserEteint')}
                      </span>
                    </span>
                  </label>
                </div>

                <div>
                  <label className="label">{t('pr.devise')}</label>
                  <select className="select" {...register('currency')}>
                    <option value="CHF">{t('pr.deviseCHF')}</option>
                    <option value="EUR">{t('pr.deviseEUR')}</option>
                  </select>
                </div>
                <div>
                  <label className="label">{t('pr.debutExercice')}</label>
                  <select className="select" {...register('fiscal_year_start_month')}>
                    {[1,2,3,4,5,6,7,8,9,10,11,12].map(m => (
                      <option key={m} value={m}>{t(`pr.mois${m}` as Cle)}</option>
                    ))}
                  </select>
                </div>
              </div>
            </div>
          )}

        </fieldset>
        </form>
        )}
      </div>
    </div>
  )
}

// Un choix de statut TVA : une pastille, un titre, une phrase.
//
// Une liste déroulante aurait tenu en moins de place et caché la conséquence :
// « assujetti » et « non assujetti » n'ont pas le même effet sur ce qui sort de
// l'imprimante, et c'est cette différence qu'il faut lire AVANT de choisir, pas
// découvrir au premier refus.
function Statut({
  valeur,
  titre,
  aide,
  register,
}: {
  valeur: 'liable' | 'exempt'
  titre: string
  aide: string
  register: UseFormRegister<FormData>
}) {
  return (
    <label className="flex items-start gap-2.5 cursor-pointer rounded-md
                      px-2 py-1.5 hover:bg-alpine-50 transition-colors">
      <input
        type="radio"
        value={valeur}
        className="mt-1 flex-shrink-0"
        {...register('vat_status')}
      />
      <span>
        <span className="block text-sm font-medium text-alpine-800">{titre}</span>
        <span className="block text-xs text-alpine-500">{aide}</span>
      </span>
    </label>
  )
}
