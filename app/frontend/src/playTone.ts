/** Short UI feedback tones (Web Audio API, no assets). */
export function playVerifyTone(success: boolean): void {
  try {
    const ctx = new AudioContext();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = "sine";
    osc.frequency.value = success ? 880 : 220;
    gain.gain.value = 0.08;
    osc.connect(gain);
    gain.connect(ctx.destination);
    const now = ctx.currentTime;
    osc.start(now);
    osc.stop(now + (success ? 0.12 : 0.2));
    osc.onended = () => void ctx.close();
  } catch {
    // ignore if audio blocked
  }
}
