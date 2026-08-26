// LedgerAlps — Layout principal

import { Outlet } from 'react-router-dom'
import { IdleGuard } from './IdleGuard'
import { Sidebar } from './Sidebar'
import { ComplianceBanner } from './ComplianceBanner'
import { LedgerAlpsLogo } from '@/components/brand/Logo'

export function AppLayout() {
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <main className="flex-1 ml-[240px] min-h-screen overflow-x-hidden">
        <div className="max-w-[1400px] mx-auto px-6 py-6">
          {/* La marque du PRODUIT, en haut à droite.
              En haut à GAUCHE vit l'entreprise de l'utilisateur — son logo, son
              nom, dans la barre latérale — et lui disputer cette place serait
              malpoli. À droite, la marque ne gêne personne et répond à la seule
              question qu'on se pose devant une capture d'écran : quel logiciel
              est-ce ?

              Sur ce fond clair elle se pose telle quelle. Elle est en bleu
              nuit : c'est sur les fonds SOMBRES — barre latérale, écran de
              connexion — qu'il lui faut une plaque claire. */}
          <div className="flex justify-end mb-3">
            <LedgerAlpsLogo className="h-5 w-auto" />
          </div>
          {/* Les évolutions légales concernent toute l'application, pas une
              page en particulier : la bannière est donc montée dans le layout. */}
          <ComplianceBanner />
          <Outlet />
          <IdleGuard />
        </div>
      </main>
    </div>
  )
}
