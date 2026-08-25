// LedgerAlps — Client API centralisé (Axios + intercepteurs JWT)

import axios, { type AxiosInstance } from 'axios'
import { useAuthStore } from '@/store/auth'
import { traduire, useLangueStore } from '@/i18n/useT'
import { preparerLogo } from '@/utils/logoImage'

const BASE_URL = import.meta.env.VITE_API_URL ?? '/api/v1'

export const api: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  headers: { 'Content-Type': 'application/json' },
  timeout: 30_000,
  // Indispensable : le jeton de rafraîchissement est un cookie HttpOnly, et
  // sans ceci le navigateur ne l'attacherait pas aux appels à /auth/refresh.
  withCredentials: true,
})

// Injecter le token JWT dans chaque requête
//
// La langue voyage avec, dans `Accept-Language`. Un en-tête plutôt qu'un
// paramètre d'URL : il s'applique à TOUTES les routes sans que chacune ait à
// y penser, et une route ajoutée demain est couverte sans qu'on s'en occupe.
// C'est le motif qui avait laissé passer des écrans non traduits côté
// interface — on ne le refait pas côté serveur.
api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token) config.headers.Authorization = `Bearer ${token}`
  config.headers['Accept-Language'] = useLangueStore.getState().langue
  return config
})

// Gestion des erreurs 401 — tenter un refresh avant de déconnecter
let isRefreshing = false
let failedQueue: Array<{ resolve: (v: unknown) => void; reject: (e: unknown) => void }> = []

function processQueue(error: unknown, token: string | null = null) {
  failedQueue.forEach((p) => (error ? p.reject(error) : p.resolve(token)))
  failedQueue = []
}

// Les routes où un 401 veut dire « ce n'est pas le bon mot de passe », et NON
// « votre session a expiré ».
//
// La distinction n'est pas théorique. Sans elle, un mot de passe faux déclenchait
// tout le mécanisme de rafraîchissement : appel à /auth/refresh, échec, puis
// `window.location.href = '/login'` — un rechargement complet de la page qui
// EFFAÇAIT le message « identifiants incorrects » avant qu'on ait pu le lire.
// L'utilisateur voyait l'écran clignoter et se retrouvait devant un formulaire
// vide, sans savoir s'il s'était trompé ou si le logiciel avait planté.
const ROUTES_AUTH = ['/auth/login', '/auth/refresh', '/auth/mfa/verify', '/auth/change-password']

api.interceptors.response.use(
  (r) => r,
  async (error) => {
    const original = error.config
    if (error.response?.status !== 401 || original._retry) {
      return Promise.reject(error)
    }
    if (ROUTES_AUTH.some(r => (original.url ?? '').startsWith(r))) {
      return Promise.reject(error)
    }

    const { setAccessToken, logout } = useAuthStore.getState()

    // Plus de vérification d'un jeton en mémoire ici : c'est le cookie
    // HttpOnly qui porte le rafraîchissement, et le code n'y a pas accès.
    // On tente donc l'appel, et c'est le serveur qui tranche.
    if (isRefreshing) {
      return new Promise((resolve, reject) => {
        failedQueue.push({ resolve, reject })
      }).then((token) => {
        original.headers.Authorization = `Bearer ${token}`
        return api(original)
      })
    }

    original._retry = true
    isRefreshing = true

    try {
      const res = await axios.post(`${BASE_URL}/auth/refresh`, null, {
        withCredentials: true,
      })
      const newToken: string = res.data.access_token
      setAccessToken(newToken)
      processQueue(null, newToken)
      original.headers.Authorization = `Bearer ${newToken}`
      return api(original)
    } catch (refreshError) {
      processQueue(refreshError, null)
      logout()
      window.location.href = '/login'
      return Promise.reject(refreshError)
    } finally {
      isRefreshing = false
    }
  }
)

// ─── Auth ──────────────────────────────────────────────────────────────────────
export const authApi = {
  login:    (email: string, password: string) =>
    api.post('/auth/login', { email, password }),
  // Pas de `register` : l'inscription publique n'existe plus côté serveur. Elle
  // créait un compte comptable actif sans qu'aucun administrateur l'ait voulu.
  // Un compte se crée par un administrateur (POST /users) ; le premier passe par
  // `bootstrap`, qui ne fonctionne qu'une fois.
  bootstrap:(data: { email: string; name: string; password: string }) =>
    api.post('/auth/bootstrap', data),
  // Ni refresh ni logout ne prennent de jeton : le cookie HttpOnly le porte,
  // et le code n'a aucun moyen de le lire — c'est précisément le but.
  refresh:  () => api.post('/auth/refresh', null),
  logout:   () => api.post('/auth/logout', null),
  // Seule route qu'un compte au mot de passe temporaire puisse appeler : elle
  // vit hors du groupe filtré, sans quoi le compte serait enfermé.
  changePassword: (current: string, next: string) =>
    api.post('/auth/change-password', { current_password: current, new_password: next }),

  // ─── Second facteur ────────────────────────────────────────────────────────
  //
  // La vérification passe DÉLIBÉRÉMENT hors de l'instance intercepté.
  //
  // Deux raisons, et les deux se voient à l'usage : l'intercepteur de requête
  // écraserait le jeton d'attente par un jeton de session résiduel, et celui de
  // réponse traiterait le 401 d'un code faux comme une session expirée — il
  // tenterait un rafraîchissement, puis déconnecterait et renverrait vers
  // /login. Une faute de frappe sur six chiffres perdrait la connexion en cours.
  mfaVerify: (mfaToken: string, code: string, rememberDevice = false) =>
    axios.post(`${BASE_URL}/auth/mfa/verify`, { code, remember_device: rememberDevice }, {
      withCredentials: true,
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${mfaToken}` },
    }),
  mfaStatus:  () => api.get('/auth/mfa'),
  mfaSetup:   () => api.post('/auth/mfa/setup', null),
  mfaConfirm: (code: string) => api.post('/auth/mfa/confirm', { code }),
  mfaDisable: (password: string) => api.delete('/auth/mfa', { data: { password } }),
  // Ordinateurs de confiance : les lister, et les oublier tous depuis un autre
  // poste quand un ordinateur est perdu ou vendu.
  devices: () => api.get('/auth/devices'),
  forgetDevices: () => api.delete('/auth/devices'),
}

// ─── Comptes ──────────────────────────────────────────────────────────────────
export const accountsApi = {
  list:         ()                      => api.get('/accounts'),
  create:       (data: unknown)         => api.post('/accounts', data),
  balance:      (code: string)          => api.get(`/accounts/${code}/balance`),
  trialBalance: ()                      => api.get('/accounts/trial-balance'),
}

// ─── Journal ──────────────────────────────────────────────────────────────────
export const journalApi = {
  list:   (params?: {
    page?: number; page_size?: number
    date_from?: string; date_to?: string
    status?: string; reference?: string
  }) => api.get('/journal', { params }),
  // Le détail avec ses lignes : la liste ne rend qu'un total, et une écriture
  // qu'on ne peut pas relire ne se contrôle pas.
  get:    (id: string)     => api.get(`/journal/${id}`),
  create: (data: unknown)  => api.post('/journal', data),
  post:   (id: string)     => api.post(`/journal/${id}/post`),
}

// ─── Factures fournisseurs (achats) ───────────────────────────────────────────
export const supplierInvoicesApi = {
  list:   (params?: { status?: string; page?: number; page_size?: number }) =>
    api.get('/supplier-invoices', { params }),
  get:    (id: string)   => api.get(`/supplier-invoices/${id}`),
  create: (data: unknown) => api.post('/supplier-invoices', data),
  update: (id: string, data: unknown) => api.put(`/supplier-invoices/${id}`, data),
  transition: (id: string, status: string) =>
    api.post(`/supplier-invoices/${id}/transition`, { status }),
  // Vide la liste des paiements sans mentir aux livres : un brouillon est
  // supprime, une facture comptabilisee est EXTOURNEE puis marquee annulee.
  // Le serveur rend un verdict PAR facture — un lot partiel est le cas normal.
  cancel: (ids: string[], reason?: string) =>
    api.post('/supplier-invoices/cancel', { ids, reason }),
  // Lit le QR d'une facture deposee. N'enregistre RIEN : le serveur rend ce que
  // le code contient, l'utilisateur confirme.
  readQR: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return api.post('/supplier-invoices/read-qr', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 60_000,
    })
  },
}

// ─── Paiements fournisseurs (pain.001) ────────────────────────────────────────
//
// L'export ne transmet QUE des identifiants de factures : le serveur relit
// lui-même le créancier, l'IBAN, le montant et la référence dans les livres.
// Envoyer les montants depuis le navigateur reviendrait à laisser une page web
// dicter ce qui part à la banque.
export const paymentsApi = {
  payable: () => api.get('/payments/payable'),
  exportRun: (executionDate: string, supplierInvoiceIds: string[]) =>
    api.post('/payments/export',
      { execution_date: executionDate, supplier_invoice_ids: supplierInvoiceIds },
      { responseType: 'text' }),
}

// ─── Contacts ─────────────────────────────────────────────────────────────────
// ─── Sauvegardes ──────────────────────────────────────────────────────────────
// Créer un instantané est immédiat : SQLite écrit une copie cohérente d'une
// base en service. Restaurer ne l'est pas — cela remplace le fichier que le
// serveur a ouvert — donc la restauration est *préparée* puis appliquée au
// démarrage suivant. L'interface doit le dire, pas le masquer.
export const backupsApi = {
  list:   ()                                   => api.get('/backups'),
  create: (passphrase?: string)                => api.post('/backups', { passphrase: passphrase ?? '' }),
  stageRestore: (name: string, passphrase: string) =>
    api.post('/backups/restore', { name, passphrase, confirm: true }),
  cancelRestore: ()                            => api.delete('/backups/restore'),
  // La politique de chiffrement des sauvegardes automatiques. Elle est lue
  // séparément de la liste : elle décrit ce qui va se passer, pas ce qui existe.
  policy:      ()                              => api.get('/backups/policy'),
  setPolicy:   (passphrase: string, encryptExisting: boolean) =>
    api.put('/backups/policy', { passphrase, encrypt_existing: encryptExisting, acknowledged: true }),
  clearPolicy: ()                              => api.delete('/backups/policy?confirm=true'),
  // Redémarre le serveur pour appliquer une restauration ou une conversion
  // préparée. Le serveur répond avant de s'arrêter, puis relance une copie de
  // lui-même.
  restart: ()                                  => api.post('/system/restart'),
}

// Chiffrement de la base elle-même. Séparé des sauvegardes : ce sont deux
// protections distinctes, et les confondre dans un seul écran a déjà produit
// des questions du type « mes sauvegardes sont chiffrées, donc ma base aussi ? ».
export const databaseApi = {
  encryption:        ()                   => api.get('/database/encryption'),
  enableEncryption:  (recovery: string)   =>
    api.post('/database/encryption', { recovery_passphrase: recovery, acknowledged: true }),
  disableEncryption: ()                   => api.delete('/database/encryption?confirm=true'),
  cancelEncryption:  ()                   => api.delete('/database/encryption/pending'),
  recoverKey:        (recovery: string)   =>
    api.post('/database/encryption/recover', { recovery_passphrase: recovery }),
  changeRecovery:    (recovery: string)   =>
    api.put('/database/encryption/recovery', { recovery_passphrase: recovery }),
}

// ─── Maintenance & Système ────────────────────────────────────────────────────
// Lecture seule : ces vues montrent l'état des données, elles ne réparent rien.
// Une comptabilité incohérente se corrige par une écriture, pas par un bouton.
export const maintenanceApi = {
  integrity: () => api.get('/maintenance/integrity'),
  health:    () => api.get('/maintenance/health'),
  // Réglages réseau. config.json n'est écrit qu'au premier lancement : sans cet
  // écran, aucune option ajoutée depuis n'est atteignable.
  getServerSettings: () => api.get('/settings/server'),
  putServerSettings: (data: unknown) => api.put('/settings/server', data),
}

// La mise en route — ce qu'il reste à régler avant qu'une facture tienne
// debout. Le serveur applique les règles (SIX IG v2.4, ISO 13616) et ne renvoie
// que des états : les phrases sont au catalogue.
export const onboardingApi = {
  get: () => api.get('/onboarding'),
}

// Piste d'audit — la chaîne d'empreintes du CO art. 957a.
export const auditApi = {
  list: (params?: { limit?: number; offset?: number; order?: 'asc' | 'desc'; from?: string; to?: string }) =>
    api.get('/audit-logs', { params }),
  // Le serveur répond 409 quand la chaîne est rompue, et le corps de cette
  // réponse EST le rapport à afficher. Sans validateStatus, axios le rejetterait
  // et l'écran ne montrerait rien précisément dans le cas qui compte.
  verifyChain: () =>
    api.get('/audit-logs/verify-chain', {
      validateStatus: (s) => s === 200 || s === 409,
      timeout: 120_000, // parcours de toute la chaîne : dix ans de livres
    }),
  // Attestation Olico art. 9 : téléchargée, pas affichée — elle est destinée à
  // être transmise à une fiduciaire ou à un réviseur.
  attestationURL: () => `${BASE_URL}/audit-logs/attestation`,
  attestation: () =>
    api.get('/audit-logs/attestation', { responseType: 'blob', timeout: 120_000 }),
  // Vérifier une attestation qu'on nous présente. Le serveur seul a les livres :
  // il compare l'empreinte attestée à celle qu'ils portent au même maillon.
  verifyAttestation: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return api.post('/audit-logs/attestation/verify', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
      timeout: 120_000,
    })
  },
}

// Chiffre d'affaires groupable. La convention de calcul est renvoyée avec les
// chiffres (`basis`) : un total sans sa définition invite à le comparer à un
// autre calculé autrement.
export const revenueApi = {
  get: (params: { group_by: 'year' | 'month' | 'contact'; from?: string; to?: string }) =>
    api.get('/reports/revenue', { params }),
}

// Journal, grand livre et balance en CSV. Ce sont des LECTURES : un rôle en
// lecture seule doit pouvoir les produire — c'est même la raison d'être de ce
// rôle, remettre les livres à sa fiduciaire sans lui donner les clés.
export const accountingExportApi = {
  journal: (from: string, to: string) =>
    api.get('/exports/journal.csv', { params: { from, to }, responseType: 'blob' }),
  ledger: (from: string, to: string) =>
    api.get('/exports/ledger.csv', { params: { from, to }, responseType: 'blob' }),
  trialBalance: (from: string, to: string) =>
    api.get('/exports/trial-balance.csv', { params: { from, to }, responseType: 'blob' }),
}

// Comptabilité simplifiée — le « carnet du lait » du CO art. 957 al. 2.
//
// Une LECTURE, comme les exports ci-dessus : le carnet relit le journal et le
// présente sous la forme que la loi reconnaît pour une entreprise individuelle
// sous le seuil de 500 000 francs. Une fiduciaire en lecture seule doit pouvoir
// l'établir.
export const carnetApi = {
  // Les chiffres, pour l'écran : c'est ce qui permet de les regarder avant de
  // télécharger quoi que ce soit.
  lire: (from: string, to: string) =>
    api.get('/reports/simplified-accounting', { params: { from, to } }),
  csv: (from: string, to: string) =>
    api.get('/exports/simplified-accounting.csv', { params: { from, to }, responseType: 'blob' }),
  // Le PDF est le document que l'on tend à l'administration.
  pdf: (from: string, to: string) =>
    api.get('/reports/simplified-accounting.pdf', { params: { from, to }, responseType: 'blob' }),
}

// Archive légale et export de réversibilité (CO art. 958f).
export const exportApi = {
  legalArchive: () =>
    api.get('/exports/legal-archive', { responseType: 'blob', timeout: 120_000 }),
}

// Rotation du secret de signature (point 6 de la roadmap).
// Comptes et rôles. Administrateur seulement, et le serveur le revérifie : ces
// appels ne sont qu'une commodité de l'interface.
export const usersApi = {
  list:      ()  => api.get('/users'),
  create:    (u: { name: string; email: string; password: string; role: string }) =>
    api.post('/users', u),
  setRole:   (id: string, role: string)   => api.put(`/users/${id}/role`, { role }),
  setActive: (id: string, active: boolean) =>
    api.put(`/users/${id}/active`, { is_active: active }),
  // Deux gestes séparés, tracés séparément côté serveur : réunis, ils
  // permettraient à un administrateur de se substituer entièrement à un compte.
  resetPassword: (id: string) => api.post(`/users/${id}/reset-password`, null),
  removeMfa:     (id: string) => api.delete(`/users/${id}/mfa`),
}

export const securityApi = {
  rotateSecret: () => api.post('/settings/server/rotate-secret'),
  // Rotation de la clé de signature et déconnexion sur inactivité. Les deux se
  // lisent ensemble : ils bornent la même chose — la durée pendant laquelle une
  // session vaut quelque chose — par deux chemins différents.
  settings:     () => api.get('/settings/security'),
  saveSettings: (body: { rotation_days?: number; idle_logout_minutes?: number }) =>
    api.put('/settings/security', body),
}

export const contactsApi = {
  list:   (params?: { contact_type?: string; page?: number; page_size?: number }) =>
    api.get('/contacts', { params }),
  get:    (id: string)           => api.get(`/contacts/${id}`),
  // Anonymisation (nLPD art. 6 al. 4 et 32). Irréversible, et c'est le but :
  // c'est ce qui a été promis à la personne concernée.
  anonymise: (id: string)        => api.post(`/contacts/${id}/anonymise`),
  create: (data: unknown)        => api.post('/contacts', data),
  update: (id: string, data: unknown) => api.patch(`/contacts/${id}`, data),
}

// ─── Factures ─────────────────────────────────────────────────────────────────
export const invoicesApi = {
  list: (params?: {
    status?: string; page?: number; page_size?: number
    contact_id?: string; document_type?: string
    // Bornes sur la date d'émission. Filtrées côté serveur : la pagination
    // l'est aussi, donc un filtre client ne verrait que la page chargée.
    from?: string; to?: string
  }) => api.get('/invoices', { params }),
  get:        (id: string)                    => api.get(`/invoices/${id}`),
  create:     (data: unknown)                 => api.post('/invoices', data),
  update:     (id: string, data: unknown)    => api.patch(`/invoices/${id}`, data),
  transition: (id: string, status: string)   =>
    api.post(`/invoices/${id}/transition`, { status }),
  // Alias kept for compatibility with pages that use updateStatus
  updateStatus: (id: string, status: string) =>
    api.post(`/invoices/${id}/transition`, { status }),
  // Convertit une offre en facture. L'offre est conservée ; la facture créée
  // porte son propre numéro FA- et référence l'offre.
  convertQuote: (id: string, data?: { issue_date?: string; due_date?: string }) =>
    api.post(`/invoices/${id}/convert`, data ?? {}),
  // Enregistre qu'une offre a été refusée ou a expiré. « accepted » n'est pas
  // acceptée ici : on accepte une offre en la convertissant.
  setQuoteOutcome: (id: string, outcome: 'refused' | 'expired') =>
    api.post(`/invoices/${id}/outcome`, { outcome }),
  // Émet une note de crédit contre une facture. Corps vide = crédit total ;
  // fournir des lignes crédite une partie. Refusé (409) si le total des notes
  // de crédit dépasserait la facture.
  createCreditNote: (id: string, data?: { issue_date?: string; reason?: string; lines?: unknown[] }) =>
    api.post(`/invoices/${id}/credit-note`, data ?? {}),
  // PDF — Go endpoint: GET /invoices/:id/pdf
  // Un PDF si un seul document, un ZIP si plusieurs. La réponse est binaire
  // dans les deux cas : c'est l'en-tête Content-Type qui tranche.
  bulkPDF: (ids: string[]) =>
    api.post('/invoices/bulk-pdf', { ids }, { responseType: 'blob', timeout: 300_000 }),
  downloadPDF: (id: string) =>
    api.get(`/invoices/${id}/pdf`, { responseType: 'blob' }),
  // Dossier a deposer sur le portail de validation SIX : le payload exact du QR
  // et le bulletin, avec la marche a suivre. Le portail est la seule
  // verification qui fasse autorite sur la conformite du bulletin produit.
  sixValidation: (id: string) =>
    api.get(`/invoices/${id}/six-validation`, { responseType: 'blob' }),
}

// ─── TVA ──────────────────────────────────────────────────────────────────────
export const vatApi = {
  rates: () => api.get('/vat/rates'),
}

// ─── ISO 20022 ────────────────────────────────────────────────────────────────
// Rapprochement bancaire. Les écritures d'un relevé, leurs suggestions, et la
// décision de l'utilisateur. Aucune de ces routes n'encaisse quoi que ce soit :
// rapprocher identifie un versement, il ne solde pas une créance.
export const bankEntriesApi = {
  list:    (all = false) => api.get('/bank-entries' + (all ? '?all=true' : '')),
  match:   (id: string, invoiceId: string) =>
    api.put(`/bank-entries/${id}/match`, { invoice_id: invoiceId }),
  unmatch: (id: string) => api.delete(`/bank-entries/${id}/match`),
  ignore:  (id: string, ignored: boolean) =>
    api.put(`/bank-entries/${id}/ignore`, { ignored }),
}

export const isoApi = {
  // pain.001.001.09 — générer un fichier de paiement
  exportPain001: (data: {
    execution_date: string
    debtor_name: string
    debtor_iban: string
    debtor_bic?: string
    transactions: Array<{
      end_to_end_id: string
      creditor_name: string
      creditor_iban: string
      amount: number
      currency?: string
      reference?: string
      unstructured?: string
    }>
  }) => api.post('/payments/export', data, { responseType: 'blob' }),

  // camt.053.001.08 — importer un relevé bancaire
  importCamt053: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return api.post('/bank-statements/import', form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}

// ─── Exercices fiscaux ────────────────────────────────────────────────────────
export const fiscalYearsApi = {
  list:  ()                    => api.get('/fiscal-years'),
  // Déclarer un exercice décalé AVANT d'y comptabiliser : sans exercice
  // couvrant la date, LedgerAlps crée l'année civile.
  create: (data: { name: string; start_date: string; end_date: string }) =>
    api.post('/fiscal-years', data),
  close: (id: string)          => api.post(`/fiscal-years/${id}/close`),
  vatDeclaration: (params: { period_start: string; period_end: string; method: string }) =>
    api.post('/vat/declaration', params),
}

// ─── Stats ────────────────────────────────────────────────────────────────────
export const statsApi = {
  get: () => api.get('/stats'),
}

// ─── Health / version ─────────────────────────────────────────────────────────
export const healthApi = {
  get: () => axios.get('/health'),
}

// ─── Paramètres société ───────────────────────────────────────────────────────
export const settingsApi = {
  getCompany: () => api.get('/settings/company'),
  putCompany: (data: unknown) => api.put('/settings/company', data),
  // Le logo est réduit à 300 px de côté AVANT l'envoi — voir utils/logoImage.
  // Le serveur le refait de son côté : c'est lui qui décide de ce qui entre en
  // base, et cette route reste ouverte à qui forge une requête.
  uploadLogo: async (file: File): Promise<import('axios').AxiosResponse> => {
    let prepare: { dataURL: string; reduit: boolean }
    try {
      prepare = await preparerLogo(file)
    } catch {
      throw new Error(traduire('ui.fichierIllisible'))
    }
    const res = await api.post('/settings/logo', { logo_data: prepare.dataURL })
    // « Réduit » vaut pour l'utilisateur dès que l'image a rétréci quelque
    // part. Le serveur ne voit que ce qu'on lui envoie : quand le navigateur a
    // déjà fait le travail, il reçoit une image conforme et répond « rien à
    // faire » — ce qui est vrai pour lui, et faux pour celui qui vient de
    // déposer une photo de 1600 px.
    res.data = { ...res.data, resized: res.data?.resized === true || prepare.reduit }
    return res
  },
  deleteLogo: () => api.delete('/settings/logo'),
}

// ─── Utilitaire — télécharger un blob ─────────────────────────────────────────
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a   = document.createElement('a')
  a.href    = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

// ─── Avis de conformité ───────────────────────────────────────────────────────
// Servis depuis le flux embarqué dans le binaire : fonctionne hors ligne.
export const complianceApi = {
  advisories: (lang = 'fr') => api.get('/compliance/advisories', { params: { lang } }),
}

// Vérification de mise à jour — unique appel sortant de LedgerAlps.
// Désactivable côté serveur (update_check: false) ; ne transmet aucune donnée.
export const updateApi = {
  check: () => api.get('/compliance/update-check'),
}
