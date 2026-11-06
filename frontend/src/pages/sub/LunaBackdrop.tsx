import { useEffect, useState } from 'react';
import type { CSSProperties } from 'react';

// Fixed positions keep the sky stable; only opacity and small transforms animate.
const stars = Array.from(
  { length: 48 },
  (_, i) =>
    ({
      left: `${((i * 73 + 19) % 997) / 10}%`,
      top: `${((i * 131 + 7) % 991) / 10}%`,
      '--star-size': `${i % 7 === 0 ? 3 : i % 3 === 0 ? 2 : 1}px`,
      '--star-delay': `${-(i % 19)}s`,
      '--star-duration': `${4 + (i % 9)}s`,
    }) as CSSProperties,
);

export default function LunaBackdrop() {
  const [paused, setPaused] = useState(document.hidden);
  useEffect(() => {
    const onVisibility = () => setPaused(document.hidden);
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, []);

  return (
    <div className="luna-backdrop" aria-hidden="true" data-paused={paused}>
      <div className="luna-stars">
        {stars.map((style, i) => (
          <span key={i} style={style} />
        ))}
      </div>
      <div className="luna-moon-orbit">
        <div className="luna-moon">
          <span className="luna-moon-shade" />
        </div>
      </div>
    </div>
  );
}
