// LedgerAlps — Client API centralisé (Axios + intercepteurs JWT)

import axios, { type AxiosInstance } from 'axios'
import { useAuthStore } from '@/store/auth'

const BASE_URL = import.meta.env.VITE_API_URL ?? '/api/v1'

export const api: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  headers: { 'Content-Type': 'application/json' },
  timeout: 30_000,
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

    const { refreshToken, setAccessToken, logout } = useAuthStore.getState()
    if (!refreshToken) {
      logout()
      window.location.href = '/login'
      return Promise.reject(error)
    }

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
        headers: { Authorization: `Bearer ${refreshToken}` },
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
  refresh:  (refreshToken: string) =>
    api.post('/auth/refresh', null, { headers: { Authorization: `Bearer ${refreshToken}` } }),
  logout:   (refreshToken: string) =>
    api.post('/auth/logout', null, { headers: { Authorization: `Bearer ${refreshToken}` } }),
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
export const contactsApi = {
  list:   (params?: { contact_type?: string; page?: number; page_size?: number }) =>
    api.get('/contacts', { params }),
  get:    (id: string)           => api.get(`/contacts/${id}`),
  create: (data: unknown)        => api.post('/contacts', data),
  update: (id: string, data: unknown) => api.patch(`/contacts/${id}`, data),
}

// ─── Factures ─────────────────────────────────────────────────────────────────
export const invoicesApi = {
  list: (params?: { status?: string; page?: number; page_size?: number }) =>
    api.get('/invoices', { params }),
  get:        (id: string)                    => api.get(`/invoices/${id}`),
  create:     (data: unknown)                 => api.post('/invoices', data),
  update:     (id: string, data: unknown)    => api.patch(`/invoices/${id}`, data),
  transition: (id: string, status: string)   =>
    api.post(`/invoices/${id}/transition`, { status }),
  // Alias kept for compatibility with pages that use updateStatus
  updateStatus: (id: string, status: string) =>
    api.post(`/invoices/${id}/transition`, { status }),
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
  uploadLogo: (file: File) => {
    const form = new FormData()
    form.append('logo', file)
    // Do NOT set Content-Type manually — Axios must add the multipart boundary automatically.
    return api.post('/settings/logo', form)
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
