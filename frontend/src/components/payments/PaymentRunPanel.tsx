// Ordre de paiement des factures fournisseurs — pain.001.
//
// Cet écran remplace une carte qui décrivait la fonction sans la fournir :
// « exportez via l'API POST /api/v1/payments/export ». Autant dire qu'elle
// n'existait pas — elle était réservée à qui sait forger une requête HTTP.
//
// # Ce qui part à la banque ne vient PAS d'ici
//
// La sélection ne transmet que des identifiants de factures. Le créancier,
// l'IBAN, le montant et la référence sont relus par le serveur dans les livres.
// Les montants affichés ci-dessous ne sont donc qu'un compte rendu : les
// modifier dans le navigateur ne changerait pas un centime du fichier produit.
//
// # Générer n'est pas payer
//
// Aucun statut ne bouge. La facture reste « comptabilisée » jusqu'à ce que le
// débit apparaisse au relevé bancaire — c'est le rapprochement camt.053 qui
// l'établit. L'écran le dit, parce qu'un bouton « Générer » suivi d'un silence
// laisse croire que l'affaire est réglée.

import { useMemo, useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  Download, AlertTriangle, Loader2, Building2, CalendarClock, Info,
} from 'lucide-react'
import { paymentsApi } from '@/api/client'
import { LoadingSpinner, EmptyState, ErrorBanner, SectionTitle } from '@/components/ui'
import { formatCHF, formatDate } from '@/utils'
import { refusalMessage } from '@/utils/refusal'

interface Payable {
  id: string
  supplier_name: string
  reference: string
  amount: number
  currency: string
  due_date: string
  days_late: number
  iban: string
  is_qr_iban: boolean
  payment_reference: string
  reference_type: string
  blocked_reason: string
}

interface PayableResponse {
  debtor: { name: string; iban: string; problem: string }
  items: Payable[]
}

export function PaymentRunPanel() {
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [execDate, setExecDate] = useState(() => new Date().toISOString().slice(0, 10))
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState<{ count: number; total: number } | null>(null)

  const payable = useQuery<PayableResponse>({
    queryKey: ['payments-payable'],
    queryFn:  () => paymentsApi.payable().then(r => r.data),
  })

  const items = payable.data?.items ?? []
  const debtor = payable.data?.debtor
  const payables = items.filter(i => i.blocked_reason === '')
  const blocked  = items.filter(i => i.blocked_reason !== '')

  const chosen = useMemo(
    () => payables.filter(i => selected.has(i.id)),
    [payables, selected],
  )
  const total = chosen.reduce((s, i) => s + i.amount, 0)

  const toggle = (id: string) =>
    setSelected(s => {
      const n = new Set(s)
      if (n.has(id)) { n.delete(id) } else { n.add(id) }
      return n
    })

  const generate = useMutation({
    mutationFn: async () => {
      const res = await paymentsApi.exportRun(execDate, chosen.map(i => i.id))
      // Le navigateur télécharge le XML. Rien n'est envoyé nulle part : le
      // fichier se dépose ensuite dans l'e-banking, à la main. Ce détour est
      // volontaire — LedgerAlps ne parle à aucun service extérieur.
      const url = URL.createObjectURL(new Blob([res.data], { type: 'application/xml' }))
      const a = document.createElement('a')
      a.href = url
      a.download = `paiements-${execDate}.xml`
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
      return { count: chosen.length, total }
    },
    onSuccess: (r) => { setError(null); setDone(r); setSelected(new Set()) },
    onError: (e) => {
      setDone(null)
      setError(refusalMessage(e, "Le fichier de paiement n'a pas pu être produit."))
    },
  })

  const bloque = debtor?.problem ?? ''
  const peutGenerer = chosen.length > 0 && bloque === '' && !generate.isPending

  return (
    <div>
      <SectionTitle>Paiements fournisseurs — ISO 20022 pain.001</SectionTitle>
      <p className="text-sm text-alpine-600 mb-4">
        Sélectionnez les factures à régler : LedgerAlps produit un fichier XML que vous déposez
        dans votre e-banking. Compatible UBS, PostFinance, Raiffeisen et Banques cantonales.
      </p>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}

      {/* L'entreprise qui paie. Rien à saisir : c'est la fiche société. */}
      {debtor && (
        <div className={`mb-4 rounded-md border px-4 py-3 text-sm ${
          bloque ? 'border-danger-500 bg-danger-500/5' : 'border-neutral-200 bg-neutral-50'
        }`}>
          <p className="flex items-center gap-2 font-medium">
            <Building2 size={15} /> Compte à débiter
          </p>
          {bloque
            ? <p className="text-danger-700 mt-1">{bloque}</p>
            : <p className="text-alpine-700 mt-1">
                {debtor.name} — <span className="font-mono">{debtor.iban}</span>
              </p>}
        </div>
      )}

      {payable.isLoading && <LoadingSpinner />}
      {payable.isError && <ErrorBanner message="La liste des factures à payer n'a pas pu être lue." />}

      {!payable.isLoading && items.length === 0 && (
        <EmptyState
          title="Aucune facture à payer"
          description="Seules les factures fournisseurs comptabilisées apparaissent ici. Une facture encore au brouillon doit d'abord être comptabilisée : la charge doit exister dans les livres avant que la trésorerie bouge."
        />
      )}

      {payables.length > 0 && (
        <div className="table-wrapper">
          <table className="table">
            <thead>
              <tr>
                <th style={{ width: '38px' }}>
                  <input
                    type="checkbox"
                    aria-label="Tout sélectionner"
                    checked={chosen.length === payables.length && payables.length > 0}
                    onChange={e => setSelected(
                      e.target.checked ? new Set(payables.map(i => i.id)) : new Set())}
                  />
                </th>
                <th>Fournisseur</th>
                <th>Facture</th>
                <th>Échéance</th>
                <th>Référence</th>
                <th className="text-right">Montant</th>
              </tr>
            </thead>
            <tbody>
              {payables.map(i => (
                <tr key={i.id} className={selected.has(i.id) ? 'bg-accent-100/30' : ''}>
                  <td>
                    <input type="checkbox" checked={selected.has(i.id)}
                           aria-label={`Payer ${i.reference}`}
                           onChange={() => toggle(i.id)} />
                  </td>
                  <td className="text-alpine-800">
                    {i.supplier_name}
                    <span className="block text-xs text-alpine-500 font-mono">{i.iban}</span>
                  </td>
                  <td className="font-mono text-xs text-accent-700">{i.reference}</td>
                  <td>
                    {i.due_date ? formatDate(i.due_date) : '—'}
                    {i.days_late > 0 && (
                      <span className="block text-xs text-danger-700">
                        {i.days_late} jour{i.days_late > 1 ? 's' : ''} de retard
                      </span>
                    )}
                  </td>
                  <td className="text-xs">
                    {i.reference_type
                      ? <><span className="badge badge-sent">{i.reference_type}</span>
                          <span className="block font-mono text-alpine-500 mt-0.5">
                            {i.payment_reference}
                          </span></>
                      : <span className="text-alpine-400">motif en texte libre</span>}
                  </td>
                  <td className="text-right font-mono tabular-nums">
                    {formatCHF(i.amount)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Les factures qu'on ne PEUT pas payer, et pourquoi. Les masquer ferait
          disparaître une dette de l'écran sans que rien ne l'explique. */}
      {blocked.length > 0 && (
        <div className="mt-4 rounded-md border border-warning-500 bg-warning-100 px-4 py-3">
          <p className="text-sm font-medium flex items-center gap-1.5">
            <AlertTriangle size={15} />
            {blocked.length} facture{blocked.length > 1 ? 's' : ''} ne peu
            {blocked.length > 1 ? 'vent' : 't'} pas être payée{blocked.length > 1 ? 's' : ''}
          </p>
          <ul className="mt-2 space-y-1 text-sm text-alpine-700">
            {blocked.map(i => (
              <li key={i.id}>
                <span className="font-mono text-xs">{i.reference}</span> — {i.supplier_name} :
                {' '}{i.blocked_reason}
              </li>
            ))}
          </ul>
        </div>
      )}

      {payables.length > 0 && (
        <div className="mt-4 flex flex-wrap items-end justify-between gap-4">
          <div>
            <label className="label" htmlFor="exec-date">
              <CalendarClock size={13} className="inline mr-1" />
              Date d&rsquo;exécution souhaitée
            </label>
            <input id="exec-date" type="date" className="input w-52"
                   value={execDate} onChange={e => setExecDate(e.target.value)} />
            <p className="text-xs text-alpine-500 mt-1">
              La banque exécute à cette date, ou au premier jour ouvrable suivant.
            </p>
          </div>

          <div className="text-right">
            <p className="text-sm text-alpine-600">
              {chosen.length} facture{chosen.length > 1 ? 's' : ''} sélectionnée
              {chosen.length > 1 ? 's' : ''}
            </p>
            <p className="text-xl font-semibold font-mono tabular-nums text-alpine-900">
              {formatCHF(total)}
            </p>
            <button onClick={() => { setError(null); generate.mutate() }}
                    disabled={!peutGenerer}
                    className="btn-primary mt-2 flex items-center gap-1.5 ml-auto">
              {generate.isPending
                ? <Loader2 size={14} className="animate-spin" />
                : <Download size={15} />}
              Générer le fichier
            </button>
          </div>
        </div>
      )}

      {done && (
        <div className="mt-4 rounded-md border border-success-500 bg-success-500/5 px-4 py-3 text-sm">
          <p className="font-medium">
            Fichier produit — {done.count} virement{done.count > 1 ? 's' : ''},
            {' '}{formatCHF(done.total)}
          </p>
          <p className="text-alpine-700 mt-1 flex items-start gap-1.5">
            <Info size={14} className="mt-0.5 shrink-0" />
            <span>
              Déposez-le dans votre e-banking pour que les virements partent. Les factures restent
              <strong> comptabilisées</strong> et non « payées » : c&rsquo;est le relevé bancaire
              qui l&rsquo;établira, à l&rsquo;import camt.053. Rien n&rsquo;a été envoyé
              nulle part — LedgerAlps ne parle à aucun service extérieur.
            </span>
          </p>
        </div>
      )}
    </div>
  )
}
