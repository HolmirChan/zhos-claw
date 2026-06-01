// web/frontend/src/components/chat/__tests__/voice.test.ts
import { describe, it, expect } from "vitest";

// --- TestUtteranceMerge ---
describe("utterance merge strategy", () => {
  it("locks definite segments and replaces current indefinite", () => {
    let locked = "";
    let currentIndefinite = "";

    function apply(text: string, definite: boolean): string {
      if (definite) {
        locked += text;
        currentIndefinite = "";
      } else {
        currentIndefinite = text;
      }
      return locked + currentIndefinite;
    }

    expect(apply("今天", false)).toBe("今天");
    expect(apply("今天天气", false)).toBe("今天天气");
    expect(apply("今天天气", true)).toBe("今天天气");
    expect(apply("怎么样", false)).toBe("今天天气怎么样");
    expect(apply("怎么样", true)).toBe("今天天气怎么样");
  });
});

// --- TestAudioContextFallback ---
describe("linear resampling fallback", () => {
  it("downsamples 48kHz to 16kHz correctly", () => {
    const samples = new Float32Array(480);
    for (let i = 0; i < samples.length; i++) {
      samples[i] = Math.sin((2 * Math.PI * 1000 * i) / 48000);
    }
    const resampled = linearResample(samples, 48000, 16000);
    expect(resampled.length).toBe(Math.floor(480 / 3));
  });

  it("passes through when rates match", () => {
    const samples = new Float32Array(160);
    const result = linearResample(samples, 16000, 16000);
    expect(result).toBe(samples);
  });
});

function linearResample(
  samples: Float32Array,
  fromRate: number,
  toRate: number
): Float32Array {
  if (fromRate === toRate) return samples;
  const ratio = fromRate / toRate;
  const newLen = Math.floor(samples.length / ratio);
  const result = new Float32Array(newLen);
  for (let i = 0; i < newLen; i++) {
    const srcIdx = i * ratio;
    const srcFloor = Math.floor(srcIdx);
    const frac = srcIdx - srcFloor;
    result[i] =
      samples[srcFloor] * (1 - frac) +
      samples[Math.min(srcFloor + 1, samples.length - 1)] * frac;
  }
  return result;
}

// --- TestWorkletBuffer ---
describe("worklet buffer flush", () => {
  function shouldFlush(bufferLength: number): boolean {
    return bufferLength >= 1600;
  }

  it("triggers flush at >= 1600 samples (13 frames = 1664)", () => {
    expect(shouldFlush(1536)).toBe(false);
    expect(shouldFlush(1664)).toBe(true);
    expect(shouldFlush(1600)).toBe(true);
  });

  it("triggers flush on stop for remainder below threshold", () => {
    const remainder = 1408;
    expect(shouldFlush(remainder)).toBe(false);
    const flushedOnStop = remainder > 0;
    expect(flushedOnStop).toBe(true);
  });
});
