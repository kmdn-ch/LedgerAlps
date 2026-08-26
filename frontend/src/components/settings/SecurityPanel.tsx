// LedgerAlps — Sécurité (Paramètres → Maintenance)
//
// Cet écran n'est plus qu'un assemblage : chaque sujet porte son propre titre et
// se lit seul.
//
// La régénération de la clé de signature vivait ici, dans une carte à elle, au
// dessus d'une seconde carte — dans le panneau suivant — qui annonçait que la
// même clé était de toute façon régénérée chaque nuit. Deux encadrés pour une
// seule clé, dont le premier renvoyait au second par « voir plus bas ». Elle a
// rejoint celui qui décrit la régénération automatique : voir
// SessionSecurityPanel.

import { DatabaseEncryptionPanel } from './DatabaseEncryptionPanel'
import { SessionSecurityPanel } from './SessionSecurityPanel'
import { UsersPanel } from './UsersPanel'
import { useAuthStore } from '@/store/auth'

export function SecurityPanel({ tlsEnabled }: { tlsEnabled: boolean }) {
  const isAdmin = useAuthStore(st => st.role) === 'admin'

  return (
    <div>
      <DatabaseEncryptionPanel />
      {/* Le second facteur a quitté cet écran : il appartient au COMPTE de
          celui qui le lit, pas à l'administration du logiciel. Le laisser ici
          l'aurait rendu inatteignable pour un comptable, qui doit pourtant
          inscrire le sien — il vit désormais dans l'onglet « Mon compte ». */}
      <SessionSecurityPanel tlsEnabled={tlsEnabled} />
      {isAdmin && <UsersPanel />}
    </div>
  )
}
