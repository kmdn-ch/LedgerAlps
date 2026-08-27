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
import { useT } from '@/i18n/useT'
import { refusalMessage } from '@/utils/refusal'

// ─── Données personnelles (nLPD) ──────────────────────────────────────────────

export function PersonalDataPanel() {
  const t = useT()
  const qc = useQueryClient()
  const [selected, setSelected] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const health = useQuery<SystemHealth>({
    queryKey: ['maintenance', 'health'],
    queryFn: () => maintenanceApi.health().then(r => r.data),
  })
  // GET /contacts renvoie un tableau JSON nu, pas un objet paginé — comme tous
  // les autres écrans qui l'appellent. Lire `r.data.items` donnait `undefined`,
  // donc une liste vide et un sélecteur sans aucun contact.
  const contacts = useQuery<Contact[]>({
    queryKey: ['contacts'],
    queryFn: () => contactsApi.list().then(r => r.data as Contact[]),
  })

  const anonymise = useMutation<AnonymiseResult>({
    mutationFn: () => contactsApi.anonymise(selected).then(r => r.data),
    onSuccess: () => {
      setConfirming(false); setError(null); setSelected('')
      qc.invalidateQueries({ queryKey: ['contacts'] })
      qc.invalidateQueries({ queryKey: ['maintenance'] })
    },
    onError: (e) => setError(refusalMessage(e, t('dp.echecAnonymisation'))),
  })

  const pd = health.data?.personal_data
  const chosen = contacts.data?.find(c => c.id === selected)

  return (
    <div>
      <SectionTitle>{t('dp.titre')}</SectionTitle>
      <p className="text-sm text-alpine-600 mb-3">
        {t('dp.introduction')}
      </p>

      {/* Ce que la rétention fait réellement, en chiffres. Annoncer une durée
          sans montrer qu'elle s'applique est le défaut qu'on vient de corriger. */}
      {pd && (
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm mb-3">
          <p className="text-alpine-500 text-xs uppercase tracking-wider mb-1.5">
            {t('dp.retentionTitre')}
          </p>
          <ul className="space-y-1 text-alpine-700">
            <li>
              {t('dp.retentionIP', {
                ip: pd.ip_retention_days, ev: pd.event_retention_days })}
            </li>
            <li className="tabular-nums">
              {t('dp.evenementsConserves', {
                n: pd.security_events ?? 0, ip: pd.ip_addresses_held ?? 0 })}
            </li>
            {(pd.contacts_anonymised ?? 0) > 0 && (
              <li className="tabular-nums">
                {t('dp.contactsAnonymises', { n: pd.contacts_anonymised ?? 0 })}
              </li>
            )}
          </ul>
          {(pd.invoices_recipient_reconstructed ?? 0) > 0 && (
            <p className="text-warning-700 mt-2">
              {t('dp.destinataireReconstitue', {
                n: pd.invoices_recipient_reconstructed ?? 0 })}
            </p>
          )}
        </div>
      )}

      {/* Anonymisation */}
      <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
        <p className="font-medium">{t('dp.anonymiserContact')}</p>
        <p className="text-alpine-600 mt-0.5 mb-2">
          {t('dp.anonymiserAide')}
        </p>

        <div className="flex flex-wrap items-center gap-2">
          <select
            className="input flex-1 min-w-[16rem]"
            value={selected}
            onChange={e => { setSelected(e.target.value); setConfirming(false); setError(null) }}
          >
            <option value="">{t('dp.choisirContact')}</option>
            {(contacts.data ?? []).filter(c => c.is_active).map(c => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          <button
            onClick={() => setConfirming(true)}
            disabled={!selected || confirming}
            className="btn-secondary btn-sm"
          >
            {t('dp.anonymiser')}
          </button>
        </div>

        {error && <div className="mt-3"><ErrorBanner message={error} /></div>}

        {confirming && chosen && (
          <div className="mt-3 rounded-md border border-danger-500 bg-danger-100 px-3 py-2.5">
            <div className="flex items-start gap-2">
              <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-danger-700" />
              <div className="flex-1">
                <p className="font-medium text-danger-700">
                  {t('dp.confirmerTitre', { nom: chosen.name })}
                </p>
                <p className="text-alpine-700 mt-1">
                  {t('dp.irreversible')}
                </p>
                <p className="text-alpine-700 mt-1">
                  {t('dp.documentsConserves')}
                </p>
                <p className="text-alpine-700 mt-1">
                  {t('dp.sauvegardesGardent')}
                </p>
                <div className="mt-3 flex gap-2">
                  <button
                    onClick={() => anonymise.mutate()}
                    disabled={anonymise.isPending}
                    className="btn-primary btn-sm flex items-center gap-1.5"
                  >
                    {anonymise.isPending && <Loader2 size={13} className="animate-spin" />}
                    {t('dp.anonymiserDefinitivement')}
                  </button>
                  <button onClick={() => setConfirming(false)} className="btn-ghost btn-sm">
                    {t('action.annuler')}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {anonymise.data && (
          <div className="mt-3 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5">
            <p className="font-medium text-success-700">
              {t('dp.anonymise', { label: anonymise.data.label })}
            </p>
            {anonymise.data.backups_notice && (
              <p className="mt-2 text-alpine-700">{anonymise.data.backups_notice}</p>
            )}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-2 text-alpine-700">
              <div>
                <p className="text-xs uppercase tracking-wider text-alpine-500 mb-1">{t('dp.efface')}</p>
                <ul className="list-disc list-inside space-y-0.5">
                  {anonymise.data.what_was_erased.map(x => <li key={x}>{x}</li>)}
                </ul>
              </div>
              <div>
                <p className="text-xs uppercase tracking-wider text-alpine-500 mb-1">{t('dp.conserve')}</p>
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
