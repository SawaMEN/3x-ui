export default function UndergroundBackdrop() {
  return (
    <div className="underground-backdrop" aria-hidden="true">
      <svg
        className="underground-city"
        viewBox="0 0 1600 1000"
        preserveAspectRatio="xMidYMid slice"
      >
        <defs>
          <pattern id="underground-windows" width="32" height="42" patternUnits="userSpaceOnUse">
            <path d="M9 8h7v13H9z" fill="#d3a370" opacity=".23" />
            <path d="M24 8h3v13h-3z" fill="#91abb1" opacity=".12" />
          </pattern>
          <linearGradient id="underground-haze" x2="0" y2="1">
            <stop stopColor="#849699" stopOpacity="0" />
            <stop offset=".65" stopColor="#849699" stopOpacity=".23" />
            <stop offset="1" stopColor="#10191e" stopOpacity=".8" />
          </linearGradient>
          <radialGradient id="underground-lamp">
            <stop stopColor="#ffe2b5" stopOpacity=".5" />
            <stop offset=".2" stopColor="#e5ad78" stopOpacity=".18" />
            <stop offset="1" stopColor="#e5ad78" stopOpacity="0" />
          </radialGradient>
        </defs>
        <path
          d="M0 250h90v130h62V210h80v85h65v160h70V310h82v125h70V365h80v160h95V400h100v80h85V340h68v100h60V270h115v120h70V195h110v155h90V260h125v740H0z"
          fill="#33464c"
        />
        <path
          d="M0 120h165v95h70v415l160 370H0zM1600 80h-180v160h-85v380l-170 380h435z"
          fill="#142229"
        />
        <path
          d="M0 120h165v95h70v415l160 370H0zM1600 80h-180v160h-85v380l-170 380h435z"
          fill="url(#underground-windows)"
        />
        <path
          d="M0 120h165v95h70M1600 80h-180v160h-85"
          fill="none"
          stroke="#789095"
          strokeOpacity=".2"
          strokeWidth="3"
        />
        <path
          d="M395 1000 710 620h180l275 380M0 770l670-150m930 150L930 620"
          fill="none"
          stroke="#a3b1ab"
          strokeOpacity=".1"
          strokeWidth="2"
        />
        <path
          d="M0 330q800 170 1600-20M0 350q800 170 1600-20"
          fill="none"
          stroke="#0e1a20"
          strokeWidth="2"
        />
        <g fill="none" stroke="#111c22" strokeWidth="7">
          <path d="M280 1000V460q0-30 30-30h70M1320 1000V415q0-30-30-30h-70" />
        </g>
        <g fill="url(#underground-lamp)">
          <ellipse cx="371" cy="438" rx="180" ry="210" />
          <ellipse cx="1229" cy="393" rx="180" ry="210" />
        </g>
        <g fill="#f4c79e">
          <rect x="351" y="429" width="40" height="5" rx="2" />
          <rect x="1209" y="384" width="40" height="5" rx="2" />
        </g>
        <path d="M0 420h1600v580H0z" fill="url(#underground-haze)" />
      </svg>
      <div className="underground-fog" />
      <div className="underground-vignette" />
    </div>
  );
}
