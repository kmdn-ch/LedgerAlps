// LedgerAlps — Achats : factures fournisseurs et ordres de paiement.
//
// Le backend des factures fournisseurs existait depuis longtemps ; l'interface,
// non. On ne pouvait donc en saisir une qu'en forgeant une requête HTTP — ce qui
// revient à dire que la fonction n'existait pas — et l'écran de paiement, qui
// s'appuie dessus, n'aurait rien eu à lister.
//
// Les deux vivent sur la même page parce qu'ils sont les deux moitiés d'un même
// geste : on saisit ce qu'on doit, on le comptabilise, on le paie.
//
// # Trois états, et ce qu'ils veulent dire
//
// **Brouillon** — saisi, hors des livres. Ne compte ni à la TVA, ni au bilan.
// **Comptabilisé** — l'écriture est passée (charge + TVA déductible / créanciers)
// et scellée ; la facture devient payable.
// **Payé** — le débit est apparu au relevé bancaire.
//
// Générer un fichier de paiement ne fait PAS passer à « payé » : c'est le
// rapprochement camt.053 qui l'établit.

import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Plus, Loader2, FileText, CheckCircle, UserPlus, X, Upload, Pencil, ScanLine,
} from 'lucide-react'
import { supplierInvoicesApi, contactsApi, accountsApi } from '@/api/client'
import {
  PageHeader, LoadingSpinner, EmptyState, ErrorBanner, SectionTitle, ConfirmDialog,
} from '@/components/ui'
import { formatCHF, formatDate, estQRIBAN } from '@/utils'
import { refusalMessage } from '@/utils/refusal'
import { useUnsavedGuard } from '@/hooks/useUnsavedGuard'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { useT } from '@/i18n/useT'
import type { Cle } from '@/i18n'
import { PaymentRunPanel } from '@/components/payments/PaymentRunPanel'
import type { Account, Contact } from '@/types'

interface SupplierInvoice {
  id: string
  supplier_id: string
  supplier_name: string
  supplier_reference: string
  status: string
  issue_date: string
  due_date: string
  currency: string
  subtotal_amount: number
  vat_amount: number
  total_amount: number
  amount_paid: number
  expense_account_code?: string
  payment_reference?: string
  journal_entry_id?: string
}

const STATUS_CLE: Record<string, Cle> = {
  draft: 'statut.brouillon', booked: 'statut.comptabilisee',
  paid: 'statut.payee', cancelled: 'statut.annulee',
}
const STATUS_CLASS: Record<string, string> = {
  draft: 'badge-draft', booked: 'badge-sent', paid: 'badge-paid', cancelled: 'badge-cancelled',
}

// Les taux en vigueur depuis le 1er janvier 2024 (LTVA art. 25). Une liste
// fermée plutôt qu'un champ libre : le taux entre dans la déclaration, et une
// faute de frappe ne se découvre qu'au décompte trimestriel.
const VAT_RATES: { value: string; cle: Cle }[] = [
  { value: '8.1', cle: 'tva.tauxNormal' },
  { value: '2.6', cle: 'tva.tauxReduit' },
  { value: '3.8', cle: 'tva.tauxHebergement' },
  { value: '0',   cle: 'tva.tauxZero' },
]

const today = () => new Date().toISOString().slice(0, 10)

export function PurchasesPage() {
  const t = useT()
  const qc = useQueryClient()
  const peutEcrire = useCanWrite()
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [toBook, setToBook] = useState<SupplierInvoice | null>(null)

  const [form, setForm] = useState({
    supplier_id: '', supplier_reference: '',
    issue_date: today(), due_date: '',
    description: '', amount: '', amount_mode: 'ttc' as 'ht' | 'ttc', vat_rate: '8.1',
    expense_account_code: '', payment_reference: '',
  })

  const [newSupplier, setNewSupplier] = useState(false)
  const [supplierForm, setSupplierForm] = useState(
    { name: '', iban: '', qr_iban: '', email: '', vat_number: '' })

  // Facture en cours de modification. Nul = on saisit une nouvelle facture.
  const [editing, setEditing] = useState<SupplierInvoice | null>(null)
  // Ce que le QR a donné, affiché tel quel pour que l'utilisateur le vérifie
  // avant d'enregistrer. Rien n'est écrit tant qu'il n'a pas confirmé.
  const [scan, setScan] = useState<{ ok: boolean; message: string } | null>(null)

  const list = useQuery<{ items: SupplierInvoice[] }>({
    queryKey: ['supplier-invoices'],
    queryFn:  () => supplierInvoicesApi.list().then(r => r.data),
  })

  // GET /contacts rend un TABLEAU, pas { items }. La liste déroulante lisait
  // `.items` : elle était donc vide quel que soit le nombre de fournisseurs.
  // TypeScript ne pouvait rien dire — le type annoncé décrivait une réponse qui
  // n'existe pas, et rien ne vérifie une annotation contre la réalité du réseau.
  const suppliers = useQuery<Contact[]>({
    queryKey: ['contacts', 'suppliers'],
    queryFn:  () => contactsApi.list({ contact_type: 'supplier' }).then(r => r.data),
  })
  const supplierList = suppliers.data ?? []

  const accounts = useQuery<Account[]>({
    queryKey: ['accounts'],
    queryFn:  () => accountsApi.list().then(r => r.data),
    staleTime: 5 * 60_000,
  })
  // Seuls les comptes de charge : proposer les 81 comptes du plan pour choisir
  // où imputer un achat revient à ne rien proposer.
  const expenseAccounts = useMemo(
    () => (accounts.data ?? []).filter(a => a.account_type === 'expense'),
    [accounts.data],
  )

  const dirty = creating && (form.supplier_reference !== '' || form.amount !== '' ||
    form.description !== '')
  useUnsavedGuard(dirty)

  // Le mode de saisie survit à l'enregistrement : celui qu'on vient d'utiliser
  // est presque toujours celui de la facture suivante.
  const reset = () => setForm(f => ({
    supplier_id: '', supplier_reference: '', issue_date: today(), due_date: '',
    description: '', amount: '', amount_mode: f.amount_mode, vat_rate: '8.1',
    expense_account_code: '', payment_reference: '',
  }))

  // Le montant se saisit au choix en hors taxe ou en TTC.
  //
  // Une facture reçue annonce ce qu'il faut PAYER — un montant TTC — et c'est
  // aussi ce que porte le QR. Obliger à saisir du hors taxe revenait à demander
  // une division à chaque saisie, et le QR ne pouvait rien pré-remplir sans
  // écrire un TTC dans un champ qui attend autre chose.
  //
  // Quand la facture ne porte pas de TVA — fournisseur non assujetti — le taux
  // est 0 % et les deux montants sont égaux. Rien à déduire, rien à calculer :
  // c'est le cas le plus simple, et il devient trivial dans les deux modes.
  const rate    = parseFloat(form.vat_rate.replace(',', '.'))
  const saisi   = parseFloat(form.amount.replace(',', '.'))
  const taux    = isFinite(rate) ? rate / 100 : 0
  const ht = !isFinite(saisi)
    ? NaN
    : form.amount_mode === 'ttc'
      ? Math.round((saisi / (1 + taux)) * 100) / 100
      : saisi
  const tva = isFinite(ht) ? Math.round(ht * taux * 100) / 100 : 0
  const ttc = isFinite(ht) ? Math.round((ht + tva) * 20) / 20 : 0

  const create = useMutation({
    mutationFn: () => {
      const payload = {
      supplier_id: form.supplier_id,
      supplier_reference: form.supplier_reference.trim(),
      issue_date: form.issue_date,
      due_date: form.due_date || undefined,
      expense_account_code: form.expense_account_code || undefined,
      payment_reference: form.payment_reference.trim() || undefined,
        lines: [{
          description: form.description.trim() || form.supplier_reference.trim(),
          quantity: 1,
          unit_price: ht,
          vat_rate: rate / 100,
          expense_account_code: form.expense_account_code || undefined,
        }],
      }
      return editing
        ? supplierInvoicesApi.update(editing.id, payload)
        : supplierInvoicesApi.create(payload)
    },
    onSuccess: () => {
      setError(null); setCreating(false); setEditing(null); setScan(null); reset()
      qc.invalidateQueries({ queryKey: ['supplier-invoices'] })
      qc.invalidateQueries({ queryKey: ['payments-payable'] })
    },
    onError: (e) => setError(refusalMessage(e, "La facture n'a pas pu être enregistrée.")),
  })

  // Ouvre le formulaire sur une facture existante. Seul un brouillon est
  // modifiable : une facture comptabilisée porte une écriture scellée.
  const openEdit = (i: SupplierInvoice) => {
    setEditing(i)
    setScan(null)
    setError(null)
    setForm({
      supplier_id: i.supplier_id,
      supplier_reference: i.supplier_reference,
      issue_date: i.issue_date.slice(0, 10),
      due_date: i.due_date ? i.due_date.slice(0, 10) : '',
      description: '',
      amount: String(i.subtotal_amount),
      amount_mode: 'ht' as const,
      vat_rate: i.subtotal_amount > 0
        ? String(Math.round((i.vat_amount / i.subtotal_amount) * 1000) / 10)
        : '8.1',
      expense_account_code: i.expense_account_code ?? '',
      payment_reference: i.payment_reference ?? '',
    })
    setCreating(true)
  }

  // Le fournisseur créé ici est immédiatement sélectionné : la saisie reprend
  // là où elle s'est arrêtée, sans que l'on ait à rouvrir la liste.
  const createSupplier = useMutation({
    mutationFn: () => {
      // Les deux comptes sont saisis dans deux cases distinctes, et chacun part
      // dans son champ. Le tri final se fait quand même ici sur l'identifiant
      // d'institution : c'est la règle objective (positions 5 à 9, plage
      // 30000–31999), et une valeur mise dans la mauvaise case serait rejetée
      // par le serveur sans que l'on comprenne pourquoi.
      const brut = (v: string) => v.replace(/\s/g, '').toUpperCase()
      const a = brut(supplierForm.iban)
      const b = brut(supplierForm.qr_iban)
      const iban   = [a, b].find(v => v && !estQRIBAN(v)) || ''
      const qrIBAN = [a, b].find(v => v && estQRIBAN(v)) || ''
      return contactsApi.create({
        name: supplierForm.name.trim(),
        contact_type: 'supplier',
        email: supplierForm.email.trim() || undefined,
        iban: iban || undefined,
        qr_iban: qrIBAN || undefined,
        vat_number: supplierForm.vat_number.trim() || undefined,
        country: 'CH',
      })
    },
    onSuccess: async (r) => {
      setError(null); setNewSupplier(false)
      setSupplierForm({ name: '', iban: '', qr_iban: '', email: '', vat_number: '' })
      await qc.invalidateQueries({ queryKey: ['contacts'] })
      const created = r.data as { id?: string }
      if (created.id) setForm(f => ({ ...f, supplier_id: created.id as string }))
    },
    onError: (e) => setError(refusalMessage(e, "Le fournisseur n'a pas pu être créé.")),
  })

  // Le QR ne remplace pas la saisie : il la prépare. Le montant hors taxe, le
  // taux de TVA et le compte de charge n'y figurent pas — ce sont des décisions
  // comptables, pas des données du bulletin.
  const readQR = useMutation({
    mutationFn: (file: File) => supplierInvoicesApi.readQR(file),
    onSuccess: (r) => {
      const d = r.data as {
        found: boolean; reason?: string
        bill?: {
          creditor_name: string; creditor_iban: string; is_qr_iban: boolean
          amount: number
          currency: string; reference: string; reference_type: string; message: string
        }
        hints?: {
          invoice_number: string; invoice_number_label: string
          issue_date: string; issue_date_label: string
          due_date: string; due_date_label: string
          vat_rate: number; vat_mentioned: boolean; vat_label: string
          supplier_uid: string
        }
        supplier?: { id: string; name: string }
      }
      if (!d.found || !d.bill) {
        setScan({ ok: false, message: d.reason ?? t('ah.aucunQR') })
        return
      }
      const h = d.hints
      setCreating(true)
      setForm(f => ({
        ...f,
        supplier_id: d.supplier?.id || '',
        payment_reference: d.bill!.reference_type === 'NON' ? '' : d.bill!.reference,
        // Le numéro lu sur la facture prime sur le message libre du bulletin :
        // le premier est étiqueté « Numéro de facture », le second est du texte
        // que le fournisseur y met parfois, parfois pas.
        supplier_reference: h?.invoice_number || d.bill!.message || f.supplier_reference,
        issue_date: h?.issue_date || f.issue_date,
        due_date: h?.due_date || f.due_date,
        // Le QR porte le montant À PAYER, donc TTC. On bascule le champ dans ce
        // mode plutôt que de le laisser vide : c'est le montant que la facture
        // annonce, et le hors taxe s'en déduit du taux.
        amount: d.bill!.amount > 0 ? String(d.bill!.amount) : '',
        amount_mode: 'ttc' as const,
        // Aucune mention de TVA sur le document : le taux est 0 %, et le
        // montant du QR est donc aussi le montant hors taxe. C'est le cas d'un
        // fournisseur non assujetti — il n'y a rien à déduire (LTVA art. 28
        // al. 1, qui exige une facture mentionnant l'impôt pour le récupérer).
        vat_rate: h?.vat_mentioned ? String(h.vat_rate) : '0',
      }))

      // Fournisseur inconnu : on ouvre sa fiche pré-remplie plutôt que de
      // renvoyer l'utilisateur la créer ailleurs. Tout ce qu'il faut est dans
      // le QR — nom, compte — et le retaper serait une occasion de se tromper.
      if (!d.supplier?.id) {
        // Le compte du créancier va dans SA case. Une QR-facture porte un
        // QR-IBAN, et le ranger comme un IBAN ordinaire ferait rejeter le
        // virement : la référence QU'ELLE porte n'est acceptée qu'avec lui
        // (SIX IG v2.4 §4.2.2). C'est le serveur qui a tranché à la lecture,
        // sur l'identifiant d'institution 30000–31999 ; l'écran s'y range.
        const estQR = d.bill.is_qr_iban
        setNewSupplier(true)
        setSupplierForm({
          name: d.bill.creditor_name,
          iban:    estQR ? '' : d.bill.creditor_iban,
          qr_iban: estQR ? d.bill.creditor_iban : '',
          email: '',
          // Le numéro IDE est sur la facture, pas dans le QR. Le reporter ici
          // évite d'avoir à rouvrir le PDF pour compléter la fiche.
          vat_number: h?.supplier_uid ?? '',
        })
      }

      // Chaque valeur est annoncée AVEC l'étiquette qui l'a produite : c'est ce
      // qui permet de repérer une lecture de travers sans rouvrir le PDF.
      const lues: string[] = []
      if (h?.invoice_number) lues.push(`n° ${h.invoice_number} (« ${h.invoice_number_label} »)`)
      if (h?.issue_date) lues.push(`date ${h.issue_date} (« ${h.issue_date_label} »)`)
      if (h?.due_date) lues.push(`échéance ${h.due_date} (« ${h.due_date_label} »)`)
      lues.push(h?.vat_mentioned
        ? `TVA ${h.vat_rate} % (« ${h.vat_label} »)`
        : 'aucune TVA mentionnée → 0 %, le montant payé est le montant hors taxe')

      const suite = d.supplier?.id
        ? ''
        : ` Ce fournisseur n’est pas encore enregistré — sa fiche est pré-remplie ci-dessous.`
      setScan({
        ok: true,
        message: `${d.bill.creditor_name}, ${d.bill.amount.toFixed(2)} ` +
          `${d.bill.currency} à payer (TTC). Lu sur la facture : ${lues.join(' · ')}.` + suite,
      })
    },
    onError: (e) => setScan({
      ok: false,
      message: refusalMessage(e, "Le document n'a pas pu être lu."),
    }),
  })

  const book = useMutation({
    mutationFn: (id: string) => supplierInvoicesApi.transition(id, 'booked'),
    onSuccess: () => {
      setError(null); setToBook(null)
      qc.invalidateQueries({ queryKey: ['supplier-invoices'] })
      qc.invalidateQueries({ queryKey: ['payments-payable'] })
      qc.invalidateQueries({ queryKey: ['journal'] })
      qc.invalidateQueries({ queryKey: ['trial-balance'] })
    },
    onError: (e) => {
      setToBook(null)
      setError(refusalMessage(e, "La facture n'a pas pu être comptabilisée."))
    },
  })

  const items = list.data?.items ?? []
  const canCreate = form.supplier_id !== '' && form.supplier_reference.trim() !== '' &&
    isFinite(ht) && ht > 0 && !create.isPending

  return (
    <div>
      <PageHeader
        title={t('nav.achats')}
        subtitle={t('ach.sousTitre')}
        actions={
          // Déposer un fichier EST une écriture : le document est transmis au
          // serveur, lu, et il en sort une facture. Un compte en lecture seule
          // n'a donc ni le champ, ni le bouton — pas un bouton grisé, pas un
          // champ caché derrière un libellé : l'élément n'existe pas dans la
          // page, et il n'y a rien à réactiver depuis la console.
          peutEcrire ? (
          <div className="flex items-center gap-2">
            {/* Déposer d'abord, saisir ensuite : c'est l'ordre réel du geste.
                Le QR d'une facture suisse porte déjà le créancier, l'IBAN, le
                montant et la référence — aucune reconnaissance de caractères,
                donc aucune valeur devinée. */}
            <label className="btn-secondary flex items-center gap-1.5 cursor-pointer">
              {readQR.isPending
                ? <Loader2 size={15} className="animate-spin" />
                : <ScanLine size={15} />}
              {readQR.isPending ? t('ach.lecture') : t('ach.lirePDF')}
              <input type="file" accept=".pdf,.png,.jpg,.jpeg" className="hidden"
                     onChange={e => {
                       const f = e.target.files?.[0]
                       if (f) { setScan(null); readQR.mutate(f) }
                       e.target.value = ''
                     }} />
            </label>
            <button onClick={() => {
                      setEditing(null); setScan(null); setError(null)
                      setCreating(v => !v)
                    }}
                    className="btn-primary">
              <Plus size={15} /> {t('ach.saisirFacture')}
            </button>
          </div>
          ) : (
            <span className="text-xs text-alpine-500">{t(RAISON_LECTURE_SEULE)}</span>
          )
        }
      />

      {error && <div className="mb-4"><ErrorBanner message={error} /></div>}

      {/* Ce que le QR a donné, montré tel quel. Rien n'est enregistré : les
          champs sont pré-remplis, et c'est l'utilisateur qui valide. Un champ
          juste qu'on n'a pas vu vaut moins qu'un champ pré-rempli qu'on relit. */}
      {scan && (
        <div className={`mb-4 rounded-md border px-4 py-3 text-sm ${
          scan.ok ? 'border-success-500 bg-success-500/5' : 'border-neutral-300 bg-neutral-50'
        }`}>
          <p className="font-medium flex items-center gap-1.5">
            {scan.ok ? <ScanLine size={15} /> : <Upload size={15} />}
            {t(scan.ok ? 'ah.qrLu' : 'ah.rienALire')}
          </p>
          <p className="text-alpine-700 mt-1">{scan.message}</p>
          <button onClick={() => setScan(null)}
                  className="text-xs text-alpine-500 hover:text-alpine-700 mt-1.5">
            {t('ach.masquer')}
          </button>
        </div>
      )}

      {creating && (
        <div className="card card-pad mb-5">
          <SectionTitle>
            {editing
              ? t('ach.titreModifier', { ref: editing.supplier_reference })
              : t('ach.titreNouvelle')}
          </SectionTitle>

          {/* Création à la volée : un fournisseur inconnu ne doit pas obliger à
              quitter la page et perdre la saisie en cours. */}
          {newSupplier && (
            <div className="mb-4 rounded-md border border-accent-700 bg-accent-100/30 px-4 py-3">
              <div className="flex items-center justify-between mb-2">
                <p className="text-sm font-medium">{t('ach.nouveauFournisseur')}</p>
                <button type="button" onClick={() => setNewSupplier(false)}
                        className="text-alpine-500 hover:text-alpine-700" aria-label={t('action.fermer')}>
                  <X size={15} />
                </button>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div>
                  <label className="label" htmlFor="ns-name">{t('ach.nom')}</label>
                  <input id="ns-name" className="input" value={supplierForm.name}
                         onChange={e => setSupplierForm({ ...supplierForm, name: e.target.value })} />
                </div>
                {/* Deux comptes, deux cases. Une QR-facture porte un QR-IBAN et
                    remplit la seconde ; l'IBAN reste vide, et c'est correct.
                    Les réunir sous une seule étiquette « IBAN » revenait à
                    afficher un QR-IBAN sous un nom qui n'est pas le sien. */}
                <div>
                  <label className="label" htmlFor="ns-iban">{t('paiement.iban')}</label>
                  <input id="ns-iban" className="input font-mono" placeholder="CH.."
                         value={supplierForm.iban}
                         onChange={e => setSupplierForm({ ...supplierForm, iban: e.target.value })} />
                  {estQRIBAN(supplierForm.iban) ? (
                    <p className="text-xs text-amber-700 mt-1">
                      {t('ach.ibanEstQR')}
                    </p>
                  ) : (
                    <p className="text-xs text-alpine-500 mt-1">
                      {t('ach.ibanAide')}
                    </p>
                  )}
                </div>
                <div>
                  <label className="label" htmlFor="ns-qriban">{t('paiement.qrIban')}</label>
                  <input id="ns-qriban" className="input font-mono" placeholder="CH.."
                         value={supplierForm.qr_iban}
                         onChange={e => setSupplierForm({
                           ...supplierForm, qr_iban: e.target.value })} />
                  {supplierForm.qr_iban && !estQRIBAN(supplierForm.qr_iban) ? (
                    <p className="text-xs text-amber-700 mt-1">
                      {t('ach.qrIbanPasQR')}
                    </p>
                  ) : (
                    <p className="text-xs text-alpine-500 mt-1">
                      {t('ach.qrIbanAide')}
                    </p>
                  )}
                </div>
                <div>
                  <label className="label" htmlFor="ns-mail">{t('ach.email')}</label>
                  <input id="ns-mail" type="email" className="input" value={supplierForm.email}
                         onChange={e => setSupplierForm({ ...supplierForm, email: e.target.value })} />
                </div>
                <div>
                  <label className="label" htmlFor="ns-uid">{t('ach.ide')}</label>
                  <input id="ns-uid" className="input font-mono" placeholder="CHE-000.000.000"
                         value={supplierForm.vat_number}
                         onChange={e => setSupplierForm({
                           ...supplierForm, vat_number: e.target.value })} />
                </div>
              </div>
              <div className="mt-3 flex items-center gap-2">
                <button type="button" onClick={() => createSupplier.mutate()}
                        disabled={supplierForm.name.trim() === '' || createSupplier.isPending}
                        className="btn-primary btn-sm flex items-center gap-1.5">
                  {createSupplier.isPending && <Loader2 size={13} className="animate-spin" />}
                  {t('ach.creerSelectionner')}
                </button>
                <button type="button" onClick={() => setNewSupplier(false)}
                        className="btn-ghost btn-sm">{t('action.annuler')}</button>
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div>
              <label className="label" htmlFor="sup">{t('ach.fournisseurObl')}</label>
              <select id="sup" className="select" value={form.supplier_id}
                      onChange={e => setForm({ ...form, supplier_id: e.target.value })}>
                <option value="">{t('ach.choisir')}</option>
                {/* Un QR-IBAN est rangé dans `qr_iban`, jamais dans `iban` :
                    ne lire que le second faisait annoncer « sans IBAN » un
                    fournisseur parfaitement payable — et l'ordre de virement
                    part bien, lui, puisque le serveur lit les deux champs.
                    L'écran démentait le produit. */}
                {supplierList.map(sp => (
                  <option key={sp.id} value={sp.id}>
                    {sp.name}{sp.iban || sp.qr_iban ? '' : ` ${t('ach.sansIban')}`}
                  </option>
                ))}
              </select>
              {/* Créer sur place. Renvoyer vers Contacts au milieu d'une saisie
                  fait perdre ce qui est déjà tapé, et la facture qu'on a sous
                  les yeux vient souvent d'un fournisseur qu'on n'a pas encore
                  enregistré. */}
              <button type="button" onClick={() => setNewSupplier(true)}
                      className="text-xs text-accent-700 hover:text-accent-800 mt-1
                                 flex items-center gap-1">
                <UserPlus size={12} /> {t('ach.nouveauFournisseur')}
              </button>
              {supplierList.length === 0 && (
                <p className="text-xs text-alpine-500 mt-1">
                  {t('ach.aucunFournisseur')}
                </p>
              )}
            </div>

            <div>
              <label className="label" htmlFor="ref">{t('ach.numeroObl')}</label>
              <input id="ref" className="input" placeholder={t('fact.colNumero')}
                     value={form.supplier_reference}
                     onChange={e => setForm({ ...form, supplier_reference: e.target.value })} />
              <p className="text-xs text-alpine-500 mt-1">{t('ach.numeroAide')}</p>
            </div>

            <div>
              <label className="label" htmlFor="issue">{t('ach.dateObl')}</label>
              <input id="issue" type="date" className="input" value={form.issue_date}
                     onChange={e => setForm({ ...form, issue_date: e.target.value })} />
            </div>

            <div>
              <label className="label" htmlFor="due">{t('doc.echeance')}</label>
              <input id="due" type="date" className="input" value={form.due_date}
                     onChange={e => setForm({ ...form, due_date: e.target.value })} />
            </div>

            <div className="sm:col-span-2">
              <label className="label" htmlFor="desc">{t('ach.objet')}</label>
              <input id="desc" className="input" placeholder={t('ach.objetExemple')}
                     value={form.description}
                     onChange={e => setForm({ ...form, description: e.target.value })} />
            </div>

            <div>
              <label className="label" htmlFor="amt">
                {t('ach.montantObl')}
              </label>
              <div className="flex gap-2">
                <input id="amt" type="number" step="0.05" min="0" inputMode="decimal"
                       className="input text-right font-mono tabular-nums"
                       value={form.amount}
                       onChange={e => setForm({ ...form, amount: e.target.value })} />
                {/* Une facture reçue annonce ce qu'il faut PAYER, et c'est aussi
                    ce que porte le QR. Le mode par défaut est donc TTC ; le hors
                    taxe se déduit du taux. */}
                <select className="select w-24" aria-label={t('ah.montantSaisiEst')}
                        value={form.amount_mode}
                        onChange={e => setForm({
                          ...form, amount_mode: e.target.value as 'ht' | 'ttc',
                        })}>
                  <option value="ttc">{t('tva.toutesTaxes')}</option>
                  <option value="ht">{t('tva.horsTaxe')}</option>
                </select>
              </div>
              <p className="text-xs text-alpine-500 mt-1">
                {form.amount_mode === 'ttc' ? t('ach.montantTTCAide') : t('ach.montantHTAide')}
              </p>
            </div>

            <div>
              <label className="label" htmlFor="rate">{t('tva.taux')}</label>
              {/* Les taux suisses sont fixés par la loi : les proposer en liste
                  supprime la faute de frappe — un 8.0 au lieu de 8.1 fausse la
                  déclaration et ne se voit qu'au décompte trimestriel. */}
              <select id="rate" className="select" value={form.vat_rate}
                      onChange={e => setForm({ ...form, vat_rate: e.target.value })}>
                {VAT_RATES.map(r => (
                  <option key={r.value} value={r.value}>{t(r.cle, { taux: r.value })}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="label" htmlFor="acct">{t('ach.compteCharge')}</label>
              <select id="acct" className="select" value={form.expense_account_code}
                      onChange={e => setForm({ ...form, expense_account_code: e.target.value })}>
                <option value="">{t('ach.compteParDefaut')}</option>
                {expenseAccounts.map(a => (
                  <option key={a.id} value={a.code}>{a.code} — {a.name}</option>
                ))}
              </select>
            </div>

            <div className="sm:col-span-2">
              <label className="label" htmlFor="payref">{t('paiement.reference')}</label>
              <input id="payref" className="input font-mono" placeholder={t('ach.refExemple')}
                     value={form.payment_reference}
                     onChange={e => setForm({ ...form, payment_reference: e.target.value })} />
              {/* Cette référence est ce qui permet au fournisseur de rapprocher
                  l'encaissement. Sans elle le virement part quand même, mais il
                  arrive anonyme — et la relance suit. */}
              <p className="text-xs text-alpine-500 mt-1">
                {t('ah.referenceAide')}
              </p>
            </div>
          </div>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t
                          border-alpine-100 pt-3">
            <div className="text-sm text-alpine-600">
              <p>
                {t('tva.horsTaxe')} <span className="font-mono">{formatCHF(isFinite(ht) ? ht : 0)}</span>
                {' · '}TVA <span className="font-mono">{formatCHF(tva)}</span>
                {' · '}<strong>TTC <span className="font-mono">{formatCHF(ttc)}</span></strong>
              </p>
              {/* Une facture sans TVA — fournisseur non assujetti — se saisit à
                  0 % : les deux montants sont alors égaux, et il n'y a pas
                  d'impôt préalable à déduire (LTVA art. 28 al. 1, qui exige une
                  facture mentionnant la TVA pour la récupérer). C'est conforme,
                  et le dire évite de chercher un taux qui n'existe pas. */}
              {rate === 0 && isFinite(ht) && ht > 0 && (
                <p className="text-xs text-alpine-500 mt-0.5">
                  {t('ach.sansTVA')}
                </p>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button onClick={() => {
                        setCreating(false); setEditing(null); setScan(null)
                        reset(); setError(null)
                      }}
                      className="btn-secondary btn-sm">{t('action.annuler')}</button>
              <button onClick={() => { setError(null); create.mutate() }} disabled={!canCreate}
                      className="btn-primary btn-sm flex items-center gap-1.5">
                {create.isPending && <Loader2 size={13} className="animate-spin" />}
                {editing ? t('ach.enregistrerModifs') : t('ach.enregistrerBrouillon')}
              </button>
            </div>
          </div>
        </div>
      )}

      <SectionTitle>{t('ach.facturesRecues')}</SectionTitle>
      <div className="table-wrapper mb-8">
        <table className="table">
          <thead>
            <tr>
              <th>{t('doc.fournisseur')}</th>
              <th>{t('ach.numeroCol')}</th>
              <th>{t('fact.colDate')}</th>
              <th>{t('doc.echeance')}</th>
              <th className="text-right">{t('tva.horsTaxe')}</th>
              <th className="text-right">{t('tva.tva')}</th>
              <th className="text-right">{t('tva.toutesTaxes')}</th>
              <th>{t('fact.colStatut')}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {list.isLoading && <tr><td colSpan={9}><LoadingSpinner /></td></tr>}
            {list.isError && (
              <tr><td colSpan={9}>
                <ErrorBanner message="Les factures fournisseurs n'ont pas pu être lues." />
              </td></tr>
            )}
            {!list.isLoading && !list.isError && items.length === 0 && (
              <tr><td colSpan={9}>
                <EmptyState
                  icon={<FileText size={28} />}
                  title={t('ach.aucuneFacture')}
                  description={t('ach.aucuneFactureAide')}
                />
              </td></tr>
            )}
            {items.map(i => (
              <tr key={i.id}>
                <td className="text-alpine-800">{i.supplier_name}</td>
                <td className="font-mono text-xs text-accent-700">{i.supplier_reference}</td>
                <td>{formatDate(i.issue_date)}</td>
                <td>{i.due_date ? formatDate(i.due_date) : '—'}</td>
                <td className="text-right font-mono tabular-nums">{formatCHF(i.subtotal_amount)}</td>
                <td className="text-right font-mono tabular-nums text-alpine-500">
                  {formatCHF(i.vat_amount)}
                </td>
                <td className="text-right font-mono tabular-nums font-medium">
                  {formatCHF(i.total_amount)}
                </td>
                <td>
                  <span className={`badge ${STATUS_CLASS[i.status] ?? 'badge-draft'}`}>
                    {t(STATUS_CLE[i.status] ?? 'statut.brouillon')}
                  </span>
                </td>
                <td className="text-right">
                  {/* Un brouillon se corrige ; une facture comptabilisée porte
                      une écriture scellée (CO art. 957a) et ne se retouche pas. */}
                  {i.status === 'draft' && peutEcrire && (
                    <div className="flex items-center justify-end gap-1">
                      <button onClick={() => openEdit(i)}
                              className="btn-ghost btn-sm flex items-center gap-1">
                        <Pencil size={13} /> {t('action.modifier')}
                      </button>
                      <button onClick={() => setToBook(i)} disabled={book.isPending}
                              className="btn-ghost btn-sm text-success-700 flex items-center gap-1">
                        <CheckCircle size={13} /> {t('ach.comptabiliser')}
                      </button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <PaymentRunPanel />

      <ConfirmDialog
        open={toBook !== null}
        title={t('ach.confirmTitre', { ref: toBook?.supplier_reference ?? '' })}
        // Sur une facture sans TVA — fournisseur non assujetti, opération
        // exclue — annoncer une « TVA déductible » et un report au chiffre 400
        // décrirait une écriture qui ne sera pas passée. Un dialogue de
        // confirmation qui décrit autre chose que ce qui va se produire ne
        // protège plus de rien : on le lit une fois, puis on l'ignore.
        consequences={
          (toBook?.vat_amount ?? 0) > 0
            ? [
                t('ach.confirmScellee'),
                t('ach.confirmDeclaration'),
                t('ach.confirmPayable'),
              ]
            : [
                t('ach.confirmScelleeSansTVA'),
                t('ach.confirmSansTVA'),
                t('ach.confirmPayable'),
              ]
        }
        reassurance={t('ach.confirmPasPayee')}
        confirmLabel={t('ach.comptabiliser')}
        tone="danger"
        busy={book.isPending}
        onConfirm={() => toBook && book.mutate(toBook.id)}
        onCancel={() => setToBook(null)}
      />
    </div>
  )
}
