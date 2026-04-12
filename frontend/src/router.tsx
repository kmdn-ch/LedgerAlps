// LedgerAlps — Routeur React (Phase 5)

import { createBrowserRouter, Navigate } from 'react-router-dom'
import { AppLayout }      from '@/components/layout/AppLayout'
import { LoginPage }      from '@/pages/LoginPage'
import { DashboardPage }  from '@/pages/DashboardPage'
import { InvoicesPage }   from '@/pages/InvoicesPage'
import { NewInvoicePage } from '@/pages/NewInvoicePage'
import { ContactsPage }   from '@/pages/ContactsPage'
import { AccountsPage }   from '@/pages/AccountsPage'
import { JournalPage }    from '@/pages/JournalPage'
import { ReportsPage }    from '@/pages/ReportsPage'
import { SettingsPage }   from '@/pages/SettingsPage'
import { InvoiceDetailPage }  from '@/pages/InvoiceDetailPage'
import { EditInvoicePage }    from '@/pages/EditInvoicePage'
import { ContactDetailPage } from '@/pages/ContactDetailPage'
import { useAuthStore }   from '@/store/auth'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const isAuth = useAuthStore(s => s.isAuth)
  if (!isAuth) return <Navigate to="/login" replace />
  return <>{children}</>
}

export const router = createBrowserRouter([
  { path: '/login', element: <LoginPage /> },
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
