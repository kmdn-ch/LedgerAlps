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
import { Plus, Loader2, FileText, CheckCircle, UserPlus, X } from 'lucide-react'
import { supplierInvoicesApi, contactsApi, accountsApi } from '@/api/client'
import {
  PageHeader, LoadingSpinner, EmptyState, ErrorBanner, SectionTitle, ConfirmDialog,
} from '@/components/ui'
import { formatCHF, formatDate } from '@/utils'
import { refusalMessage } from '@/utils/refusal'
import { useUnsavedGuard } from '@/hooks/useUnsavedGuard'
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

const STATUS_LABEL: Record<string, string> = {
  draft: 'Brouillon', booked: 'Comptabilisée', paid: 'Payée', cancelled: 'Annulée',
}
const STATUS_CLASS: Record<string, string> = {
  draft: 'badge-draft', booked: 'badge-sent', paid: 'badge-paid', cancelled: 'badge-cancelled',
}

// Les taux en vigueur depuis le 1er janvier 2024 (LTVA art. 25). Une liste
// fermée plutôt qu'un champ libre : le taux entre dans la déclaration, et une
// faute de frappe ne se découvre qu'au décompte trimestriel.
const VAT_RATES = [
  { value: '8.1', label: '8.1 % — taux normal' },
  { value: '2.6', label: '2.6 % — taux réduit (alimentation, livres, médicaments)' },
  { value: '3.8', label: '3.8 % — hébergement' },
  { value: '0',   label: '0 % — exonéré ou hors du champ' },
]

const today = () => new Date().toISOString().slice(0, 10)

export function PurchasesPage() {
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [toBook, setToBook] = useState<SupplierInvoice | null>(null)

  const [form, setForm] = useState({
    supplier_id: '', supplier_reference: '',
    issue_date: today(), due_date: '',
    description: '', amount_ht: '', vat_rate: '8.1',
    expense_account_code: '', payment_reference: '',
  })

  const [newSupplier, setNewSupplier] = useState(false)
  const [supplierForm, setSupplierForm] = useState({ name: '', iban: '', email: '' })

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

  const dirty = creating && (form.supplier_reference !== '' || form.amount_ht !== '' ||
    form.description !== '')
  useUnsavedGuard(dirty)

  const reset = () => setForm({
    supplier_id: '', supplier_reference: '', issue_date: today(), due_date: '',
    description: '', amount_ht: '', vat_rate: '8.1',
    expense_account_code: '', payment_reference: '',
  })

  const ht   = parseFloat(form.amount_ht.replace(',', '.'))
  const rate = parseFloat(form.vat_rate.replace(',', '.'))
  const tva  = isFinite(ht) && isFinite(rate) ? Math.round(ht * rate) / 100 : 0
  const ttc  = isFinite(ht) ? Math.round((ht + tva) * 20) / 20 : 0

  const create = useMutation({
    mutationFn: () => supplierInvoicesApi.create({
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
    }),
    onSuccess: () => {
      setError(null); setCreating(false); reset()
      qc.invalidateQueries({ queryKey: ['supplier-invoices'] })
    },
    onError: (e) => setError(refusalMessage(e, "La facture n'a pas pu être enregistrée.")),
  })

  // Le fournisseur créé ici est immédiatement sélectionné : la saisie reprend
  // là où elle s'est arrêtée, sans que l'on ait à rouvrir la liste.
  const createSupplier = useMutation({
    mutationFn: () => contactsApi.create({
      name: supplierForm.name.trim(),
      contact_type: 'supplier',
      email: supplierForm.email.trim() || undefined,
      iban: supplierForm.iban.trim() || undefined,
      country: 'CH',
    }),
    onSuccess: async (r) => {
      setError(null); setNewSupplier(false)
      setSupplierForm({ name: '', iban: '', email: '' })
      await qc.invalidateQueries({ queryKey: ['contacts'] })
      const created = r.data as { id?: string }
      if (created.id) setForm(f => ({ ...f, supplier_id: created.id as string }))
    },
    onError: (e) => setError(refusalMessage(e, "Le fournisseur n'a pas pu être créé.")),
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
        title="Achats"
        subtitle="Factures fournisseurs et ordres de paiement"
        actions={
          <button onClick={() => { setCreating(v => !v); setError(null) }} className="btn-primary">
            <Plus size={15} /> Saisir une facture
          </button>
        }
      />

      {error && <div className="mb-4"><ErrorBanner message={error} /></div>}

      {creating && (
        <div className="card card-pad mb-5">
          <SectionTitle>Nouvelle facture fournisseur</SectionTitle>

          {/* Création à la volée : un fournisseur inconnu ne doit pas obliger à
              quitter la page et perdre la saisie en cours. */}
          {newSupplier && (
            <div className="mb-4 rounded-md border border-accent-700 bg-accent-100/30 px-4 py-3">
              <div className="flex items-center justify-between mb-2">
                <p className="text-sm font-medium">Nouveau fournisseur</p>
                <button type="button" onClick={() => setNewSupplier(false)}
                        className="text-alpine-500 hover:text-alpine-700" aria-label="Fermer">
                  <X size={15} />
                </button>
              </div>
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div>
                  <label className="label" htmlFor="ns-name">Nom *</label>
                  <input id="ns-name" className="input" value={supplierForm.name}
                         onChange={e => setSupplierForm({ ...supplierForm, name: e.target.value })} />
                </div>
                <div>
                  <label className="label" htmlFor="ns-iban">IBAN</label>
                  <input id="ns-iban" className="input font-mono" placeholder="CH.."
                         value={supplierForm.iban}
                         onChange={e => setSupplierForm({ ...supplierForm, iban: e.target.value })} />
                  <p className="text-xs text-alpine-500 mt-1">
                    Sans lui, ses factures ne pourront pas être payées.
                  </p>
                </div>
                <div>
                  <label className="label" htmlFor="ns-mail">E-mail</label>
                  <input id="ns-mail" type="email" className="input" value={supplierForm.email}
                         onChange={e => setSupplierForm({ ...supplierForm, email: e.target.value })} />
                </div>
              </div>
              <div className="mt-3 flex items-center gap-2">
                <button type="button" onClick={() => createSupplier.mutate()}
                        disabled={supplierForm.name.trim() === '' || createSupplier.isPending}
                        className="btn-primary btn-sm flex items-center gap-1.5">
                  {createSupplier.isPending && <Loader2 size={13} className="animate-spin" />}
                  Créer et sélectionner
                </button>
                <button type="button" onClick={() => setNewSupplier(false)}
                        className="btn-ghost btn-sm">Annuler</button>
              </div>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div>
              <label className="label" htmlFor="sup">Fournisseur *</label>
              <select id="sup" className="select" value={form.supplier_id}
                      onChange={e => setForm({ ...form, supplier_id: e.target.value })}>
                <option value="">Choisir…</option>
                {supplierList.map(sp => (
                  <option key={sp.id} value={sp.id}>
                    {sp.name}{sp.iban ? '' : '  (sans IBAN)'}
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
                <UserPlus size={12} /> Nouveau fournisseur
              </button>
              {supplierList.length === 0 && (
                <p className="text-xs text-alpine-500 mt-1">
                  Aucun fournisseur enregistré pour l&rsquo;instant.
                </p>
              )}
            </div>

            <div>
              <label className="label" htmlFor="ref">N° de la facture *</label>
              <input id="ref" className="input" placeholder="FA-2026-118"
                     value={form.supplier_reference}
                     onChange={e => setForm({ ...form, supplier_reference: e.target.value })} />
              <p className="text-xs text-alpine-500 mt-1">Tel qu&rsquo;imprimé par le fournisseur.</p>
            </div>

            <div>
              <label className="label" htmlFor="issue">Date de la facture *</label>
              <input id="issue" type="date" className="input" value={form.issue_date}
                     onChange={e => setForm({ ...form, issue_date: e.target.value })} />
            </div>

            <div>
              <label className="label" htmlFor="due">Échéance</label>
              <input id="due" type="date" className="input" value={form.due_date}
                     onChange={e => setForm({ ...form, due_date: e.target.value })} />
            </div>

            <div className="sm:col-span-2">
              <label className="label" htmlFor="desc">Objet</label>
              <input id="desc" className="input" placeholder="Fournitures, abonnement, sous-traitance…"
                     value={form.description}
                     onChange={e => setForm({ ...form, description: e.target.value })} />
            </div>

            <div>
              <label className="label" htmlFor="ht">Montant hors taxe *</label>
              <input id="ht" type="number" step="0.05" min="0" inputMode="decimal"
                     className="input text-right font-mono tabular-nums"
                     value={form.amount_ht}
                     onChange={e => setForm({ ...form, amount_ht: e.target.value })} />
            </div>

            <div>
              <label className="label" htmlFor="rate">Taux de TVA</label>
              {/* Les taux suisses sont fixés par la loi : les proposer en liste
                  supprime la faute de frappe — un 8.0 au lieu de 8.1 fausse la
                  déclaration et ne se voit qu'au décompte trimestriel. */}
              <select id="rate" className="select" value={form.vat_rate}
                      onChange={e => setForm({ ...form, vat_rate: e.target.value })}>
                {VAT_RATES.map(r => (
                  <option key={r.value} value={r.value}>{r.label}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="label" htmlFor="acct">Compte de charge</label>
              <select id="acct" className="select" value={form.expense_account_code}
                      onChange={e => setForm({ ...form, expense_account_code: e.target.value })}>
                <option value="">6500 — Charges d&rsquo;administration (par défaut)</option>
                {expenseAccounts.map(a => (
                  <option key={a.id} value={a.code}>{a.code} — {a.name}</option>
                ))}
              </select>
            </div>

            <div className="sm:col-span-2">
              <label className="label" htmlFor="payref">Référence de paiement</label>
              <input id="payref" className="input font-mono" placeholder="27 chiffres, ou RF…"
                     value={form.payment_reference}
                     onChange={e => setForm({ ...form, payment_reference: e.target.value })} />
              {/* Cette référence est ce qui permet au fournisseur de rapprocher
                  l'encaissement. Sans elle le virement part quand même, mais il
                  arrive anonyme — et la relance suit. */}
              <p className="text-xs text-alpine-500 mt-1">
                Celle du bulletin de versement, pas le n° de facture. Elle voyagera dans l&rsquo;ordre
                de virement pour que le fournisseur reconnaisse votre paiement.
              </p>
            </div>
          </div>

          <div className="mt-4 flex flex-wrap items-center justify-between gap-3 border-t
                          border-alpine-100 pt-3">
            <p className="text-sm text-alpine-600">
              Hors taxe <span className="font-mono">{formatCHF(isFinite(ht) ? ht : 0)}</span>
              {' · '}TVA <span className="font-mono">{formatCHF(tva)}</span>
              {' · '}<strong>TTC <span className="font-mono">{formatCHF(ttc)}</span></strong>
            </p>
            <div className="flex items-center gap-2">
              <button onClick={() => { setCreating(false); reset(); setError(null) }}
                      className="btn-secondary btn-sm">Annuler</button>
              <button onClick={() => { setError(null); create.mutate() }} disabled={!canCreate}
                      className="btn-primary btn-sm flex items-center gap-1.5">
                {create.isPending && <Loader2 size={13} className="animate-spin" />}
                Enregistrer le brouillon
              </button>
            </div>
          </div>
        </div>
      )}

      <SectionTitle>Factures reçues</SectionTitle>
      <div className="table-wrapper mb-8">
        <table className="table">
          <thead>
            <tr>
              <th>Fournisseur</th>
              <th>N° facture</th>
              <th>Date</th>
              <th>Échéance</th>
              <th className="text-right">HT</th>
              <th className="text-right">TVA</th>
              <th className="text-right">TTC</th>
              <th>Statut</th>
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
                  title="Aucune facture fournisseur"
                  description="Saisissez ce que vous devez : la TVA payée à vos fournisseurs se déduit de celle que vous encaissez, et la charge entre dans votre résultat."
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
                    {STATUS_LABEL[i.status] ?? i.status}
                  </span>
                </td>
                <td className="text-right">
                  {i.status === 'draft' && (
                    <button onClick={() => setToBook(i)} disabled={book.isPending}
                            className="btn-ghost btn-sm text-success-700 flex items-center gap-1 ml-auto">
                      <CheckCircle size={13} /> Comptabiliser
                    </button>
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
        title={`Comptabiliser la facture ${toBook?.supplier_reference ?? ''} ?`}
        consequences={[
          <>L&rsquo;écriture est passée et <strong>scellée</strong> : charge et TVA déductible
             au débit, créanciers au crédit.</>,
          <>La TVA payée entre dans votre déclaration (impôt préalable, chiffre 400).</>,
          <>La facture devient payable et apparaît dans l&rsquo;ordre de paiement.</>,
        ]}
        reassurance="Elle ne devient pas « payée » pour autant : c'est le relevé bancaire qui l'établira."
        confirmLabel="Comptabiliser"
        tone="danger"
        busy={book.isPending}
        onConfirm={() => toBook && book.mutate(toBook.id)}
        onCancel={() => setToBook(null)}
      />
    </div>
  )
}
