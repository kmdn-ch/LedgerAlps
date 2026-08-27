// Durée de vie d'une session : déconnexion sur inactivité, régénération de la
// clé de signature.
//
// Les deux réglages vivent ensemble parce qu'ils bornent la même chose par deux
// chemins — combien de temps une session vaut quelque chose. Les séparer conduit
// à en durcir un et à laisser l'autre grand ouvert, ce qui donne l'illusion
// d'une protection sans la protection.
//
// # Pourquoi la régénération forcée est ici et pas ailleurs
//
// Elle vivait sur un autre écran, dans sa propre carte, à côté d'une carte qui
// annonçait la régénération automatique. Deux encadrés parlant de la même clé,
// dont l'un renvoyait à l'autre par « voir plus bas ». Qui lisait le premier
// croyait devoir cliquer ; ce que la machine fait déjà toute seule chaque nuit.
//
// Il n'y a qu'une clé : il n'y a qu'une carte. Elle dit ce qui se passe sans
// qu'on demande rien, puis offre la seule commande qui ajoute quelque chose —
// ne pas attendre demain.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Clock, RefreshCw, Loader2, AlertTriangle, ShieldCheck } from 'lucide-react'
import { securityApi, backupsApi } from '@/api/client'
import { SectionTitle, ErrorBanner } from '@/components/ui'
import { refusalMessage } from '@/utils/refusal'
import { targetURLAfterRestart, waitForShutdownThenGo } from '@/utils/restart'
import type { SecuritySettings, RotateSecretResult } from '@/types'
import { useT, useLangue } from '@/i18n/useT'
import { localeIntl } from '@/i18n'
import type { Cle } from '@/i18n'

// Deux minutes est le plancher accepté par le serveur : en dessous, la
// déconnexion tombe pendant la lecture d'un document, et comme aucun brouillon
// n'est enregistré, la saisie est perdue.
const IDLE_CHOICES: { v: number; cle: Cle }[] = [
  { v: 0,  cle: 'ss.jamais' },
  { v: 5,  cle: 'ss.cinqMinutes' },
  { v: 10, cle: 'ss.dixMinutes' },
  { v: 20, cle: 'ss.vingtMinutes' },
  { v: 60, cle: 'ss.uneHeure' },
]

// La locale suit la langue choisie : « 09.02.2026 14:30 » est suisse, un
// Britannique lit « 09/02/2026, 14:30 ». Le figer en fr-CH affichait une date
// française sur une interface anglaise.
function formatDateTime(iso: string | undefined, loc: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString(loc, {
    dateStyle: 'short', timeStyle: 'short',
  })
}

export function SessionSecurityPanel({ tlsEnabled }: { tlsEnabled: boolean }) {
  const t = useT()
  const loc = localeIntl(useLangue())
  const qc = useQueryClient()
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const [rotateError, setRotateError] = useState<string | null>(null)

  const settings = useQuery<SecuritySettings>({
    queryKey: ['settings', 'security'],
    queryFn:  () => securityApi.settings().then(r => r.data),
  })

  const save = useMutation({
    mutationFn: (body: { idle_logout_minutes?: number }) =>
      securityApi.saveSettings(body),
    onSuccess: () => { setError(null); qc.invalidateQueries({ queryKey: ['settings', 'security'] }) },
    onError:   (e) => setError(refusalMessage(e, t('ss.echecReglage'))),
  })

  const rotate = useMutation<RotateSecretResult>({
    mutationFn: () => securityApi.rotateSecret().then(r => r.data),
    onSuccess: () => {
      setConfirming(false)
      setRotateError(null)
      // La date affichée juste au-dessus vient de changer : la relire évite
      // d'annoncer « prochaine le … » à partir de la clé qu'on vient de jeter.
      qc.invalidateQueries({ queryKey: ['settings', 'security'] })
    },
    onError: (e) => setRotateError(refusalMessage(e, t('sp.echecRotation'))),
  })

  async function restartNow() {
    setRestarting(true)
    try {
      await backupsApi.restart()
      await waitForShutdownThenGo(targetURLAfterRestart(tlsEnabled))
    } catch {
      setRestarting(false)
      setRotateError(t('sp.echecRedemarrage'))
    }
  }

  const s = settings.data
  if (!s) return null

  return (
    <div className="mt-6">
      <SectionTitle>{t('ss.titre')}</SectionTitle>

      {error && <div className="mb-3"><ErrorBanner message={error} /></div>}

      <div className="space-y-3">
        {/* ── Déconnexion sur inactivité ──────────────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <Clock size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div className="flex-1">
              <p className="font-medium">{t('ss.deconnexionTitre')}</p>
              <p className="text-alpine-600 mt-0.5">
                {t('ss.deconnexionAide')}
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-2">
                {IDLE_CHOICES.map(c => (
                  <button
                    key={c.v}
                    onClick={() => save.mutate({ idle_logout_minutes: c.v })}
                    disabled={save.isPending}
                    className={`btn-sm rounded-md border px-2.5 py-1 ${
                      s.idle_logout_minutes === c.v
                        ? 'border-accent-700 bg-accent-700 text-white'
                        : 'border-neutral-200 hover:border-alpine-500'
                    }`}
                  >
                    {t(c.cle)}
                  </button>
                ))}
                {save.isPending && <Loader2 size={13} className="animate-spin text-alpine-500" />}
              </div>
              {s.idle_logout_minutes === 0 && (
                <p className="mt-2 text-warning-700 text-xs">
                  {t('ss.deconnexionDesactivee')}
                </p>
              )}
            </div>
          </div>
        </div>

        {/* ── Régénération de la clé de signature ─────────────────────────── */}
        <div className="rounded-md border border-neutral-200 px-4 py-3 text-sm">
          <div className="flex items-start gap-2">
            <RefreshCw size={16} className="mt-0.5 flex-shrink-0 text-alpine-500" />
            <div className="flex-1">
              <p className="font-medium">{t('ss.rotationTitre')}</p>
              <p className="text-alpine-600 mt-0.5">
                {t('ss.rotationAide')}
              </p>
              {/* Dire la vérité plutôt qu'afficher une date qui avance sans
                  effet. Quand JWT_SECRET vient de l'environnement — les
                  installations en service Linux/systemd et Windows Service —
                  la clé est réimposée à chaque démarrage : la rotation ne peut
                  pas aboutir, et l'annoncer quand même décourageait de
                  chercher ailleurs une protection qu'on croyait acquise. */}
              {s.rotation.bloquee_par_environnement ? (
                <p className="mt-2 text-warning-700">
                  {t('ss.rotationBloqueeEnv')}
                </p>
              ) : (
                <>
                  <p className="mt-2 text-success-700">
                    {t('ss.rotationCadence')}
                  </p>
                  <p className="mt-1 text-xs text-alpine-500">
                    {t('ss.derniereRegeneration', { date: formatDateTime(s.rotation.rotated_at, loc) })}
                    {s.rotation.next_at
                      && t('ss.prochaineAu', { date: formatDateTime(s.rotation.next_at, loc) })}
                  </p>
                </>
              )}

              {/* La seule commande de l'écran. Tout le reste se fait sans elle ;
                  elle ne sert qu'au cas que la périodicité ne couvre pas — on
                  vient de s'apercevoir d'une fuite, demain est trop tard. */}
              <div className="mt-3 border-t border-neutral-200 pt-3">
                <div className="flex items-start justify-between gap-4">
                  <div>
                    <p className="font-medium">{t('ss.regenerationImmediate')}</p>
                    <p className="text-alpine-600 mt-0.5">
                      {t('sp.regenererAide')}
                    </p>
                  </div>
                  {!confirming && !rotate.data && (
                    <button onClick={() => { setRotateError(null); setConfirming(true) }}
                            className="btn-secondary btn-sm flex-shrink-0">
                      {t('sp.regenerer')}
                    </button>
                  )}
                </div>

                {rotateError && <div className="mt-3"><ErrorBanner message={rotateError} /></div>}

                {/* La confirmation énonce la portée EXACTE plutôt qu'un
                    avertissement vague. « Êtes-vous sûr ? » n'aide personne à
                    décider ; « cela déconnecte les sessions et rien d'autre »
                    permet d'agir sans craindre de perdre sa comptabilité. */}
                {confirming && !rotate.data && (
                  <div className="mt-3 rounded-md border border-warning-500 bg-warning-100 px-3 py-2.5">
                    <div className="flex items-start gap-2">
                      <AlertTriangle size={15} className="mt-0.5 flex-shrink-0 text-warning-700" />
                      <div className="flex-1">
                        <p className="font-medium text-warning-700">{t('sp.ceQueCelaFait')}</p>
                        <ul className="mt-1.5 space-y-0.5 text-alpine-700 list-disc list-inside">
                          <li>{t('sp.effet1')}</li>
                          <li>{t('sp.effet2')}</li>
                          <li>{t('sp.effet3')}</li>
                          <li>{t('sp.effet4')}</li>
                        </ul>
                        <p className="mt-2 text-alpine-700">
                          {t('sp.pasLeDisque')}
                        </p>
                        <div className="mt-3 flex gap-2">
                          <button onClick={() => rotate.mutate()} disabled={rotate.isPending}
                                  className="btn-primary btn-sm flex items-center gap-1.5">
                            {rotate.isPending && <Loader2 size={13} className="animate-spin" />}
                            {t('sp.regenererLaCle')}
                          </button>
                          <button onClick={() => setConfirming(false)} className="btn-ghost btn-sm">
                            {t('action.annuler')}
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                {rotate.data && (
                  <div className="mt-3 rounded-md border border-neutral-200 bg-neutral-50 px-3 py-2.5">
                    <div className="flex items-start gap-2">
                      <ShieldCheck size={15} className="mt-0.5 flex-shrink-0 text-success-700" />
                      <div className="flex-1">
                        <p className="font-medium text-success-700">{t('sp.nouvelleCle')}</p>
                        <p className="text-alpine-700 mt-0.5">
                          {rotate.data.sessions_revoked > 0 && (
                            <>{t('sp.sessionsRevoquees', { n: rotate.data.sessions_revoked })} </>
                          )}
                          {t('sp.prendEffet')}
                        </p>
                        <button onClick={restartNow} disabled={restarting}
                                className="btn-primary btn-sm mt-2 flex items-center gap-1.5">
                          {restarting
                            ? <><Loader2 size={13} className="animate-spin" /> {t('sv.redemarrage')}</>
                            : <><RefreshCw size={13} /> {t('rs.redemarrerMaintenant')}</>}
                        </button>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
