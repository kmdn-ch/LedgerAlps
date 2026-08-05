// LedgerAlps — Routeur React (Phase 5)

import { createBrowserRouter, Navigate } from 'react-router-dom'
import { AppLayout }      from '@/components/layout/AppLayout'
import { LoginPage }      from '@/pages/LoginPage'
import { DashboardPage }  from '@/pages/DashboardPage'
import { InvoicesPage }   from '@/pages/InvoicesPage'
import { NewInvoicePage } from '@/pages/NewInvoicePage'
import { ContactsPage }   from '@/pages/ContactsPage'
import { AccountsPage }   from '@/pages/AccountsPage'
import { PurchasesPage }  from '@/pages/PurchasesPage'
import { JournalPage }    from '@/pages/JournalPage'
import { ReportsPage }    from '@/pages/ReportsPage'
import { SettingsPage }   from '@/pages/SettingsPage'
import { InvoiceDetailPage }  from '@/pages/InvoiceDetailPage'
import { EditInvoicePage }    from '@/pages/EditInvoicePage'
import { ContactDetailPage } from '@/pages/ContactDetailPage'
import { useEffect, useState } from 'react'
import { useAuthStore }   from '@/store/auth'
import { authApi }        from '@/api/client'
import { ChangePasswordPage } from '@/pages/ChangePasswordPage'
import { MFAEnrolmentPage } from '@/pages/MFAEnrolmentPage'

/**
 * Protège les routes et restaure la session au chargement.
 *
 * Le jeton d'accès ne vit qu'en mémoire : un rechargement de page le perd.
 * On demande donc au serveur d'en émettre un nouveau à partir du cookie
 * HttpOnly, que le navigateur envoie tout seul. Sans cette étape, chaque
 * F5 renverrait l'utilisateur à l'écran de connexion.
 */
function RequireAuth({ children }: { children: React.ReactNode }) {
  // Tous les hooks AVANT le moindre retour anticipé.
  //
  // React identifie un hook par son rang d'appel : un `return` placé au milieu
  // change ce rang au rendu suivant, et l'état d'un hook se retrouve lu par un
  // autre. Ce composant redirigeait déjà avant d'appeler ses hooks suivants ;
  // ça tenait parce que la redirection démonte tout, mais chaque garde ajoutée
  // rapprochait la panne.
  //
  // Un mot de passe temporaire n'ouvre rien d'autre que son propre changement,
  // et un administrateur sans second facteur ne peut rien faire non plus. Le
  // serveur refuse déjà toute autre requête dans les deux cas ; ces détours
  // évitent d'afficher une application dont chaque appel échouerait.
  const mustChange = useAuthStore(s => s.mustChangePassword)
  const needsMfa   = useAuthStore(s => s.mfaEnrolmentRequired)
  const isAuth     = useAuthStore(s => s.isAuth)
  // État initial calculé une seule fois : y a-t-il une session à restaurer ?
  const [restoring, setRestoring] = useState(() => {
    const s = useAuthStore.getState()
    return s.isAuth && !s.accessToken
  })

  // Tableau de dépendances vide, délibérément : l'effet ne doit s'exécuter
  // qu'au montage. Avec [isAuth, accessToken], poser le nouveau jeton
  // déclenchait le nettoyage de l'effet AVANT que le .finally ne s'exécute —
  // « annulé » passait à vrai, setRestoring(false) était sauté, et l'écran
  // restait bloqué sur « Restauration de la session… » alors que l'appel avait
  // pourtant réussi.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!restoring) return
    let monte = true

    authApi.refresh()
      .then((res) => {
        if (monte) useAuthStore.getState().setAccessToken(res.data.access_token)
      })
      .catch(() => {
        // Cookie absent, expiré ou révoqué : la session est bel et bien finie.
        if (monte) useAuthStore.getState().logout()
      })
      .finally(() => {
        if (monte) setRestoring(false)
      })

    return () => { monte = false }
  }, [])

  if (mustChange) return <Navigate to="/change-password" replace />
  if (needsMfa) return <Navigate to="/second-facteur" replace />
  if (!isAuth) return <Navigate to="/login" replace />

  // Sans cette attente, les pages partiraient chercher leurs données sans
  // jeton, prendraient un 401 et déconnecteraient l'utilisateur au rechargement.
  if (restoring) {
    return (
      <div className="min-h-screen flex items-center justify-center text-sm text-alpine-500">
        Restauration de la session…
      </div>
    )
  }

  return <>{children}</>
}

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
  // Hors de la coquille protégée : un compte au mot de passe temporaire n'a
  // accès à rien d'autre, et l'inclure dans RequireAuth l'y renverrait en
  // boucle.
  { path: '/change-password', element: <ChangePasswordPage /> },
  // Hors de la coquille protégée pour la même raison : un administrateur non
  // inscrit n'a accès à rien d'autre, et l'inclure l'y renverrait en boucle.
  { path: '/second-facteur', element: <MFAEnrolmentPage /> },
  {
    path: '/',
    element: <RequireAuth><AppLayout /></RequireAuth>,
    children: [
      { index: true,          element: <DashboardPage  /> },
      { path: 'invoices',     element: <InvoicesPage   /> },
      { path: 'invoices/new',              element: <NewInvoicePage    /> },
      { path: 'invoices/:invoiceId',      element: <InvoiceDetailPage /> },
      { path: 'invoices/:invoiceId/edit', element: <EditInvoicePage   /> },
      { path: 'quotes',       element: <InvoicesPage mode="quote" /> },
      { path: 'purchases',    element: <PurchasesPage  /> },
      { path: 'contacts',                element: <ContactsPage      /> },
      { path: 'contacts/:contactId',     element: <ContactDetailPage /> },
      { path: 'accounts',     element: <AccountsPage   /> },
      { path: 'journal',      element: <JournalPage    /> },
      { path: 'reports',      element: <ReportsPage    /> },
      { path: 'settings',     element: <SettingsPage   /> },
      { path: '*',            element: <Navigate to="/" replace /> },
    ],
  },
])
