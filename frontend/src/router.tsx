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
import { useAuthStore }   from '@/store/auth'
import { useCanWrite } from '@/hooks/usePermissions'
import { useRestoreSession } from '@/hooks/useRestoreSession'
import { useT }           from '@/i18n/useT'
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
  const t = useT()
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
  // La restauration de session vit dans un hook partagé : les écrans hors de cette coquille
  // (changement du mot de passe temporaire, second facteur) en ont besoin aussi.
  const restoring = useRestoreSession()

  if (mustChange) return <Navigate to="/change-password" replace />
  if (needsMfa) return <Navigate to="/second-facteur" replace />
  if (!isAuth) return <Navigate to="/login" replace />

  // Sans cette attente, les pages partiraient chercher leurs données sans
  // jeton, prendraient un 401 et déconnecteraient l'utilisateur au rechargement.
  if (restoring) {
    return (
      <div className="min-h-screen flex items-center justify-center text-sm text-alpine-500">
        {t('rt.restauration')}
      </div>
    )
  }

  return <>{children}</>
}

/**
 * RequireWrite protège les écrans qui n'existent QUE pour écrire.
 *
 * Masquer les boutons ne suffit pas, et l'oubli est arrivé : le tableau de bord
 * portait « Nouvelle facture » sous forme de lien, hors de toute liste de
 * mutations — un compte en lecture seule y accédait. Chercher les commandes une
 * par une, c'est refaire la recherche à chaque écran ajouté.
 *
 * Ici la barrière porte sur l'ADRESSE. `/invoices/new` et `/invoices/:id/edit`
 * n'ont aucune raison d'être atteintes par quelqu'un qui ne peut rien
 * enregistrer, quel que soit le chemin emprunté pour y arriver — bouton oublié,
 * lien collé, favori, bouton « précédent ». Le serveur refuserait de toute
 * façon l'enregistrement ; ceci évite de laisser remplir un formulaire entier
 * avant de le dire.
 */
function RequireWrite({ children }: { children: React.ReactNode }) {
  const peutEcrire = useCanWrite()
  if (!peutEcrire) return <Navigate to="/" replace />
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
      { path: 'invoices/new',              element: <RequireWrite><NewInvoicePage /></RequireWrite> },
      { path: 'invoices/:invoiceId',      element: <InvoiceDetailPage /> },
      { path: 'invoices/:invoiceId/edit', element: <RequireWrite><EditInvoicePage /></RequireWrite> },
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
