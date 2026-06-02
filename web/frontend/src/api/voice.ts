import { launcherFetch } from "@/api/http"

export async function fetchVoiceCapabilities(): Promise<{
  asr: boolean
  tts: boolean
  streaming: boolean
}> {
  const res = await launcherFetch("/api/voice/capabilities")
  return res.json()
}

export async function transcribeAudio(
  blob: Blob,
): Promise<{ text: string; language?: string; error?: string }> {
  const form = new FormData()
  form.append("file", blob, "recording.webm")

  const res = await launcherFetch("/api/voice/transcribe", {
    method: "POST",
    body: form,
    signal: AbortSignal.timeout(30_000),
  })

  return res.json()
}

export async function synthesizeSpeech(
  text: string,
  sessionId?: string,
  messageIndex?: number,
): Promise<{ audio_url: string }> {
  const res = await launcherFetch("/api/voice/synthesize", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      text,
      ...(sessionId && messageIndex != null ? { session_id: sessionId, message_index: messageIndex } : {}),
    }),
    signal: AbortSignal.timeout(60_000),
  })

  if (!res.ok) {
    throw new Error(`TTS failed: ${res.status}`)
  }

  return res.json()
}

export async function saveSessionAudioURL(
  sessionId: string,
  messageIndex: number,
  audioUrl: string,
): Promise<void> {
  await launcherFetch(`/api/sessions/${encodeURIComponent(sessionId)}/audio-url`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message_index: messageIndex, audio_url: audioUrl }),
  })
}

export async function fetchVersionURLs(): Promise<{ http_url: string; https_url: string }> {
  const res = await launcherFetch("/api/system/version")
  if (!res.ok) return { http_url: "", https_url: "" }
  const data = await res.json()
  return {
    http_url: data.http_url || "",
    https_url: data.https_url || "",
  }
}
