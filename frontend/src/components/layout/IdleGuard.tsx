// Déconnexion après inactivité — la partie visible.
//
// Le délai vient du serveur. Il n'est lu qu'une fois la session ouverte, et un
// échec de lecture laisse le mécanisme inactif plutôt que de déconnecter sur un
// délai deviné : se faire éjecter parce qu'une requête a échoué serait la pire
// façon de découvrir cette fonctionnalité.
//
// L'avertissement est un dialogue modal, volontairement. LedgerAlps
// n'enregistre aucun brouillon automatique : si quelqu'un est en train de saisir
// une facture, il doit voir arriver la coupure et pouvoir l'arrêter, pas
// découvrir un écran de connexion à la place de son travail.

import { useCallback, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Clock } from 'lucide-react'
import { authApi, securityApi } from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { useIdleLogout } from '@/hooks/useIdleLogout'
import type { SecuritySettings } from '@/types'
import { useT, useFormats } from '@/i18n/useT'

export function IdleGuard() {
  const t = useT()
  const { pluriel } = useFormats()
  const navigate = useNavigate()
  const logout   = useAuthStore(s => s.logout)
  const isAuth   = useAuthStore(s => s.isAuth)

  const settings = useQuery<SecuritySettings>({
    queryKey: ['settings', 'security'],
    queryFn:  () => securityApi.settings().then(r => r.data),
    enabled:  isAuth,
    // Le réglage change rarement ; le relire en boucle n'apporterait rien.
    staleTime: 5 * 60_000,
    retry: false,
  })

  const doLogout = useCallback(() => {
    // Prévenir le serveur pour qu'il révoque le jeton de rafraîchissement :
    // sans cela le cookie resterait valable trente jours, et « déconnecté »
    // ne voudrait dire déconnecté que dans cet onglet.
    authApi.logout().catch(() => { /* la session locale part de toute façon */ })
    logout()
    navigate('/login?raison=inactivite', { replace: true })
  }, [logout, navigate])

  const minutes = settings.data?.idle_logout_minutes ?? 0
  const { warning, remaining, stay } = useIdleLogout(isAuth ? minutes : 0, doLogout)

  // Échap remet la session en marche : c'est le geste réflexe devant un
  // dialogue, et ici il doit vouloir dire « je suis là », pas « ignore-moi ».
  useEffect(() => {
    if (!warning) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') stay() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [warning, stay])

  if (!warning) return null

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 p-4"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="idle-title"
      aria-describedby="idle-body"
    >
      <div className="bg-white rounded-lg shadow-xl max-w-md w-full p-5">
        <h2 id="idle-title" className="text-base font-semibold flex items-center gap-2">
          <Clock size={18} className="text-warning-700" />
          {t('ig.toujoursLa')}
        </h2>
        <p id="idle-body" className="text-sm text-alpine-700 mt-2">
          {pluriel(remaining,
            t('ig.deconnexionDansUne', { n: remaining }),
            t('ig.deconnexionDans', { n: remaining }))}
        </p>
        <p className="text-xs text-alpine-600 mt-2">
          {t('ig.saisiePerdue')}
        </p>
        <div className="flex items-center gap-2 mt-4">
          <button onClick={stay} className="btn-primary btn-sm" autoFocus>
            {t('ig.resterConnecte')}
          </button>
          <button onClick={doLogout} className="btn-ghost btn-sm">
            {t('ig.deconnecterMaintenant')}
          </button>
        </div>
      </div>
    </div>
  )
}
