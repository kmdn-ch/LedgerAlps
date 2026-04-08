// LedgerAlps — Gestion des contacts

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Plus, Building2, User, Search } from 'lucide-react'
import { contactsApi } from '@/api/client'
import { PageHeader, LoadingSpinner, EmptyState } from '@/components/ui'
import type { Contact } from '@/types'
import { NewContactModal } from '@/components/contact/NewContactModal'

export function ContactsPage() {
  const [search,     setSearch]     = useState('')
  const [showModal,  setShowModal]  = useState(false)
  const [typeFilter, setTypeFilter] = useState<'client' | 'supplier' | ''>('')

  const { data: contacts = [], isLoading } = useQuery<Contact[]>({
    queryKey: ['contacts', typeFilter],
    queryFn:  () => contactsApi.list(typeFilter ? { contact_type: typeFilter } : undefined).then(r => r.data),
  })

  const filtered = contacts.filter(c =>
    c.name.toLowerCase().includes(search.toLowerCase()) ||
    (c.email ?? '').toLowerCase().includes(search.toLowerCase())
  )

  return (
    <div>
      <PageHeader
        title="Contacts"
        subtitle={`${contacts.length} contact${contacts.length !== 1 ? 's' : ''}`}
        actions={
          <button className="btn-primary" onClick={() => setShowModal(true)}>
            <Plus size={15} /> Nouveau contact
          </button>
        }
      />

      {/* Filtres */}
      <div className="flex items-center gap-3 mb-5">
        <div className="relative">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-alpine-400" />
          <input
            className="input pl-8 w-56"
            placeholder="Rechercher…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>
        {(['', 'client', 'supplier'] as const).map(t => (
          <button
            key={t}
            onClick={() => setTypeFilter(t)}
            className={`px-3 py-1.5 rounded text-xs font-medium transition-all ${
              typeFilter === t
                ? 'bg-alpine-800 text-white'
                : 'bg-white border border-alpine-200 text-alpine-600 hover:bg-alpine-50'
            }`}
          >
            {t === '' ? 'Tous' : t === 'client' ? 'Clients' : 'Fournisseurs'}
          </button>
        ))}
      </div>

      {/* Grille cartes */}
      {isLoading && <LoadingSpinner />}
      {!isLoading && filtered.length === 0 && (
        <EmptyState
          title="Aucun contact"
          description="Ajoutez vos clients et fournisseurs."
          action={
            <button className="btn-primary btn-sm" onClick={() => setShowModal(true)}>
              <Plus size={13} /> Ajouter
            </button>
          }
        />
      )}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        {filtered.map(c => (
          <div key={c.id} className="card hover:shadow-md transition-shadow cursor-pointer p-5">
            <div className="flex items-start gap-3">
              <div className="w-9 h-9 rounded-lg bg-alpine-100 flex items-center
                              justify-center flex-shrink-0">
                {c.is_company
                  ? <Building2 size={16} className="text-alpine-600" />
                  : <User      size={16} className="text-alpine-600" />
                }
              </div>
              <div className="min-w-0">
                <div className="font-medium text-alpine-900 truncate">{c.name}</div>
                <div className="text-xs text-alpine-400 mt-0.5">
                  {c.city ? `${c.city}, ${c.country}` : c.country}
                </div>
              </div>
            </div>
            {c.email && (
              <div className="mt-3 text-xs text-alpine-500 truncate">{c.email}</div>
            )}
            <div className="mt-3 flex items-center justify-between">
              <span className={`badge ${c.contact_type === 'client' ? 'badge-sent' : 'badge-draft'}`}>
                {c.contact_type === 'client' ? 'Client' : 'Fournisseur'}
              </span>
              <span className="text-xs text-alpine-400">
                {c.payment_term_days}j
              </span>
            </div>
          </div>
        ))}
      </div>

      {showModal && <NewContactModal onClose={() => setShowModal(false)} />}
    </div>
  )
}
