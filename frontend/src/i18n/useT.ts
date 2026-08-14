// Le crochet que les écrans appellent.
//
// `const t = useT()` puis `t('nav.achats')`. Rien de plus : une clé inconnue est
// une erreur de compilation, pas une chaîne vide découverte par un utilisateur.
//
// # Pourquoi la langue vit dans son propre magasin
//
// Elle survit à la déconnexion. Quelqu'un qui règle l'italien puis se déconnecte
// doit retrouver l'italien sur l'écran de connexion — sinon il lui faut se
// connecter en français pour pouvoir choisir l'italien, ce qui est absurde. Le
// magasin d'authentification, lui, est vidé à la déconnexion.
//
// La préférence est aussi enregistrée SUR LE COMPTE, pour suivre la personne
// d'un poste à l'autre. Le magasin local sert d'abord : il répond sans requête,
// et il faut bien afficher quelque chose avant de savoir qui se connecte.

import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { fr } from './fr'
import { de } from './de'
import { it } from './it'
import { en } from './en'
import {
  type Cle, type Catalogue, type Langue,
  interpole, langueValide, localeIntl, plural,
} from './index'

const CATALOGUES: Record<Langue, Catalogue> = { fr, de, it, en }

interface EtatLangue {
  langue: Langue
  definir: (l: Langue) => void
}

export const useLangueStore = create<EtatLangue>()(
  persist(
    (set) => ({
      langue: 'fr',
      definir: (l) => set({ langue: langueValide(l) }),
    }),
    {
      name: 'ledgeralps-langue',
      // Le nom du réglage est distinct de celui de la session : effacer l'un ne
      // doit pas effacer l'autre.
      merge: (persiste, courant) => ({
        ...courant,
        langue: langueValide((persiste as Partial<EtatLangue>)?.langue),
      }),
    },
  ),
)

/** La langue courante. */
export function useLangue(): Langue {
  return useLangueStore(s => s.langue)
}

/**
 * useT rend la fonction de traduction.
 *
 * Les valeurs interpolées passent en second argument, nommées :
 * `t('tva.tauxNormal', { taux: 8.1 })`.
 */
export function useT() {
  const langue = useLangueStore(s => s.langue)
  const cat = CATALOGUES[langue] ?? fr
  return (cle: Cle, valeurs?: Record<string, string | number>): string =>
    interpole(cat[cle] ?? fr[cle] ?? String(cle), valeurs)
}

/**
 * traduire est la version HORS composant de `useT()`.
 *
 * Elle sert aux modules qui ne sont pas des composants React — la couche API,
 * les formateurs — et qui doivent tout de même rendre un texte à l'utilisateur.
 * Elle lit la langue dans le magasin, qui vit en dehors de React.
 *
 * Ne l'utilisez PAS dans un composant : n'étant abonnée à rien, elle ne
 * provoquerait aucun rendu au changement de langue, et l'écran garderait
 * l'ancien texte jusqu'au prochain rendu venu d'ailleurs. Dans un composant,
 * `useT()`.
 */
export function traduire(cle: Cle, valeurs?: Record<string, string | number>): string {
  const cat = CATALOGUES[useLangueStore.getState().langue] ?? fr
  return interpole(cat[cle] ?? fr[cle] ?? String(cle), valeurs)
}

/**
 * useTv traduit un message de validation.
 *
 * # Pourquoi un crochet séparé
 *
 * Les schémas zod sont des constantes de module : ils sont construits une fois,
 * au chargement du fichier, avant qu'aucun composant n'existe. `useT()` n'y est
 * donc pas appelable. On écrit la CLÉ dans le schéma — `z.string().min(1,
 * 'val.requis')` — et on la traduit ici, au moment d'afficher l'erreur.
 *
 * Un message qui n'est pas une clé connue ressort tel quel : les refus du
 * serveur arrivent déjà rédigés, et les rendre en « val.requis » serait pire
 * que de ne rien traduire.
 */
export function useTv() {
  const t = useT()
  return (message?: string): string => {
    if (!message) return ''
    return message in fr ? t(message as Cle) : message
  }
}

/**
 * useFormats rend les formateurs accordés à la langue.
 *
 * Ils comptent autant que les mots : `1'234.50` est suisse, `1,234.50` est
 * anglais, et une date en `jj.mm.aaaa` se lit à l'envers pour un Britannique
 * habitué à `jj/mm/aaaa`. Voir docs/GLOSSAIRE.md.
 */
export function useFormats() {
  const langue = useLangueStore(s => s.langue)
  const loc = localeIntl(langue)
  return {
    langue,
    montant: (v: number, devise = 'CHF') =>
      // La position du symbole change avec la langue : le français suisse le met
      // après, l'allemand et l'italien devant. Intl le sait.
      new Intl.NumberFormat(loc, { style: 'currency', currency: devise }).format(v),
    nombre: (v: number, decimales = 2) =>
      new Intl.NumberFormat(loc, {
        minimumFractionDigits: decimales, maximumFractionDigits: decimales,
      }).format(v),
    date: (iso: string | Date) => {
      const d = typeof iso === 'string' ? new Date(iso) : iso
      if (isNaN(d.getTime())) return ''
      return new Intl.DateTimeFormat(loc, {
        day: '2-digit', month: '2-digit', year: 'numeric',
      }).format(d)
    },
    pluriel: (n: number, un: string, plusieurs: string) => plural(n, langue, un, plusieurs),
  }
}
