// LedgerAlps — Formulaire de création de facture

import { useEffect, useRef, useState } from 'react'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { Plus, Trash2, ArrowLeft, Save, UserPlus, X, AlertTriangle } from 'lucide-react'
import { invoicesApi, contactsApi, settingsApi } from '@/api/client'
import { PageHeader, ErrorBanner, ConfirmDialog } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { formatCHF } from '@/utils'
import type { Contact, CompanySettings } from '@/types'
import { useUnsavedGuard } from '@/hooks/useUnsavedGuard'
import { useT, useTv } from '@/i18n/useT'

const lineSchema = z.object({
  description:      z.string().min(1, 'val.requis'),
  quantity:         z.coerce.number().positive(),
  unit:             z.string().optional(),
  unit_price:       z.coerce.number().positive('val.prixRequis'),
  discount_pct: z.coerce.number().min(0).max(100).default(0),
  vat_rate:         z.coerce.number().min(0).default(8.1),
})

const schema = z.object({
  document_type: z.enum(['invoice', 'quote', 'credit_note']).default('invoice'),
  contact_id:    z.string().min(1, 'val.contactRequis'),
  issue_date:    z.string().min(1, 'val.dateRequise'),
  due_date:      z.string().min(1, 'val.echeanceRequise'),
  notes:         z.string().optional(),
  terms:         z.string().optional(),
  lines:         z.array(lineSchema).min(1, 'val.auMoinsUneLigne'),
})

type FormData = z.infer<typeof schema>

function computeLineTotals(line: Partial<FormData['lines'][0]>) {
  const qty      = Number(line.quantity     ?? 1)
  const price    = Number(line.unit_price   ?? 0)
  const discount = Number(line.discount_pct ?? 0) / 100
  const vatRate  = Number(line.vat_rate     ?? 8.1) / 100
  const base     = qty * price * (1 - discount)
  const vat      = Math.round(base * vatRate * 20) / 20  // arrondi 5 rappen
  return { base: Math.round(base * 100) / 100, vat: Math.round(vat * 100) / 100, total: Math.round((base + vat) * 100) / 100 }
}

// ── Modal de création rapide de contact ────────────────────────────────────────
const EMPTY_CONTACT = {
  name: '', is_company: false, email: '', phone: '', city: '', country: 'CH',
}

function NewContactModal({
  onClose,
  onCreated,
}: {
  onClose: () => void
  onCreated: (contact: Contact) => void
}) {
  const t = useT()
  const qc = useQueryClient()
  const [fields, setFields] = useState(EMPTY_CONTACT)
  const [err, setErr] = useState<string | null>(null)

  const create = useMutation({
    mutationFn: () => contactsApi.create({
      contact_type:      'customer',
      is_company:        fields.is_company,
      name:              fields.name.trim(),
      email:             fields.email || undefined,
      phone:             fields.phone || undefined,
      city:              fields.city  || undefined,
      country:           fields.country || 'CH',
      payment_term_days: 30,
      currency:          'CHF',
    }),
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ['contacts'] })
      onCreated(res.data as Contact)
    },
    onError: () => setErr(t('nf.erreurContact')),
  })

  const set = (key: keyof typeof EMPTY_CONTACT, value: string | boolean) =>
    setFields(f => ({ ...f, [key]: value }))

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
              className="input"
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

          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2">
              <label className="label">{t('nf.ville')}</label>
              <input className="input" value={fields.city}
                onChange={e => set('city', e.target.value)} />
            </div>
            <div>
              <label className="label">{t('nf.pays')}</label>
              <input className="input" maxLength={2} value={fields.country}
                onChange={e => set('country', e.target.value.toUpperCase())} />
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-alpine-200">
          <button type="button" onClick={onClose} className="btn-secondary btn-sm">
            {t('action.annuler')}
          </button>
          <button
            type="button"
            disabled={!fields.name.trim() || create.isPending}
            onClick={() => create.mutate()}
            className="btn-primary btn-sm"
          >
            {create.isPending ? t('fd.creationEnCours') : t('nf.creerContact')}
          </button>
        </div>
      </div>
    </div>
  )
}

// ── Page principale ────────────────────────────────────────────────────────────
export function NewInvoicePage() {
  const t = useT()
  const tv = useTv()
  const navigate  = useNavigate()
  const qc        = useQueryClient()
  const [showContactModal, setShowContactModal] = useState(false)

  const { data: contacts = [] } = useQuery<Contact[]>({
    queryKey: ['contacts'],
    queryFn:  () => contactsApi.list().then(r => r.data),
  })

  const { data: company } = useQuery<CompanySettings>({
    queryKey: ['company-settings'],
    queryFn:  () => settingsApi.getCompany().then(r => r.data),
  })

  // Le taux proposé suit le statut TVA déclaré.
  //
  // Avant ce réglage, chaque ligne partait à 8.1 % et le refus tombait au
  // moment d'établir la facture : le mur arrivait APRÈS le travail, et celui
  // qui n'est pas assujetti ne pouvait le comprendre qu'en lisant la LTVA.
  // Tant que la question n'a pas de réponse, on garde 8.1 % — c'est le cas le
  // plus fréquent, et proposer 0 % à un assujetti lui ferait sous-facturer.
  const tauxParDefaut = company?.vat_status === 'exempt' ? 0 : 8.1

  const today = new Date().toISOString().slice(0, 10)
  const defaultDueDate = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString().slice(0, 10)

  const {
    register, control, handleSubmit, setValue,
    formState: { errors, isDirty },
  } = useForm<FormData>({
    resolver: zodResolver(schema),
    defaultValues: {
      document_type: 'invoice',
      issue_date:    today,
      due_date:      defaultDueDate,
      lines: [{ description: '', quantity: 1, unit_price: 0, discount_pct: 0, vat_rate: 8.1 }],
    },
  })

  const { fields, append, remove } = useFieldArray({ control, name: 'lines' })

  // La fiche société arrive APRÈS le montage : `defaultValues` est déjà figé
  // quand on apprend le statut. La première ligne est donc recalée une fois,
  // et seulement tant que rien n'a été saisi — corriger un taux que quelqu'un
  // vient de choisir serait pire que le mauvais défaut.
  const tauxRecale = useRef(false)
  useEffect(() => {
    if (!company || tauxRecale.current || isDirty) return
    tauxRecale.current = true
    if (tauxParDefaut !== 8.1) setValue('lines.0.vat_rate', tauxParDefaut)
  }, [company, isDirty, tauxParDefaut, setValue])

  // Une facture de quinze lignes disparaît entièrement sur un clic dans le
  // menu : LedgerAlps n'enregistre aucun brouillon automatique. Le garde bloque
  // la navigation interne ET la fermeture de l'onglet, et ne demande rien tant
  // que rien n'a été saisi.
  const guard = useUnsavedGuard(isDirty)
  const watchedLines     = useWatch({ control, name: 'lines' })
  const watchedContactId = useWatch({ control, name: 'contact_id' })
  const watchedDocType   = useWatch({ control, name: 'document_type' })

  const totals = (watchedLines ?? []).map(computeLineTotals)

  // QR bill readiness: check what's missing so the payment slip can be generated.
  const selectedContact = contacts.find(c => c.id === watchedContactId)
  const qrIssues: string[] = []
  if (watchedDocType === 'invoice') {
    if (!company?.iban) qrIssues.push(t('nf.qrSansIban'))
    if (selectedContact) {
      if (!selectedContact.address) qrIssues.push(t('nf.qrSansAdresse'))
      if (!selectedContact.postal_code || !selectedContact.city) qrIssues.push(t('nf.qrSansNPA'))
    }
  }
  const subtotal   = totals.reduce((s, t) => s + t.base,  0)
  const totalVAT   = totals.reduce((s, t) => s + t.vat,   0)
  const grandTotal = totals.reduce((s, t) => s + t.total, 0)

  const create = useMutation({
    mutationFn: (data: FormData) => invoicesApi.create(data),
    onSuccess: (res) => {
      // Désarmer AVANT de naviguer : la facture est enregistrée, et le garde
      // annoncerait sinon que la saisie va être perdue — un message faux, sur
      // lequel on clique « Annuler », ce qui bloque sur un écran dont le
      // bouton principal semble ne rien faire.
      guard.désarmer()
      qc.invalidateQueries({ queryKey: ['invoices'] })
      navigate(`/invoices/${res.data.id}`)
    },
  })

  return (
    <div>
      <ConfirmDialog
        open={guard.blocked}
        tone="danger"
        title={t('nf.quitterTitre')}
        consequences={[t('nf.quitterCons')]}
        reassurance={t('nf.quitterRassur')}
        confirmLabel={t('nf.quitterConfirmer')}
        onConfirm={guard.confirmLeave}
        onCancel={guard.cancelLeave}
      />
      {showContactModal && (
        <NewContactModal
          onClose={() => setShowContactModal(false)}
          onCreated={(contact) => {
            setShowContactModal(false)
            // Poser le contact dans le cache, PUIS le selectionner apres rendu.
            //
            // Deux ecueils se referment ici. L'ordre d'origine attendait le
            // rechargement avant de selectionner : le champ restait vide le
            // temps de l'appel reseau, et pour toujours si celui-ci echouait,
            // puisque rien n'attrapait le rejet. Mais selectionner sans plus
            // attendre ne suffit pas non plus : tant que la liste n'a pas ete
            // rechargee, le <select> n'a AUCUNE option portant cet
            // identifiant, et la valeur posee ne s'affiche pas -- verifie en
            // navigateur, le champ restait sur « Selectionnez un contact ».
            //
            // On insere donc le contact dans le cache, ce qui cree l'option
            // immediatement et sans reseau. La modale a deja demande le
            // rechargement de son cote : la liste faisant foi arrive juste
            // apres et remplace celle-ci.
            //
            // La selection, elle, attend le tour de rendu suivant. setValue
            // ecrit dans l'element du DOM, et React n'a pas encore pose la
            // nouvelle option quand la ligne au-dessus rend la main : le
            // <select> refusait la valeur en silence. Le .finally donne ce
            // tour, et il s'execute que le rechargement reussisse ou non --
            // l'option existe de toute facon, elle vient du cache.
            qc.setQueryData<Contact[]>(['contacts'], (liste) =>
              liste ? [...liste, contact] : [contact])
            void qc.invalidateQueries({ queryKey: ['contacts'] })
              .finally(() => setValue('contact_id', contact.id, { shouldValidate: true }))
          }}
        />
      )}

      <PageHeader
        title={t('nf.titre')}
        actions={
          <button onClick={() => navigate(-1)} className="btn-secondary">
            <ArrowLeft size={15} /> {t('action.retour')}
          </button>
        }
      />

      {/* Le message générique masquait la vraie raison d'un refus : sans TVA
          enregistrée, le serveur explique pourquoi il refuse et ce qu'il faut
          faire. Un « Erreur lors de la création » n'apprend rien. */}
      {create.error && <ErrorBanner message={refusalMessage(create.error, t('nf.erreurCreation'))} />}

      <form onSubmit={handleSubmit(d => create.mutate(d))} className="space-y-5">
        {/* Infos document */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('nf.infosDocument')}</h2>
          </div>
          <div className="card-body grid grid-cols-2 md:grid-cols-4 gap-4">
            {/* Type de document */}
            <div>
              <label className="label">{t('nf.type')}</label>
              <select className="select" {...register('document_type')}>
                <option value="invoice">{t('doc.facture')}</option>
                <option value="quote">{t('doc.offre')}</option>
                <option value="credit_note">{t('doc.noteDeCredit')}</option>
              </select>
            </div>

            {/* Contact */}
            <div className="col-span-2 md:col-span-1">
              <label className="label">{t('nf.contact')}</label>
              <div className="flex gap-2">
                <select
                  className={`select flex-1 ${errors.contact_id ? 'input-error' : ''}`}
                  {...register('contact_id')}
                >
                  <option value="">{t('nf.choisirContact')}</option>
                  {contacts.map(c => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
                <button
                  type="button"
                  onClick={() => setShowContactModal(true)}
                  className="btn-secondary btn-sm shrink-0 flex items-center gap-1.5"
                  title={t('nf.nouveauContactInfoBulle')}
                >
                  <UserPlus size={14} />
                  <span className="hidden sm:inline">{t('nf.nouveau')}</span>
                </button>
              </div>
              {errors.contact_id && <p className="error-msg">{tv(errors.contact_id.message)}</p>}
            </div>

            {/* Date émission */}
            <div>
              <label className="label">{t('nf.dateEmission')}</label>
              <input
                type="date"
                className={`input ${errors.issue_date ? 'input-error' : ''}`}
                {...register('issue_date')}
              />
            </div>

            {/* Échéance */}
            <div>
              <label className="label">{t('nf.echeance')}</label>
              <input
                type="date"
                className={`input ${errors.due_date ? 'input-error' : ''}`}
                {...register('due_date')}
              />
              {errors.due_date && <p className="error-msg">{tv(errors.due_date.message)}</p>}
            </div>

          </div>
        </div>

        {/* QR bill readiness warning */}
        {watchedDocType === 'invoice' && qrIssues.length > 0 && (
          <div className="flex items-start gap-2.5 rounded-lg border border-warning-100 bg-warning-100/70 px-4 py-3 text-sm">
            <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-warning-500" />
            <div>
              <p className="font-medium text-warning-700">{t('nf.qrIncomplet')}</p>
              <ul className="mt-1 space-y-0.5 list-disc list-inside text-xs text-warning-700">
                {qrIssues.map((issue, i) => <li key={i}>{issue}</li>)}
              </ul>
            </div>
          </div>
        )}

        {/* Lignes */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('nf.lignesFacture')}</h2>
            <button
              type="button"
              className="btn-secondary btn-sm"
              onClick={() => append({ description: '', quantity: 1, unit_price: 0, discount_pct: 0, vat_rate: tauxParDefaut })}
            >
              <Plus size={14} /> {t('nf.ajouterLigne')}
            </button>
          </div>
          <div className="card-body p-0">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-alpine-50 border-b border-alpine-200">
                    {([
                      'jr.colDescription', 'fd.colQte', 'nf.colUnite', 'nf.colPrixUnit',
                      'nf.colRabaisPct', 'nf.colTVAPct', 'nf.colTotalHT',
                    ] as const).map(cle => (
                      <th key={cle} className="px-4 py-2.5 text-left text-xs font-semibold text-alpine-600 uppercase tracking-wide">
                        {t(cle)}
                      </th>
                    ))}
                    <th className="px-4 py-2.5" />
                  </tr>
                </thead>
                <tbody>
                  {fields.map((field, i) => {
                    const ligne = totals[i] ?? { base: 0, vat: 0, total: 0 }
                    return (
                      <tr key={field.id} className="border-b border-alpine-100 last:border-0">
                        {/* aria-label sur chaque champ : sans lui, un lecteur
                            d'écran annonce « 1 », « 0 », « 0 » sans dire de
                            quelle colonne il s'agit — l'en-tête du tableau ne
                            suffit pas à nommer un champ de saisie. */}
                        <td className="px-4 py-2 w-[30%]">
                          <input
                            className={`input ${errors.lines?.[i]?.description ? 'input-error' : ''}`}
                            placeholder={t('nf.placeholderDescription')}
                            aria-label={t('nf.ariaDescription', { n: i + 1 })}
                            {...register(`lines.${i}.description`)}
                          />
                          {errors.lines?.[i]?.description && (
                            <p className="error-msg">{tv(errors.lines[i]?.description?.message)}</p>
                          )}
                        </td>
                        <td className="px-2 py-2 w-20">
                          <input type="number" step="0.001" min="0.001"
                            aria-label={t('nf.ariaQuantite', { n: i + 1 })}
                            className="input text-right" {...register(`lines.${i}.quantity`)} />
                        </td>
                        <td className="px-2 py-2 w-16">
                          <input className="input" placeholder={t('nf.placeholderUnite')}
                            aria-label={t('nf.ariaUnite', { n: i + 1 })}
                            {...register(`lines.${i}.unit`)} />
                        </td>
                        <td className="px-2 py-2 w-28">
                          <input type="number" step="0.01" min="0"
                            aria-label={t('nf.ariaPrix', { n: i + 1 })}
                            className={`input text-right font-mono ${errors.lines?.[i]?.unit_price ? 'input-error' : ''}`}
                            {...register(`lines.${i}.unit_price`)} />
                          {errors.lines?.[i]?.unit_price && (
                            <p className="error-msg">{tv(errors.lines[i]?.unit_price?.message)}</p>
                          )}
                        </td>
                        <td className="px-2 py-2 w-20">
                          <input type="number" step="0.1" min="0" max="100"
                            aria-label={t('nf.ariaRabais', { n: i + 1 })}
                            className="input text-right" {...register(`lines.${i}.discount_pct`)} />
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
                          {formatCHF(ligne.base)}
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
                <span>{t('fd.sousTotalHT')}</span>
                <span className="font-mono">{formatCHF(subtotal)}</span>
              </div>
              <div className="flex justify-between text-alpine-600">
                <span>{t('tva.tva')}</span>
                <span className="font-mono">{formatCHF(totalVAT)}</span>
              </div>
              <div className="flex justify-between font-semibold text-base text-alpine-900 pt-1 border-t border-alpine-200">
                <span>{t('fact.colTotal')}</span>
                <span className="font-mono">{formatCHF(grandTotal)}</span>
              </div>
            </div>
          </div>
        </div>

        {/* Notes / Conditions */}
        <div className="card">
          <div className="card-header">
            <h2 className="text-sm font-semibold text-alpine-800">{t('fd.remarques')}</h2>
          </div>
          <div className="card-body grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label className="label">{t('nf.notesInternes')}</label>
              <textarea rows={3} className="input resize-none" {...register('notes')} />
            </div>
            <div>
              <label className="label">{t('nf.conditionsPaiement')}</label>
              <textarea rows={3} className="input resize-none"
                placeholder={t('nf.placeholderConditions')}
                {...register('terms')} />
            </div>
          </div>
        </div>

        {/* Actions */}
        <div className="flex justify-end gap-3 pb-6">
          <button type="button" onClick={() => navigate(-1)} className="btn-secondary">
            {t('action.annuler')}
          </button>
          <button
            type="submit"
            className="btn-primary"
            disabled={create.isPending}
          >
            <Save size={15} />
            {create.isPending ? t('etat.enregistrement') : t('nf.creerBrouillon')}
          </button>
        </div>
      </form>
    </div>
  )
}
