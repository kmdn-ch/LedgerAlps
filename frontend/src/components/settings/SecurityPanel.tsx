// LedgerAlps — Sécurité (Paramètres → Maintenance)
//
// Cet écran n'est plus qu'un assemblage : chaque sujet porte son propre titre et
// se lit seul.
//
// La régénération de la clé de signature vit dans SessionSecurityPanel, une seule
// carte pour une seule clé.

import { DatabaseEncryptionPanel } from './DatabaseEncryptionPanel'
import { SessionSecurityPanel } from './SessionSecurityPanel'
import { UsersPanel } from './UsersPanel'
import { useAuthStore } from '@/store/auth'

export function SecurityPanel() {
  const isAdmin = useAuthStore(st => st.role) === 'admin'

  return (
    <div>
      <DatabaseEncryptionPanel />
      {/* Le second facteur a quitté cet écran : il appartient au COMPTE de
          celui qui le lit, pas à l'administration du logiciel. Le laisser ici
          l'aurait rendu inatteignable pour un comptable, qui doit pourtant
          inscrire le sien — il vit désormais dans l'onglet « Mon compte ». */}
      <SessionSecurityPanel />
      {isAdmin && <UsersPanel />}
    </div>
  )
}
