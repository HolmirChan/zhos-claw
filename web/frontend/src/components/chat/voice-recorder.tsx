import { useState, useRef, useCallback } from "react";
import { transcribeAudio } from "@/api/voice";

interface VoiceRecorderProps {
  onTranscribed: (text: string) => void;
}

export function VoiceRecorder({ onTranscribed }: VoiceRecorderProps) {
  const [recording, setRecording] = useState(false);
  const [loading, setLoading] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const mediaRecorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const isSupported = typeof MediaRecorder !== "undefined";

  const startRecording = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      // Safari doesn't support audio/webm, fall back to audio/mp4
      const mimeType = MediaRecorder.isTypeSupported("audio/webm")
        ? "audio/webm"
        : "audio/mp4";
      const blobType = mimeType;
      const recorder = new MediaRecorder(stream, { mimeType });
      mediaRecorderRef.current = recorder;
      chunksRef.current = [];

      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) chunksRef.current.push(e.data);
      };

      recorder.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop());
        setLoading(true);
        try {
          const blob = new Blob(chunksRef.current, { type: blobType });
          const result = await transcribeAudio(blob);
          if (result.text) {
            onTranscribed(result.text);
          }
        } finally {
          setLoading(false);
          setRecording(false);
          setElapsed(0);
        }
      };

      recorder.start();
      setRecording(true);
      setElapsed(0);
      timerRef.current = setInterval(() => {
        setElapsed((prev) => {
          if (prev >= 59) {
            recorder.stop();
            if (timerRef.current) clearInterval(timerRef.current);
            return prev;
          }
          return prev + 1;
        });
      }, 1000);

      // Auto-stop at 60 seconds
      const timeoutId = setTimeout(() => {
        if (recorder.state === "recording") {
          recorder.stop();
          if (timerRef.current) clearInterval(timerRef.current);
        }
      }, 60_000);
      // Clear timeout on normal stop
      const origOnStop = recorder.onstop;
      recorder.onstop = (e) => {
        clearTimeout(timeoutId);
        origOnStop?.call(recorder, e);
      };
    } catch {
      // User denied microphone permission
    }
  }, [onTranscribed]);

  const stopRecording = useCallback(() => {
    mediaRecorderRef.current?.stop();
    if (timerRef.current) clearInterval(timerRef.current);
  }, []);

  if (!isSupported) return null;

  return (
    <button
      type="button"
      onClick={recording ? stopRecording : startRecording}
      disabled={loading}
      className={`voice-mic-btn ${recording ? "recording" : ""} ${loading ? "loading" : ""}`}
      title={recording ? "停止录音" : "语音输入"}
    >
      {loading ? "..." : recording ? `${elapsed}s` : "🎤"}
    </button>
  );
}
