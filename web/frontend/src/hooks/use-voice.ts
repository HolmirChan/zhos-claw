import { useAtom, useSetAtom } from "jotai";
import { useEffect, useCallback } from "react";
import { fetchVoiceCapabilities } from "@/api/voice";
import {
  voiceInputAtom,
  voiceOutputAtom,
  voiceAsrAvailableAtom,
  voiceTtsAvailableAtom,
} from "@/store/voice";

export function useVoice() {
  const [inputEnabled, setInputEnabled] = useAtom(voiceInputAtom);
  const [outputEnabled, setOutputEnabled] = useAtom(voiceOutputAtom);
  const [asrAvailable, setAsrAvailable] = useAtom(voiceAsrAvailableAtom);
  const [ttsAvailable, setTtsAvailable] = useAtom(voiceTtsAvailableAtom);

  useEffect(() => {
    fetchVoiceCapabilities().then((caps) => {
      setAsrAvailable(caps.asr);
      setTtsAvailable(caps.tts);
    }).catch(() => {});
  }, []);

  const toggleInput = useCallback(() => setInputEnabled((v) => !v), []);
  const toggleOutput = useCallback(() => setOutputEnabled((v) => !v), []);

  return { inputEnabled, outputEnabled, asrAvailable, ttsAvailable, toggleInput, toggleOutput };
}
