// LedgerAlps — Liste des factures

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Download, Search, Filter } from 'lucide-react'
import { invoicesApi, contactsApi, downloadBlob } from '@/api/client'
import {
  PageHeader, StatusBadge, LoadingSpinner, EmptyState, ConfirmDialog,
} from '@/components/ui'
import { useCanWrite, RAISON_LECTURE_SEULE } from '@/hooks/usePermissions'
import { formatCHF, formatDate, isOverdue } from '@/utils'
import type { Invoice, DisplayStatus, Contact } from '@/types'
import { useNavigate } from 'react-router-dom'

const STATUS_FILTERS: { value: DisplayStatus | ''; label: string }[] = [
  { value: '',          label: 'Toutes'       },
  { value: 'draft',     label: 'Brouillons'   },
  { value: 'sent',      label: 'Envoyées'     },
  { value: 'paid',      label: 'Payées'       },
  { value: 'overdue',   label: 'En retard'    },
]

interface Props { mode?: 'invoice' | 'quote' }

export function InvoicesPage({ mode = 'invoice' }: Props) {
  const peutEcrire = useCanWrite()
  // Les deux vues du même registre. Le menu n'en propose plus qu'une entrée :
  // c'est ici qu'on bascule, là où l'on regarde déjà les documents.
  const navigate = useNavigate()

  const isQuote = mode === 'quote'
  // Le filtre accepte 'overdue' : c'est une question posée au serveur
  // (« envoyée et échue »), pas un statut qu'on écrirait.
  const [status,  setStatus]  = useState<DisplayStatus | ''>('')
  const [search,  setSearch]  = useState('')
  // Filtrer par client se fait côté serveur, pour que la pagination reste
  // juste : un filtre appliqué à la page affichée ne verrait pas les pièces
  // des pages suivantes.
  const [contactId, setContactId] = useState('')
  const qc = useQueryClient()

  const { data: contacts = [] } = useQuery<Contact[]>({
    queryKey: ['contacts'],
    queryFn:  () => contactsApi.list().then(r => (r.data.items ?? r.data ?? []) as Contact[]),
  })

  const { data: allItems = [], isLoading } = useQuery<Invoice[]>({
    queryKey: ['invoices', status, mode, contactId],
    queryFn:  () => invoicesApi.list({
      ...(status ? { status } : {}),
      ...(contactId ? { contact_id: contactId } : {}),
    }).then(r => (r.data.items ?? []) as Invoice[]),
  })

  // Client-side filter by document_type
  const invoices = allItems.filter(i =>
    isQuote ? i.document_type === 'quote' : i.document_type !== 'quote'
  )

  const downloadPDF = async (id: string, invoiceNumber: string) => {
    const resp = await invoicesApi.downloadPDF(id)
    downloadBlob(resp.data, `facture_${invoiceNumber}.pdf`)
  }

  const markPaid = useMutation({
    mutationFn: (id: string) =>
      invoicesApi.updateStatus(id, 'paid'),
    onSuccess: () => {
      setPendingPaid(null)
      qc.invalidateQueries({ queryKey: ['invoices'] })
    },
  })

  // Le raccourci « Payer » de la liste est le plus exposé de l'application :
  // il est aligné avec ceux des autres lignes, et un clic décalé encaisse la
  // mauvaise facture. Enregistrer un paiement passe une écriture au journal et
  // fige le document — il n'y a pas de bouton pour revenir en arrière.
  const [pendingPaid, setPendingPaid] = useState<Invoice | null>(null)

  const filtered = invoices.filter(i =>
    search === '' ||
    i.invoice_number.toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div>
      <ConfirmDialog
        open={pendingPaid !== null}
        title={pendingPaid ? `Marquer la facture ${pendingPaid.invoice_number} comme payée ?` : ''}
        consequences={[
          pendingPaid
            ? `${formatCHF(pendingPaid.total_amount)} encaissé${pendingPaid.contact_name ? ` de ${pendingPaid.contact_name}` : ''}, à la date d'aujourd'hui.`
            : '',
          'Le paiement est passé au journal (banque / débiteurs).',
          'La facture ne sera plus modifiable.',
        ]}
        reassurance="Si le montant reçu diffère, ouvrez la facture et enregistrez un paiement partiel."
        confirmLabel="Marquer payée"
        busy={markPaid.isPending}
        onConfirm={() => pendingPaid && markPaid.mutate(pendingPaid.id)}
        onCancel={() => setPendingPaid(null)}
      />

      <PageHeader
        title="Facturation"
        subtitle={`${invoices.length} document${invoices.length !== 1 ? 's' : ''}`}
        actions={
          peutEcrire ? (
            <Link to="/invoices/new" className="btn-primary">
              <Plus size={15} /> {isQuote ? 'Nouvelle offre' : 'Nouvelle facture'}
            </Link>
          ) : (
            <span className="text-xs text-alpine-500">{RAISON_LECTURE_SEULE}</span>
          )
        }
      />

      {/* Bascule Factures / Offres.
          Le menu de gauche ne porte plus qu'une entrée : les deux sont deux
          vues du même registre, et une offre acceptée devient une facture. */}
      <div className="inline-flex rounded-md border border-alpine-200 overflow-hidden mb-4">
        <button
          onClick={() => navigate('/invoices')}
          className={`px-3 py-1.5 text-sm ${
            isQuote ? 'text-alpine-600 hover:bg-alpine-50' : 'bg-alpine-900 text-white'
          }`}
        >
          Factures
        </button>
        <button
          onClick={() => navigate('/quotes')}
          className={`px-3 py-1.5 text-sm border-l border-alpine-200 ${
            isQuote ? 'bg-alpine-900 text-white' : 'text-alpine-600 hover:bg-alpine-50'
          }`}
        >
          Offres de prix
        </button>
      </div>

      {/* Filtres */}
      <div className="flex flex-wrap items-center gap-3 mb-5">
        <div className="relative">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-alpine-400" />
          <input
            className="input pl-8 w-56"
            placeholder="Rechercher un numéro…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>
        <div className="flex items-center gap-1">
          <Filter size={14} className="text-alpine-400" />
          {STATUS_FILTERS.map(f => (
            <button
              key={f.value}
              onClick={() => setStatus(f.value)}
              className={`px-3 py-1.5 rounded text-xs font-medium transition-all ${
                status === f.value
                  ? 'bg-alpine-800 text-white'
                  : 'bg-white border border-alpine-200 text-alpine-600 hover:bg-alpine-50'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>

        <select
          className="input w-56"
          value={contactId}
          onChange={e => setContactId(e.target.value)}
          aria-label={isQuote ? 'Filtrer les offres par contact' : 'Filtrer les factures par contact'}
        >
          <option value="">Tous les contacts</option>
          {contacts.map(ct => (
            <option key={ct.id} value={ct.id}>{ct.name}</option>
          ))}
        </select>
      </div>

      {/* Table */}
      <div className="table-wrapper">
        <table className="table">
          <thead>
            <tr>
              <th>Numéro</th>
              <th>Date</th>
              <th>Échéance</th>
              <th>Contact</th>
              <th className="text-right">Total CHF</th>
              <th>Statut</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {isLoading && (
              <tr><td colSpan={7}><LoadingSpinner /></td></tr>
            )}
            {!isLoading && filtered.length === 0 && (
              <tr>
                <td colSpan={7}>
                  <EmptyState
                    title={isQuote ? 'Aucune offre de prix' : 'Aucune facture'}
                    description={isQuote
                      ? 'Créez votre première offre de prix.'
                      : 'Créez votre première facture pour démarrer.'}
                    action={peutEcrire ? (
                      <Link to="/invoices/new" className="btn-primary btn-sm">
                        <Plus size={13} /> Créer
                      </Link>
                    ) : undefined}
                  />
                </td>
              </tr>
            )}
            {filtered.map(inv => (
              <tr key={inv.id}>
                <td>
                  <Link
                    to={`/invoices/${inv.id}`}
                    className="font-mono text-accent-700 hover:text-accent-600 font-medium"
                  >
                    {inv.invoice_number}
                  </Link>
                </td>
                <td className="text-alpine-600">{formatDate(inv.issue_date)}</td>
                <td className={`text-alpine-600 ${
                  isOverdue(inv) ? 'text-danger-700 font-medium' : ''
                }`}>
                  {formatDate(inv.due_date)}
                </td>
                <td className="text-alpine-700">
                  {inv.contact_name || <span className="text-alpine-400 italic">contact supprimé</span>}
                </td>
                <td className="text-right font-mono font-medium tabular-nums">
                  {formatCHF(inv.total_amount)}
                </td>
                <td><StatusBadge status={inv.status} overdue={isOverdue(inv)} /></td>
                <td>
                  <div className="flex items-center gap-1 justify-end">
                    {inv.status === 'sent' && (
                      <button
                        onClick={() => setPendingPaid(inv)}
                        className="btn-ghost btn-sm text-success-700"
                      >
                        Payer
                      </button>
                    )}
                    <button
                      onClick={() => downloadPDF(inv.id, inv.invoice_number)}
                      className="btn-ghost btn-sm"
                      title="Télécharger PDF"
                    >
                      <Download size={14} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
