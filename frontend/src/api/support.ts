import { apiClient } from './client'

/**
 * Two-way support chat (§15). The user's message is forwarded to the admin
 * Telegram bot; the admin answers with Telegram's "Reply" and the answer shows
 * up in the thread returned by `getThread`.
 */
export interface SupportMessage {
  id: number
  direction: 'user' | 'admin'
  body: string
  request_id: string
  model: string
  created_at: string
  read: boolean
}

export interface SupportThread {
  items: SupportMessage[]
  unread: number
}

export interface SendSupportMessagePayload {
  message: string
  request_id?: string
  model?: string
  page?: string
}

export interface SendSupportMessageResult {
  message: SupportMessage
  delivered: boolean
}

export async function sendSupportMessage(payload: SendSupportMessagePayload): Promise<SendSupportMessageResult> {
  const { data } = await apiClient.post<SendSupportMessageResult>('/support/messages', payload)
  return data
}

export async function getSupportThread(): Promise<SupportThread> {
  const { data } = await apiClient.get<SupportThread>('/support/messages')
  return data
}

export async function markSupportRead(): Promise<void> {
  await apiClient.post('/support/messages/read')
}
