// Redémarrer le serveur depuis l'interface, puis y revenir.
//
// Deux pièges, tous deux rencontrés :
//
// 1. Sonder `/health` en relatif ne marche pas quand le schéma change. La page
//    est sur http://localhost:8000, le serveur repart en https sur le MÊME
//    port : la requête en clair se heurte à une poignée de main TLS et échoue
//    jusqu'au délai d'attente. Le bouton tournait alors indéfiniment.
//
// 2. Sonder la nouvelle adresse ne marche pas non plus. Avec un certificat
//    auto-signé, le navigateur refuse toute requête tant qu'il n'a pas montré
//    son avertissement — qu'il ne montre que pour une navigation, jamais pour
//    un fetch. Aucune attente ne rendra ce sondage possible.
//
// D'où cette approche : on attend que le serveur ACTUEL cesse de répondre —
// preuve que le redémarrage a bien commencé — puis on navigue vers la nouvelle
// adresse. La navigation laisse le navigateur afficher son avertissement si
// besoin, ce qu'un fetch ne permet pas.

/** URL de destination après redémarrage, selon le schéma qui sera servi. */
export function targetURLAfterRestart(tlsEnabled: boolean): string {
  const scheme = tlsEnabled ? 'https' : 'http'
  const { hostname, port } = window.location
  return `${scheme}://${hostname}${port ? `:${port}` : ''}/`
}

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
