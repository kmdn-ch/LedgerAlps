// LedgerAlps — Réseau & chiffrement (onglet Maintenance)
//
// Ces réglages vivent dans config.json, écrit une seule fois par l'assistant de
// premier démarrage et jamais retouché ensuite. Toute option ajoutée depuis
// était donc absente du fichier, lue comme sa valeur par défaut, et
// inatteignable — c'est pourquoi activer HTTPS ne fonctionnait pas.
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

const LOOPBACK = ['127.0.0.1', 'localhost', '::1']

export function NetworkSettings({ onSaved }: { onSaved: () => void }) {
  const { data, isLoading, refetch } = useQuery<{
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
      const tlsAfter = saved.force_tls || !LOOPBACK.includes(saved.host)
      await backupsApi.restart()
      await waitForShutdownThenGo(targetURLAfterRestart(tlsAfter))
    },
  })

  if (isLoading || !form) return <LoadingSpinner />

  const loopback = LOOPBACK.includes(form.host)
  const pending = save.isSuccess || data?.restart_required
  const certPlaceholder = 'C:\\LedgerAlps\\cert.pem'
  const keyPlaceholder = 'C:\\LedgerAlps\\key.pem'

  return (
    <div>
      <SectionTitle>Réseau &amp; chiffrement</SectionTitle>

      <label className="label" htmlFor="net-host">Interface d'écoute</label>
      <select
        id="net-host"
        className="input w-full max-w-md"
        value={loopback ? '127.0.0.1' : '0.0.0.0'}
        onChange={e => setForm({ ...form, host: e.target.value })}
      >
        <option value="127.0.0.1">Cette machine uniquement (recommandé)</option>
        <option value="0.0.0.0">Accessible depuis le réseau</option>
      </select>
      <p className="text-xs text-alpine-500 mt-1.5 max-w-md">
        {loopback
          ? 'Le trafic ne quitte jamais cet ordinateur : rien sur le réseau ne peut le lire.'
          : "D'autres postes pourront se connecter. LedgerAlps servira alors en HTTPS, avec un certificat auto-signé si vous n'en fournissez pas — votre navigateur avertira à la première visite."}
      </p>

      <label className="flex items-start gap-2 mt-4 max-w-md cursor-pointer">
        <input
          type="checkbox"
          className="mt-0.5"
          checked={form.force_tls}
          onChange={e => setForm({ ...form, force_tls: e.target.checked })}
        />
        <span className="text-sm">
          Chiffrer aussi l'accès local (HTTPS sur <code className="font-mono text-xs">localhost</code>)
          <span className="block text-xs text-alpine-500 mt-0.5">
            Non nécessaire : le trafic vers cet ordinateur ne touche aucune interface réseau, et les
            navigateurs considèrent <code className="font-mono">localhost</code> comme une origine de
            confiance. À activer si votre politique de sécurité exige TLS partout — vous verrez alors
            un avertissement de certificat.
          </span>
        </span>
      </label>

      <div className="mt-4 max-w-md space-y-2">
        <div>
          <label className="label" htmlFor="net-cert">
            Certificat TLS <span className="text-alpine-400">(facultatif)</span>
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
          <label className="label" htmlFor="net-key">Clé privée</label>
          <input
            id="net-key"
            className="input w-full"
            placeholder={keyPlaceholder}
            value={form.tls_key}
            onChange={e => setForm({ ...form, tls_key: e.target.value })}
          />
        </div>
        <p className="text-xs text-alpine-500">
          Laissez-les vides pour un certificat auto-signé. Les fournir évite l'avertissement du
          navigateur, si le certificat vient d'une autorité que vos postes reconnaissent.
        </p>
      </div>

      {save.isError && (
        <div className="mt-3 max-w-md">
          <ErrorBanner message="Réglages refusés. Vérifiez les chemins de certificat — rien n'a été modifié." />
        </div>
      )}

      <div className="flex items-center gap-2 mt-4">
        <button
          onClick={() => save.mutate(form)}
          disabled={save.isPending}
          className="btn-primary btn-sm flex items-center gap-1.5"
        >
          {save.isPending ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
          Enregistrer
        </button>
        {pending && (
          <button
            onClick={() => restart.mutate(saved ?? data!.settings)}
            disabled={restart.isPending}
            className="btn-secondary btn-sm flex items-center gap-1.5"
          >
            {restart.isPending ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
            {restart.isPending ? 'Redémarrage…' : 'Redémarrer maintenant'}
          </button>
        )}
      </div>

      {pending && (
        <div className="mt-3 max-w-xl rounded-md border border-warning-500 bg-warning-100 px-4 py-3 text-sm">
          Réglages enregistrés mais <strong>pas encore appliqués</strong> : l'adresse d'écoute et le
          chiffrement sont choisis une seule fois, au démarrage. Redémarrez LedgerAlps pour qu'ils
          prennent effet.
          {(saved ?? form).force_tls && (
            <> L'application se rouvrira sur <code className="font-mono">https://localhost</code>, et
            votre navigateur affichera un avertissement de certificat à la première visite.</>
          )}
        </div>
      )}

      {restart.isPending && (
        <p className="text-xs text-alpine-600 mt-2">
          Redémarrage en cours… la page va s'ouvrir sur la nouvelle adresse.
          {(() => { const t = saved ?? data!.settings; return t.force_tls || !LOOPBACK.includes(t.host) })() &&
            " Votre navigateur affichera un avertissement de certificat : c'est attendu avec un certificat auto-signé."}
        </p>
      )}
      {restart.isError && (
        <p className="text-xs text-danger-700 mt-2">
          Le redémarrage n'a pas abouti. Fermez puis rouvrez l'application manuellement.
        </p>
      )}

      {data?.config_file && (
        <p className="text-xs text-alpine-500 mt-2 font-mono break-all">{data.config_file}</p>
      )}
    </div>
  )
}
