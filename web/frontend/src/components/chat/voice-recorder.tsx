import { useState, useRef, useEffect, useCallback } from "react";

interface VoiceRecorderProps {
  onInterimText: (text: string, definite: boolean) => void;
  onTranscribed: (text: string) => void;
  onError?: (error: string) => void;
}

export function VoiceRecorder({ onInterimText, onTranscribed, onError }: VoiceRecorderProps) {
  const [elapsed, setElapsed] = useState(0);
  const [loading, setLoading] = useState(true);
  const wsRef = useRef<WebSocket | null>(null);
  const audioCtxRef = useRef<AudioContext | null>(null);
  const workletRef = useRef<AudioWorkletNode | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const autoStopRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  const lockedTextRef = useRef("");
  const currentTextRef = useRef("");

  const isSupported = typeof AudioContext !== "undefined" && typeof WebSocket !== "undefined";

  const cleanup = useCallback(() => {
    if (timerRef.current) { clearInterval(timerRef.current); timerRef.current = null; }
    if (autoStopRef.current) { clearTimeout(autoStopRef.current); autoStopRef.current = null; }
    streamRef.current?.getTracks().forEach((t) => t.stop());
    workletRef.current?.port.close();
    audioCtxRef.current?.close();
    wsRef.current?.close();
    wsRef.current = null;
    audioCtxRef.current = null;
    workletRef.current = null;
    streamRef.current = null;
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    startRecording();
    return () => {
      mountedRef.current = false;
      cleanup();
    };
  }, []);

  const startRecording = useCallback(async () => {
    try {
      lockedTextRef.current = "";
      currentTextRef.current = "";
      const audioCtx = new AudioContext({ sampleRate: 16000 });
      audioCtxRef.current = audioCtx;

      await audioCtx.audioWorklet.addModule("/voice-processor.js");

      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { sampleRate: { ideal: 16000 }, channelCount: 1 },
      });
      streamRef.current = stream;

      const source = audioCtx.createMediaStreamSource(stream);
      const workletNode = new AudioWorkletNode(audioCtx, "voice-processor");
      workletRef.current = workletNode;
      source.connect(workletNode);

      const protocol = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(`${protocol}//${location.host}/api/voice/stream`);
      ws.binaryType = "arraybuffer";
      wsRef.current = ws;

      ws.onopen = () => {
        setLoading(false);
        workletNode.port.onmessage = (e: MessageEvent) => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(e.data);
          }
        };
      };

      ws.onmessage = (e) => {
        if (!mountedRef.current) return;
        try {
          const msg = JSON.parse(e.data);
          if (msg.type === "partial") {
            if (msg.definite) {
              lockedTextRef.current += msg.text;
              currentTextRef.current = "";
            } else {
              currentTextRef.current = msg.text;
            }
            onInterimText(lockedTextRef.current + currentTextRef.current, msg.definite);
          } else if (msg.type === "final") {
            onTranscribed(msg.text);
          } else if (msg.type === "error") {
            onError?.(msg.message);
          }
        } catch {}
      };

      ws.onerror = () => onError?.("WebSocket 连接失败");

      setElapsed(0);
      timerRef.current = setInterval(() => {
        setElapsed((prev) => {
          if (prev >= 59) {
            cleanup();
            return prev;
          }
          return prev + 1;
        });
      }, 1000);

      autoStopRef.current = setTimeout(() => {
        if (mountedRef.current) {
          cleanup();
        }
      }, 60_000);
    } catch {
      setLoading(false);
    }
  }, [onInterimText, onTranscribed, onError, cleanup]);

  if (!isSupported) return null;

  return (
    <button
      type="button"
      disabled
      className="voice-mic-btn recording"
      title="录音中"
    >
      {loading ? "..." : `${elapsed}s`}
    </button>
  );
}
