// Redémarrer le serveur depuis l'interface, puis y revenir.
//
// On attend que le serveur ACTUEL cesse de répondre — preuve que le redémarrage a bien commencé —
// puis on navigue vers la même adresse : le serveur repart sur le même port, en HTTP, sur la boucle
// locale (il n'y a plus de TLS ni d'adresse d'écoute réglable).

/** Vrai tant que le serveur en cours répond. */
async function stillAlive(): Promise<boolean> {
  try {
    const res = await fetch('/health', { cache: 'no-store' })
    return res.ok
  } catch {
    return false
  }
}

/**
 * Attend l'arrêt du serveur actuel, puis navigue vers `target`.
 *
 * Ne renvoie jamais en cas de succès : la page est remplacée. Si le serveur ne
 * s'arrête pas dans le délai imparti, on navigue quand même — l'utilisateur
 * verra une erreur de connexion franche plutôt qu'un indicateur qui tourne sans
 * fin, et pourra recharger.
 */
export async function waitForShutdownThenGo(target: string, timeoutMs = 20_000): Promise<void> {
  const deadline = Date.now() + timeoutMs

  // Phase 1 : le serveur doit disparaître. C'est le seul signal fiable que le
  // redémarrage a commencé.
  while (Date.now() < deadline) {
    if (!(await stillAlive())) break
    await new Promise(r => setTimeout(r, 500))
  }

  // Phase 2 : lui laisser le temps de réouvrir sa socket avant de naviguer.
  // Arriver trop tôt donnerait une erreur de connexion alors que tout va bien.
  await new Promise(r => setTimeout(r, 2000))
  window.location.assign(target)
}
