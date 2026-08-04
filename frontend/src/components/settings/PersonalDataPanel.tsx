// LedgerAlps — Données personnelles (Paramètres → Maintenance)
//
// La nLPD art. 6 al. 4 impose de détruire ou d'anonymiser une donnée
// personnelle dès qu'elle n'est plus nécessaire ; son art. 32 ouvre un droit à
// l'effacement. Le CO art. 958f impose en parallèle dix ans de conservation.
//
// Ce n'est pas contradictoire : ce que la loi commerciale protège est la
// PIÈCE, pas la fiche client. Cet écran réunit les deux faces — ce que la
// rétention efface d'elle-même, et ce que vous pouvez effacer sur demande.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, AlertTriangle } from 'lucide-react'
import { contactsApi, maintenanceApi } from '@/api/client'
import { SectionTitle, ErrorBanner } from '@/components/ui'
import type { Contact, SystemHealth, AnonymiseResult } from '@/types'

// ─── Données personnelles (nLPD) ──────────────────────────────────────────────

export function PersonalDataPanel() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const health = useQuery<SystemHealth>({
    queryKey: ['maintenance', 'health'],
    queryFn: () => maintenanceApi.health().then(r => r.data),
  })
  const contacts = useQuery<Contact[]>({
    queryKey: ['contacts', 'all'],
    queryFn: () => contactsApi.list({ page_size: 500 }).then(r => r.data.items ?? []),
  })

  const anonymise = useMutation<AnonymiseResult>({
    mutationFn: () => contactsApi.anonymise(selected).then(r => r.data),
    onSuccess: () => {
      setConfirming(false); setError(null); setSelected('')
      qc.invalidateQueries({ queryKey: ['contacts'] })
      qc.invalidateQueries({ queryKey: ['maintenance'] })
    },
    onError: (e: any) => setError(e?.response?.data?.error ?? "L'anonymisation a échoué."),
  })

  const pd = health.data?.personal_data
  const chosen = contacts.data?.find(c => c.id === selected)

  return (
    <div>
      <SectionTitle>Données personnelles (nLPD)</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        La nLPD art. 6 al. 4 impose de détruire ou d'anonymiser une donnée personnelle
        dès qu'elle n'est plus nécessaire, et son art. 32 permet à une personne d'en
        demander l'effacement. Le CO art. 958f impose en parallèle de conserver vos
        pièces dix ans. Ce n'est pas contradictoire : ce que la loi commerciale protège,
        c'est la <strong>pièce</strong>, pas la fiche client.
      </p>

      {/* Ce que la rétention fait réellement, en chiffres. Annoncer une durée
          sans montrer qu'elle s'applique est le défaut qu'on vient de corriger. */}
      {pd && (
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm mb-3">
          <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">
            Rétention appliquée à chaque démarrage
          </p>
          <ul className="space-y-1 text-alpine-700">
            <li>
              Les adresses IP des tentatives de connexion bloquées sont anonymisées après{' '}
              <strong>{pd.ip_retention_days} jours</strong>, et l'événement supprimé après{' '}
              <strong>{pd.event_retention_days} jours</strong>.
            </li>
            <li className="tabular-nums">
              {pd.security_events ?? 0} événement(s) de sécurité conservé(s), dont{' '}
              <strong>{pd.ip_addresses_held ?? 0}</strong> portent encore une adresse IP.
            </li>
            {(pd.contacts_anonymised ?? 0) > 0 && (
              <li className="tabular-nums">
                {pd.contacts_anonymised} contact(s) anonymisé(s).
              </li>
            )}
          </ul>
          {(pd.invoices_recipient_reconstructed ?? 0) > 0 && (
            <p className="text-warning-700 mt-2">
              <strong>{pd.invoices_recipient_reconstructed} facture(s)</strong> portent une
              identité de destinataire <em>reconstituée</em> depuis la fiche contact, faute
              d'instantané d'époque : elles sont antérieures à la version 1.4.6, où le PDF
              relisait le contact vivant. Une reconstitution et une pièce d'origine ne se
              valent pas devant un réviseur, donc LedgerAlps les distingue plutôt que de
              les confondre.
            </p>
          )}
        </div>
      )}

      {/* Anonymisation */}
      <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
        <p className="font-medium">Anonymiser un contact</p>
        <p className="text-alpine-600 mt-0.5 mb-2">
          Efface nom, adresse, courriel, téléphone, IBAN et numéro de TVA de la fiche.
          Les factures déjà émises gardent l'identité qu'elles portaient à l'émission —
          la LTVA art. 26 exige qu'une facture nomme son destinataire.
        </p>

        <div className="flex flex-wrap items-center gap-2">
          <select
            className="input flex-1 min-w-[16rem]"
            value={selected}
            onChange={e => { setSelected(e.target.value); setConfirming(false); setError(null) }}
          >
            <option value="">Choisir un contact…</option>
            {(contacts.data ?? []).filter(c => c.is_active).map(c => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          <button
            onClick={() => setConfirming(true)}
            disabled={!selected || confirming}
            className="btn-secondary btn-sm"
          >
            Anonymiser
          </button>
        </div>

        {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

        {confirming && chosen && (
          <div className="mt-3 rounded-md border border-danger-500 bg-danger-100 px-3 py-2.5">
            <div className="flex items-start gap-2">
              <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-danger-700" />
              <div className="flex-1">
                <p className="font-medium text-danger-700">
                  Anonymiser « {chosen.name} » ?
                </p>
                <p className="text-alpine-700 mt-1">
                  <strong>C'est irréversible</strong>, et c'est le but : c'est ce qui a été
                  promis à la personne concernée. Aucune copie de ces coordonnées n'est
                  conservée dans l'application.
                </p>
                <p className="text-alpine-700 mt-1">
                  Ses documents comptables, eux, sont conservés : le CO art. 958f l'impose
                  pendant dix ans.
                </p>
                <div className="mt-3 flex gap-2">
                  <button
                    onClick={() => anonymise.mutate()}
                    disabled={anonymise.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5"
                  >
                    {anonymise.isPending && <Loader2 size={13} className="animate-spin" />}
                    Anonymiser définitivement
                  </button>
                  <button onClick={() => setConfirming(false)} className="btn-ghost btn-sm">
                    Annuler
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {anonymise.data && (
          <div className="mt-3 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5">
            <p className="font-medium text-success-700">
              {anonymise.data.label} — anonymisé
            </p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-2 text-alpine-700">
              <div>
                <p className="text-xs uppercase tracking-wider text-alpine-500 mb-1">Effacé</p>
                <ul className="list-disc list-inside space-y-0.5">
                  {anonymise.data.what_was_erased.map(x => <li key={x}>{x}</li>)}
                </ul>
              </div>
              <div>
                <p className="text-xs uppercase tracking-wider text-alpine-500 mb-1">Conservé</p>
                <ul className="list-disc list-inside space-y-0.5">
                  {anonymise.data.what_was_kept.map(x => <li key={x}>{x}</li>)}
                </ul>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
