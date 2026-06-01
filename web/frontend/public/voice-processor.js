// web/frontend/public/voice-processor.js
class VoiceProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this._targetRate = 16000;
    this._buffer = new Float32Array(0);
  }

  process(inputs) {
    const input = inputs[0];
    if (!input || !input[0] || input[0].length === 0) return true;

    let samples = input[0]; // mono channel 0

    // Downsample if device rate != 16kHz (Safari / Bluetooth headset)
    if (sampleRate !== this._targetRate) {
      samples = this._linearResample(samples, sampleRate, this._targetRate);
    }

    // Accumulate buffer
    const newBuf = new Float32Array(this._buffer.length + samples.length);
    newBuf.set(this._buffer);
    newBuf.set(samples, this._buffer.length);
    this._buffer = newBuf;

    // Flush when >= 1600 samples (~100ms at 16kHz)
    if (this._buffer.length >= 1600) {
      const int16 = new Int16Array(this._buffer.length);
      for (let i = 0; i < this._buffer.length; i++) {
        const s = Math.max(-1, Math.min(1, this._buffer[i]));
        int16[i] = Math.round(s < 0 ? s * 0x8000 : s * 0x7FFF);
      }
      this.port.postMessage(int16.buffer, [int16.buffer]);
      this._buffer = new Float32Array(0);
    }

    return true;
  }

  // Linear interpolation resampling — sufficient for speech ASR (80–3400 Hz energy band)
  _linearResample(samples, fromRate, toRate) {
    if (fromRate === toRate) return samples;
    const ratio = fromRate / toRate;
    const newLen = Math.floor(samples.length / ratio);
    const result = new Float32Array(newLen);
    for (let i = 0; i < newLen; i++) {
      const srcIdx = i * ratio;
      const srcFloor = Math.floor(srcIdx);
      const frac = srcIdx - srcFloor;
      result[i] = samples[srcFloor] * (1 - frac) + samples[Math.min(srcFloor + 1, samples.length - 1)] * frac;
    }
    return result;
  }
}

registerProcessor("voice-processor", VoiceProcessor);
