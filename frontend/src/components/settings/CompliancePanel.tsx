// LedgerAlps — Conformité & clôture (Paramètres → Maintenance)
//
// Trois choses que la loi suisse attend d'un logiciel comptable et qui
// n'avaient aucun écran :
//
//   - l'exercice comptable, sans lequel une clôture n'a pas de périmètre ;
//   - la clôture elle-même, et le verrouillage qui la rend crédible
//     (CO art. 958f, Olico art. 3) : un exercice bouclé ne bouge plus ;
//   - de quoi PROUVER l'intégrité à un tiers — fiduciaire, réviseur, AFC —
//     ce que l'Olico art. 9 exige d'un support modifiable.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Lock, LockOpen, CalendarRange, Plus, Loader2, FileCheck2,
  Download, AlertTriangle, X,
} from 'lucide-react'
import { fiscalYearsApi, auditApi, exportApi, contactsApi, maintenanceApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { formatDate } from '@/utils'
import type { FiscalYear, Contact, SystemHealth, AnonymiseResult } from '@/types'

export function CompliancePanel() {
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [confirmClose, setConfirmClose] = useState<FiscalYear | null>(null)
  const [error, setError] = useState<string | null>(null)

  const years = useQuery<FiscalYear[]>({
    queryKey: ['fiscal-years'],
    queryFn: () => fiscalYearsApi.list().then(r => r.data.items ?? []),
  })

  const close = useMutation({
    mutationFn: (id: string) => fiscalYearsApi.close(id),
    onSuccess: () => {
      setConfirmClose(null); setError(null)
      qc.invalidateQueries({ queryKey: ['fiscal-years'] })
      qc.invalidateQueries({ queryKey: ['maintenance'] })
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
    },
    onError: (e: any) => setError(e?.response?.data?.error ?? "La clôture a échoué."),
  })

  return (
    <div className="space-y-6">
      {/* ── Exercices ───────────────────────────────────────────────────────── */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <SectionTitle>Exercices comptables</SectionTitle>
          <button
            onClick={() => setCreating(v => !v)}
            className="btn-ghost btn-sm flex items-center gap-1.5"
          >
            {creating ? <X size={13} /> : <Plus size={13} />}
            {creating ? 'Annuler' : 'Déclarer un exercice'}
          </button>
        </div>
        <p className="text-sm text-alpine-600 mb-3">
          Chaque écriture appartient à un exercice. Si aucun ne couvre sa date,
          LedgerAlps crée l'<strong>année civile</strong> correspondante. Pour un
          exercice décalé — juillet à juin, par exemple — déclarez-le ici{' '}
          <strong>avant</strong> d'y comptabiliser quoi que ce soit.
        </p>

        {creating && <CreateFiscalYearForm onDone={() => {
          setCreating(false)
          qc.invalidateQueries({ queryKey: ['fiscal-years'] })
        }} />}

        {years.isLoading && <LoadingSpinner />}
        {years.isError && <ErrorBanner message="Les exercices n'ont pas pu être lus." />}
        {error && <div className="mb-2"><ErrorBanner message={error} /></div>}

        {years.data && years.data.length === 0 && (
          <p className="text-sm text-alpine-500">
            Aucun exercice déclaré. Le premier sera créé automatiquement à votre
            première écriture.
          </p>
        )}

        {years.data && years.data.length > 0 && (
          <div className="space-y-2">
            {years.data.map(y => (
              <div key={y.id} className="flex items-center justify-between rounded-md border border-neutral-200 px-4 py-3 text-sm">
                <div className="flex items-center gap-2.5">
                  {y.is_closed
                    ? <Lock size={15} className="text-alpine-500 flex-shrink-0" />
                    : <LockOpen size={15} className="text-success-700 flex-shrink-0" />}
                  <div>
                    <p className="font-medium">{y.name}</p>
                    <p className="text-alpine-500 text-xs flex items-center gap-1">
                      <CalendarRange size={11} />
                      {formatDate(y.start_date)} — {formatDate(y.end_date)}
                    </p>
                  </div>
                </div>
                {y.is_closed
                  ? <span className="text-xs text-alpine-500">Clôturé — plus modifiable</span>
                  : (
                    <button
                      onClick={() => { setError(null); setConfirmClose(y) }}
                      className="btn-secondary btn-sm"
                    >
                      Clôturer
                    </button>
                  )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ── Confirmation de clôture ─────────────────────────────────────────── */}
      {confirmClose && (
        <div className="rounded-md border border-warning-500 bg-warning-100 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <AlertTriangle size={16} className="mt-0.5 flex-shrink-0 text-warning-700" />
            <div className="flex-1">
              <p className="font-medium text-warning-700">
                Clôturer l'exercice {confirmClose.name} ?
              </p>
              <ul className="mt-1.5 space-y-1 text-alpine-700 list-disc list-inside">
                <li>
                  Les soldes de produits et de charges sont virés au résultat par une
                  écriture de clôture, qui rejoint la chaîne d'intégrité comme les autres.
                </li>
                <li>
                  <strong>Plus aucune écriture ne pourra être passée ni comptabilisée
                  dans cet exercice</strong>, y compris antidatée. Une correction se passe
                  dans l'exercice ouvert (CO art. 958f, Olico art. 3).
                </li>
                <li>Cette opération ne s'annule pas depuis l'application.</li>
              </ul>
              <p className="mt-2 text-alpine-700">
                Les écritures encore en brouillon empêchent la clôture : comptabilisez-les
                ou supprimez-les d'abord.
              </p>
              <div className="mt-3 flex gap-2">
                <button
                  onClick={() => close.mutate(confirmClose.id)}
                  disabled={close.isPending}
                  className="btn-primary btn-sm flex items-center gap-1.5"
                >
                  {close.isPending && <Loader2 size={13} className="animate-spin" />}
                  Clôturer définitivement
                </button>
                <button onClick={() => setConfirmClose(null)} className="btn-ghost btn-sm">
                  Annuler
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      <PersonalDataSection />

      {/* ── Preuves à remettre à un tiers ───────────────────────────────────── */}
      <div>
        <SectionTitle>Attestation et archive</SectionTitle>
        <p className="text-sm text-alpine-600 mb-3">
          L'<a className="underline decoration-alpine-300 hover:decoration-alpine-600" href="https://www.fedlex.admin.ch/eli/cc/2002/216/fr"
             target="_blank" rel="noreferrer">Olico art. 9</a> autorise la conservation sur
          support modifiable à condition que des procédés techniques garantissent
          l'intégrité des données. La chaîne d'empreintes le fait ; ces deux documents
          permettent de le montrer.
        </p>

        <div className="space-y-2">
          <DownloadRow
            icon={FileCheck2}
            title="Attestation d'intégrité"
            description="État de la chaîne, empreinte de tête, périmètre couvert — et ce que l'attestation ne prouve pas. À remettre à votre fiduciaire ou à un réviseur."
            filename="attestation"
            fetcher={() => auditApi.attestation()}
          />
          <DownloadRow
            icon={Download}
            title="Archive légale et export de réversibilité"
            description="Dix ans de pièces en JSON (exact) et en CSV (ouvrable dans un tableur, importable ailleurs), avec un manifeste d'empreintes. Vos données ne sont retenues par aucun format."
            filename="archive"
            fetcher={() => exportApi.legalArchive()}
          />
        </div>
      </div>
    </div>
  )
}

// ─── Déclaration d'un exercice ────────────────────────────────────────────────

function CreateFiscalYearForm({ onDone }: { onDone: () => void }) {
  const thisYear = new Date().getFullYear()
  const [name, setName] = useState(String(thisYear))
  const [start, setStart] = useState(`${thisYear}-01-01`)
  const [end, setEnd] = useState(`${thisYear}-12-31`)
  const [error, setError] = useState<string | null>(null)

  const create = useMutation({
    mutationFn: () => fiscalYearsApi.create({ name, start_date: start, end_date: end }),
    onSuccess: onDone,
    onError: (e: any) => setError(e?.response?.data?.error ?? "L'exercice n'a pas pu être créé."),
  })

  return (
    <div className="rounded-md border border-neutral-200 px-4 py-3 mb-3">
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
        <label className="text-sm">
          <span className="block text-xs text-alpine-500 mb-1">Nom</span>
          <input className="input w-full" value={name} onChange={e => setName(e.target.value)}
                 placeholder="2026 ou 2026/27" />
        </label>
        <label className="text-sm">
          <span className="block text-xs text-alpine-500 mb-1">Début</span>
          <input type="date" className="input w-full" value={start} onChange={e => setStart(e.target.value)} />
        </label>
        <label className="text-sm">
          <span className="block text-xs text-alpine-500 mb-1">Fin</span>
          <input type="date" className="input w-full" value={end} onChange={e => setEnd(e.target.value)} />
        </label>
      </div>
      {error && <p className="text-sm text-danger-700 mt-2">{error}</p>}
      <p className="text-xs text-alpine-500 mt-2">
        Deux exercices ne peuvent pas se chevaucher : le rattachement d'une écriture,
        et donc la clôture, deviendraient arbitraires.
      </p>
      <button
        onClick={() => { setError(null); create.mutate() }}
        disabled={create.isPending}
        className="btn-primary btn-sm mt-2 flex items-center gap-1.5"
      >
        {create.isPending && <Loader2 size={13} className="animate-spin" />}
        Créer l'exercice
      </button>
    </div>
  )
}

// ─── Téléchargement ───────────────────────────────────────────────────────────

function DownloadRow({ icon: Icon, title, description, filename, fetcher }: {
  icon: typeof Download
  title: string
  description: string
  filename: string
  // Le type de réponse d'axios n'est pas resserré ici : les en-têtes y sont
  // partiels par nature, et seul content-disposition nous intéresse.
  fetcher: () => Promise<{ data: Blob; headers: any }>
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Le téléchargement passe par axios plutôt que par un lien direct : la route
  // exige l'en-tête Authorization, qu'une navigation du navigateur n'enverrait
  // pas — le fichier reviendrait en 401 déguisé en téléchargement.
  async function download() {
    setBusy(true); setError(null)
    try {
      const res = await fetcher()
      const disposition: string = res.headers['content-disposition'] ?? ''
      const match = /filename="([^"]+)"/.exec(disposition)
      const url = URL.createObjectURL(res.data)
      const a = document.createElement('a')
      a.href = url
      a.download = match ? match[1] : filename
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(url)
    } catch {
      setError('Le téléchargement a échoué.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-2">
          <Icon size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
          <div>
            <p className="font-medium">{title}</p>
            <p className="text-alpine-600 mt-0.5">{description}</p>
            {error && <p className="text-danger-700 mt-1">{error}</p>}
          </div>
        </div>
        <button onClick={download} disabled={busy}
                className="btn-secondary btn-sm flex-shrink-0 flex items-center gap-1.5">
          {busy ? <Loader2 size={13} className="animate-spin" /> : <Download size={13} />}
          Télécharger
        </button>
      </div>
    </div>
  )
}

// ─── Données personnelles (nLPD) ──────────────────────────────────────────────

function PersonalDataSection() {
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
