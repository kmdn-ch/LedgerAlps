// LedgerAlps — Client API centralisé (Axios + intercepteurs JWT)

import axios, { type AxiosInstance } from 'axios'
import { useAuthStore } from '@/store/auth'

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
api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().accessToken
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// Gestion des erreurs 401 — tenter un refresh avant de déconnecter
let isRefreshing = false
let failedQueue: Array<{ resolve: (v: unknown) => void; reject: (e: unknown) => void }> = []

function processQueue(error: unknown, token: string | null = null) {
  failedQueue.forEach((p) => (error ? p.reject(error) : p.resolve(token)))
  failedQueue = []
}

api.interceptors.response.use(
  (r) => r,
  async (error) => {
    const original = error.config
    if (error.response?.status !== 401 || original._retry) {
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
  register: (data: { email: string; name: string; password: string }) =>
    api.post('/auth/register', data),
  bootstrap:(data: { email: string; name: string; password: string }) =>
    api.post('/auth/bootstrap', data),
  // Ni refresh ni logout ne prennent de jeton : le cookie HttpOnly le porte,
  // et le code n'a aucun moyen de le lire — c'est précisément le but.
  refresh:  () => api.post('/auth/refresh', null),
  logout:   () => api.post('/auth/logout', null),
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
  create: (data: unknown)  => api.post('/journal', data),
  post:   (id: string)     => api.post(`/journal/${id}/post`),
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
  // Redémarre le serveur pour appliquer la restauration préparée. Le serveur
  // répond avant de s'arrêter, puis relance une copie de lui-même.
  restart: ()                                  => api.post('/system/restart'),
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
}

// Archive légale et export de réversibilité (CO art. 958f).
export const exportApi = {
  legalArchive: () =>
    api.get('/exports/legal-archive', { responseType: 'blob', timeout: 120_000 }),
}

// Rotation du secret de signature (point 6 de la roadmap).
export const securityApi = {
  rotateSecret: () => api.post('/settings/server/rotate-secret'),
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
  downloadPDF: (id: string) =>
    api.get(`/invoices/${id}/pdf`, { responseType: 'blob' }),
}

// ─── TVA ──────────────────────────────────────────────────────────────────────
export const vatApi = {
  rates: () => api.get('/vat/rates'),
}

// ─── ISO 20022 ────────────────────────────────────────────────────────────────
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
  uploadLogo: (file: File): Promise<import('axios').AxiosResponse> =>
    new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = (e) => {
        const logoData = e.target?.result as string
        api.post('/settings/logo', { logo_data: logoData }).then(resolve).catch(reject)
      }
      reader.onerror = () => reject(new Error('Impossible de lire le fichier'))
      reader.readAsDataURL(file)
    }),
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
