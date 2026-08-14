// LedgerAlps — Réseau & chiffrement (onglet Maintenance)
//
// Ces réglages vivent dans config.json, écrit une seule fois par l'assistant de
// premier démarrage et jamais retouché ensuite. Toute option ajoutée depuis
// était donc absente du fichier, lue comme sa valeur par défaut, et
// inatteignable — c'est pourquoi ouvrir l'accès réseau ne fonctionnait pas.
//
// Éditer du JSON dans %APPDATA% n'est pas une réponse pour un logiciel de
// comptabilité. Cet écran écrit le fichier, en préservant les clés qu'il ne
// connaît pas, et demande un redémarrage : l'adresse d'écoute et le chiffrement
// sont choisis une seule fois, au démarrage du serveur.

import { useState, useEffect } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { Save, RefreshCw, Loader2 } from 'lucide-react'
import { maintenanceApi, backupsApi } from '@/api/client'
import { SectionTitle, LoadingSpinner, ErrorBanner } from '@/components/ui'
import { targetURLAfterRestart, waitForShutdownThenGo } from '@/utils/restart'
import type { ServerSettings } from '@/types'
import { useT } from '@/i18n/useT'

const LOOPBACK = ['127.0.0.1', 'localhost', '::1']

export function NetworkSettings({ onSaved }: { onSaved: () => void }) {
  const t = useT()
  const { data, isLoading, isError, refetch } = useQuery<{
    settings: ServerSettings; restart_required: boolean; config_file: string
  }>({
    queryKey: ['settings', 'server'],
    queryFn: () => maintenanceApi.getServerSettings().then(r => r.data),
  })

  const [form, setForm] = useState<ServerSettings | null>(null)
  useEffect(() => { if (data?.settings) setForm(data.settings) }, [data])

  // Ce qui est ENREGISTRÉ dans config.json. Distinct de `data.settings`, qui
  // décrit la configuration en cours d'exécution : après un enregistrement,
  // celle-ci est encore l'ancienne — c'est tout l'objet du redémarrage.
  const [saved, setSaved] = useState<ServerSettings | null>(null)

  const save = useMutation({
    mutationFn: (s: ServerSettings) => maintenanceApi.putServerSettings(s),
    onSuccess: (_resp, s) => { setSaved(s); onSaved(); refetch() },
  })

  // Le schéma peut changer sous nos pieds : activer TLS fait repartir le
  // serveur en https sur le MÊME port. Recharger la page en place, ou sonder
  // /health en relatif, se heurterait alors à une poignée de main TLS — c'est
  // ce qui faisait tourner ce bouton indéfiniment. On attend l'arrêt, puis on
  // navigue vers la bonne adresse.
  // Le schéma cible vient des réglages ENREGISTRÉS, pas du formulaire : une
  // case cochée sans avoir enregistré enverrait vers une adresse que le serveur
  // ne servira pas.
  const restart = useMutation({
    mutationFn: async (saved: ServerSettings) => {
      // Le chiffrement suit l'exposition : local en clair, réseau en HTTPS.
      const tlsAfter = !LOOPBACK.includes(saved.host)
      await backupsApi.restart()
      await waitForShutdownThenGo(targetURLAfterRestart(tlsAfter))
    },
  })

  // `!form` seul tournait indéfiniment : une requête refusée (403) ne charge
  // jamais le formulaire, et l'écran restait sur un rond qui tourne — le pire
  // des états, puisqu'il annonce que quelque chose arrive.
  if (isLoading) return <LoadingSpinner />
  if (isError || !form) {
    return <ErrorBanner message={t('rs.erreurLecture')} />
  }

  const loopback = LOOPBACK.includes(form.host)
  const pending = save.isSuccess || data?.restart_required
  const certPlaceholder = 'C:\\LedgerAlps\\cert.pem'
  const keyPlaceholder = 'C:\\LedgerAlps\\key.pem'

  return (
    <div>
      <SectionTitle>{t('rs.titre')}</SectionTitle>

      <label className="label" htmlFor="net-host">{t('rs.interfaceEcoute')}</label>
      <select
        id="net-host"
        className="input w-full max-w-md"
        value={loopback ? '127.0.0.1' : '0.0.0.0'}
        onChange={e => setForm({ ...form, host: e.target.value })}
      >
        <option value="127.0.0.1">{t('rs.machineUniquement')}</option>
        <option value="0.0.0.0">{t('rs.accessibleReseau')}</option>
      </select>
      <p className="text-xs text-alpine-500 mt-1.5 max-w-md">
        {t(loopback ? 'rs.loopbackAide' : 'rs.reseauAide')}
      </p>

      <div className="mt-4 max-w-md space-y-2">
        <div>
          <label className="label" htmlFor="net-cert">
            {t('rs.certificatTLS')} <span className="text-alpine-400">{t('pr.facultatif')}</span>
          </label>
          <input
            id="net-cert"
            className="input w-full"
            placeholder={certPlaceholder}
            value={form.tls_cert}
            onChange={e => setForm({ ...form, tls_cert: e.target.value })}
          />
        </div>
        <div>
          <label className="label" htmlFor="net-key">{t('rs.clePrivee')}</label>
          <input
            id="net-key"
            className="input w-full"
            placeholder={keyPlaceholder}
            value={form.tls_key}
            onChange={e => setForm({ ...form, tls_key: e.target.value })}
          />
        </div>
        <p className="text-xs text-alpine-500">
          {t('rs.certificatAide')}
        </p>
      </div>

      {save.isError && (
        <div className="mt-3 max-w-md">
          <ErrorBanner message={t('rs.reglagesRefuses')} />
        </div>
      )}

      <div className="flex items-center gap-2 mt-4">
        <button
          onClick={() => save.mutate(form)}
          disabled={save.isPending}
          className="btn-primary btn-sm flex items-center gap-1.5"
        >
          {save.isPending ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
          {t('action.enregistrer')}
        </button>
        {pending && (
          <button
            onClick={() => restart.mutate(saved ?? data!.settings)}
            disabled={restart.isPending}
            className="btn-secondary btn-sm flex items-center gap-1.5"
          >
            {restart.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
            {restart.isPending ? t('sv.redemarrage') : t('rs.redemarrerMaintenant')}
          </button>
        )}
      </div>

      {pending && (
        <div className="mt-3 max-w-xl rounded-md border border-warning-500 bg-warning-100 px-4 py-3 text-sm">
          {t('rs.pasAppliques')}
          {!LOOPBACK.includes((saved ?? form).host) && <> {t('rs.serviEnHttps')}</>}
        </div>
      )}

      {restart.isPending && (
        <p className="text-xs text-alpine-600 mt-2">
          {t('rs.redemarrageEnCours')}
          {!LOOPBACK.includes((saved ?? data!.settings).host) && ' ' + t('rs.avertissementAttendu')}
        </p>
      )}
      {restart.isError && (
        <p className="text-xs text-danger-700 mt-2">
          {t('rs.redemarrageEchoue')}
        </p>
      )}

      {data?.config_file && (
        <p className="text-xs text-alpine-500 mt-2 font-mono break-all">{data.config_file}</p>
      )}
    </div>
  )
}
