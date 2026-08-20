// Choisir la langue AVANT d'être identifié.
//
// # Pourquoi il faut qu'il soit là
//
// Le sélecteur de langue vivait dans Paramètres → Mon compte, c'est-à-dire
// derrière la connexion. Un employé germanophone ou une fiduciaire tessinoise
// qui arrive sur cet écran devait donc lire le français pour trouver comment ne
// plus lire le français. Et l'écran de connexion est précisément celui où l'on
// hésite : « ai-je le bon mot de passe » se pense mal dans une langue qu'on
// déchiffre.
//
// # Ce qu'il ne fait pas
//
// Il ne demande rien au serveur. Le catalogue est embarqué, le choix se garde
// dans le navigateur, et le changement est immédiat — recharger la page pour
// changer une langue laisserait croire qu'il se passe quelque chose de plus
// lourd.

import { Languages } from 'lucide-react'
import { LANGUES, type Langue } from '@/i18n'
import { useLangueStore } from '@/i18n/useT'

export function SelecteurLangue({ className = '' }: { className?: string }) {
  const langue = useLangueStore(s => s.langue)
  const definir = useLangueStore(s => s.definir)

  return (
    <div className={`flex items-center gap-1 ${className}`}>
      <Languages size={13} className="text-slate-500 mr-1" aria-hidden="true" />
      {LANGUES.map(l => (
        <button
          key={l.code}
          type="button"
          onClick={() => definir(l.code as Langue)}
          aria-pressed={langue === l.code}
          // Le nom de la langue DANS cette langue — « Deutsch », pas
          // « Allemand ». C'est le seul libellé qui se lit quand on ne
          // comprend pas la langue affichée.
          title={l.nom}
          className={`rounded px-2 py-1 text-[11px] font-medium tracking-wide transition-colors ${
            langue === l.code
              ? 'bg-brand-orange/15 text-brand-orange'
              : 'text-slate-500 hover:text-slate-300'
          }`}
        >
          {l.drapeau}
        </button>
      ))}
    </div>
  )
}
