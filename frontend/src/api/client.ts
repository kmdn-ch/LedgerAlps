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

// Gestion des erreurs 401 → déconnexion
api.interceptors.response.use(
  (r) => r,
  async (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().logout()
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// ─── Auth ──────────────────────────────────────────────────────────────────────
export const authApi = {
  login:    (email: string, password: string) =>
    api.post('/auth/login', { email, password }),
  register: (data: { email: string; name: string; password: string }) =>
    api.post('/auth/register', data),
}

// ─── Comptes ──────────────────────────────────────────────────────────────────
export const accountsApi = {
  list:         ()                     => api.get('/accounts'),
  balance:      (number: string, asOf?: string) =>
    api.get(`/accounts/${number}/balance`, { params: { as_of: asOf } }),
  trialBalance: (asOf?: string)        => api.get('/accounts/trial-balance', { params: { as_of: asOf } }),
}

// ─── Journal ──────────────────────────────────────────────────────────────────
export const journalApi = {
  list:    (params?: { page?: number; page_size?: number; date_from?: string; date_to?: string; status?: string; reference?: string }) =>
    api.get('/journal', { params }),
  create:  (data: unknown) => api.post('/journal', data),
  post:    (id: string)    => api.post(`/journal/${id}/post`),
  reverse: (id: string, date: string) =>
    api.post(`/journal/${id}/reverse`, null, { params: { reversal_date: date } }),
}

// ─── Contacts ─────────────────────────────────────────────────────────────────
export const contactsApi = {
  list:   (params?: { contact_type?: string; page?: number; page_size?: number }) =>
    api.get('/contacts', { params }),
  get:    (id: string) => api.get(`/contacts/${id}`),
  create: (data: unknown) => api.post('/contacts', data),
  update: (id: string, data: unknown) => api.patch(`/contacts/${id}`, data),
}

// ─── Factures ─────────────────────────────────────────────────────────────────
export const invoicesApi = {
  list:         (status?: string, type = 'invoice') =>
    api.get('/invoices', { params: { status, document_type: type } }),
  get:          (id: string)                     => api.get(`/invoices/${id}`),
  create:       (data: unknown)                  => api.post('/invoices', data),
  transition: (id: string, status: string) =>
    api.post(`/invoices/${id}/transition`, { status }),
  // Legacy alias kept for Python backend compatibility
  updateStatus: (id: string, status: string, extra?: object) =>
    api.post(`/invoices/${id}/transition`, { status, ...extra }),
  downloadPDF:  (id: string) =>
    api.get(`/pdf/invoice/${id}`, { responseType: 'blob' }),
}

// ─── TVA ──────────────────────────────────────────────────────────────────────
export const vatApi = {
  compute: (amount: number, rate: number, included = 'excluded') =>
    api.post('/vat/compute', { amount, vat_rate: rate, included }),
  rates:   () => api.get('/vat/rates'),
}

// ─── Exports ──────────────────────────────────────────────────────────────────
export const exportsApi = {
  trialBalance:   (asOf?: string) =>
    api.get('/exports/trial-balance', { params: { as_of: asOf }, responseType: 'blob' }),
  generalLedger:  (start: string, end: string) =>
    api.get('/exports/general-ledger', { params: { start_date: start, end_date: end }, responseType: 'blob' }),
  journal:        (start: string, end: string) =>
    api.get('/exports/journal', { params: { start_date: start, end_date: end }, responseType: 'blob' }),
  legalArchive:   (fiscalYearId: string) =>
    api.post(`/exports/legal-archive/${fiscalYearId}`, null, { responseType: 'blob' }),
}

// ─── QR-facture ───────────────────────────────────────────────────────────────
export const qrApi = {
  generatePayload: (data: unknown) => api.post('/qr-invoice/generate-payload', data),
  generateRef:     (ref: string)   =>
    api.post('/qr-invoice/reference/generate-qrr', null, { params: { customer_ref: ref } }),
}

// ─── ISO 20022 ────────────────────────────────────────────────────────────────
export const isoApi = {
  generatePain001: (data: unknown) =>
    api.post('/iso20022/pain001', data, { responseType: 'blob' }),
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
