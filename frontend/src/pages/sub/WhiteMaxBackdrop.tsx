import type { CSSProperties } from 'react';

// Deterministic positions avoid layout changes between renders. CSS-only motion
// stops for reduced-motion and low-power preferences, without a canvas render loop.
const particles = Array.from(
  { length: 56 },
  (_, i) =>
    ({
      left: `${((i * 73 + 19) % 997) / 10}%`,
      top: `${((i * 131 + 7) % 991) / 10}%`,
      '--wm-dot-size': `${1 + (i % 4)}px`,
      '--wm-dot-delay': `${-(i % 17)}s`,
      '--wm-dot-duration': `${18 + (i % 13)}s`,
    }) as CSSProperties,
);

export default function WhiteMaxBackdrop() {
  return (
    <div className="wm-backdrop" aria-hidden="true">
      <div className="wm-glow" />
      <div className="wm-grid" />
      <div className="wm-particles">
        {particles.map((style, i) => (
          <span key={i} style={style} />
        ))}
      </div>
    </div>
  );
}
