// Ramener un logo d'entreprise au format de l'interface, avant l'envoi.
//
// # Pourquoi le faire aussi ici, alors que le serveur le refait
//
// Le serveur est ce qui TIENT la règle : la route est ouverte à qui forge une
// requête, et c'est elle qui décide de ce qui entre en base. Mais envoyer huit
// mégaoctets pour qu'ils reviennent en trente kilooctets fait attendre sans
// raison, et sur une connexion lente donne l'impression que l'envoi a échoué.
//
// Les deux ne font donc pas la même chose : celui-ci sert le confort, l'autre
// tient la règle. Aucun des deux ne rend l'autre superflu.
//
// # Ce qui n'est pas fait
//
// Aucune image n'est agrandie : un logo de 80 px reste à 80 px. L'étirer ne
// crée pas de détail, cela rend flou ce qui était net.

/** Le côté maximal, en pixels. Doit valoir `LogoTailleMax` côté serveur. */
export const LOGO_TAILLE_MAX = 300

/**
 * Lit un fichier image et rend une adresse de données dont aucun côté ne
 * dépasse `LOGO_TAILLE_MAX`.
 *
 * Le fichier est rendu INCHANGÉ s'il tient déjà dans la limite : le repasser
 * par un canevas ne gagnerait rien et retirerait au passage ce qu'un PNG
 * optimisé contient de mieux qu'un ré-encodage.
 */
export async function preparerLogo(fichier: File): Promise<{ dataURL: string; reduit: boolean }> {
  const original = await lireEnDataURL(fichier)
  const img = await chargerImage(original)

  if (img.naturalWidth <= LOGO_TAILLE_MAX && img.naturalHeight <= LOGO_TAILLE_MAX) {
    return { dataURL: original, reduit: false }
  }

  const [l, h] = tailleAjustee(img.naturalWidth, img.naturalHeight)
  const canevas = document.createElement('canvas')
  canevas.width = l
  canevas.height = h

  const ctx = canevas.getContext('2d')
  if (!ctx) return { dataURL: original, reduit: false } // pas de canevas : le serveur s'en chargera
  // Le navigateur sait interpoler proprement ; sans ces deux lignes, un logo
  // fait de traits fins ressort crénelé.
  ctx.imageSmoothingEnabled = true
  ctx.imageSmoothingQuality = 'high'
  ctx.drawImage(img, 0, 0, l, h)

  // PNG et non JPEG : un logo a souvent un fond transparent, que le JPEG
  // remplacerait par du noir.
  return { dataURL: canevas.toDataURL('image/png'), reduit: true }
}

/** Les dimensions qui tiennent dans le carré, sans déformer. */
export function tailleAjustee(l: number, h: number): [number, number] {
  if (l <= LOGO_TAILLE_MAX && h <= LOGO_TAILLE_MAX) return [l, h]
  return l >= h
    ? [LOGO_TAILLE_MAX, Math.max(1, Math.round((h * LOGO_TAILLE_MAX) / l))]
    : [Math.max(1, Math.round((l * LOGO_TAILLE_MAX) / h)), LOGO_TAILLE_MAX]
}

function lireEnDataURL(fichier: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const lecteur = new FileReader()
    lecteur.onload = e => resolve(e.target?.result as string)
    lecteur.onerror = () => reject(new Error('lecture impossible'))
    lecteur.readAsDataURL(fichier)
  })
}

function chargerImage(dataURL: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = () => reject(new Error('image illisible'))
    img.src = dataURL
  })
}
