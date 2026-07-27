// LedgerAlps — Layout principal

import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { ComplianceBanner } from './ComplianceBanner'

export function AppLayout() {
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <main className="flex-1 ml-[240px] min-h-screen overflow-x-hidden">
        <div className="max-w-[1400px] mx-auto px-6 py-6">
          {/* Les évolutions légales concernent toute l'application, pas une
              page en particulier : la bannière est donc montée dans le layout. */}
          <ComplianceBanner />
          <Outlet />
        </div>
      </main>
    </div>
  )
}
