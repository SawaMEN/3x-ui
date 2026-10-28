import { useId } from 'react';

const buildings = [
  [0, 90, 110],
  [88, 180, 78],
  [160, 130, 105],
  [255, 225, 72],
  [320, 155, 95],
  [405, 250, 65],
  [465, 205, 85],
  [545, 280, 60],
  [600, 225, 85],
  [680, 300, 75],
  [750, 265, 70],
  [815, 200, 85],
  [895, 275, 70],
  [960, 190, 95],
  [1048, 245, 80],
  [1120, 165, 95],
  [1208, 215, 80],
  [1280, 125, 95],
  [1368, 175, 95],
  [1455, 80, 145],
];

export default function UndergroundBackdrop() {
  const id = useId().replace(/:/g, '');
  const paint = (name: string) => `url(#${id}-${name})`;

  return (
    <div className="underground-backdrop" aria-hidden="true">
      <svg className="underground-city" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMin slice">
        <defs>
          <linearGradient id={`${id}-sky`} x2="0" y2="1">
            <stop stopColor="#172a36" />
            <stop offset=".42" stopColor="#697b7e" />
            <stop offset=".7" stopColor="#c18c70" />
            <stop offset="1" stopColor="#233c43" />
          </linearGradient>
          <linearGradient id={`${id}-street`} x2="0" y2="1">
            <stop stopColor="#78908c" stopOpacity=".35" />
            <stop offset="1" stopColor="#0b151b" />
          </linearGradient>
          <linearGradient id={`${id}-mist`} x2="0" y2="1">
            <stop stopColor="#b9cac6" stopOpacity="0" />
            <stop offset=".5" stopColor="#b9cac6" stopOpacity=".42" />
            <stop offset="1" stopColor="#b9cac6" stopOpacity="0" />
          </linearGradient>
          <radialGradient id={`${id}-light`}>
            <stop stopColor="#ffe4bd" stopOpacity=".65" />
            <stop offset=".15" stopColor="#f8c490" stopOpacity=".25" />
            <stop offset="1" stopColor="#f8c490" stopOpacity="0" />
          </radialGradient>
          <pattern id={`${id}-windows`} width="34" height="38" patternUnits="userSpaceOnUse">
            <path d="M8 8h5v11H8zM23 8h4v11h-4z" fill="#9bbabd" opacity=".18" />
            <path d="M8 8h5v8H8z" fill="#edbd87" opacity=".5" />
          </pattern>
        </defs>
        <path d="M0 0h1600v900H0z" fill={paint('sky')} />
        <g fill="#324e5a" opacity=".85">
          {buildings.map(([x, y, width]) => (
            <path
              key={x}
              d={`M${x} ${y * 0.5 + 30}h${width}v${550 - y * 0.5}h-${width}zM${x + 12} ${y * 0.5 + 22}h${width - 24}v8h-${width - 24}z`}
            />
          ))}
        </g>
        <path
          d="M0 380h110v-75h80v100h105v-50h95v105h115v-80h85v100h100v-70h80v50h105v-85h95v65h115v-100h90v60h85v-140h100v85h100v-50h140v500H0z"
          fill="#29444e"
        />
        <path
          d="M0 110h170v50h80v440L470 900H0zM1600 60h-150v110h-95v440l-235 290h480z"
          fill="#122832"
        />
        <path
          d="M0 110h170v50h80v440L470 900H0zM1600 60h-150v110h-95v440l-235 290h480z"
          fill={paint('windows')}
        />
        <path
          d="M0 110h170v50h80v440M1600 60h-150v110h-95v440"
          fill="none"
          stroke="#b3c5bd"
          strokeOpacity=".18"
          strokeWidth="3"
        />
        <path d="M0 590 710 490h180l710 100v310H0z" fill={paint('street')} />
        <path
          d="M170 900 738 505M1430 900 865 505M450 900 775 510M1150 900 830 510"
          fill="none"
          stroke="#99b8b4"
          strokeOpacity=".22"
          strokeWidth="2"
        />
        <path
          d="m733 600 30-35m-100 135 35-45m-125 170 50-65m240-160-25-35m90 135-30-45m120 170-45-65"
          fill="none"
          stroke="#edbd87"
          strokeOpacity=".17"
          strokeWidth="6"
        />
        <path
          d="M0 215q800 195 1600-20M0 240q800 195 1600-20M140 0l120 600M1450 0l-130 600"
          fill="none"
          stroke="#10242e"
          strokeWidth="2"
        />
        <g fill="none" stroke="#10232b" strokeWidth="6">
          <path d="M325 900V340q0-28 28-28h75M1275 900V325q0-28-28-28h-75M575 680V410q0-16 16-16h43M1025 680V400q0-16-16-16h-43" />
        </g>
        <g fill={paint('light')}>
          <ellipse cx="410" cy="320" rx="165" ry="130" />
          <ellipse cx="1190" cy="305" rx="165" ry="130" />
          <ellipse cx="625" cy="400" rx="80" ry="70" />
          <ellipse cx="975" cy="390" rx="80" ry="70" />
        </g>
        <g fill="#ffe0ad">
          <rect x="390" y="312" width="38" height="4" rx="2" />
          <rect x="1172" y="297" width="38" height="4" rx="2" />
          <rect x="615" y="394" width="19" height="3" rx="1" />
          <rect x="966" y="384" width="19" height="3" rx="1" />
        </g>
        <path d="M0 320h1600v480H0z" fill={paint('mist')} />
      </svg>
      <div className="underground-fog underground-fog-far" />
      <div className="underground-fog underground-fog-near" />
      <div className="underground-vignette" />
    </div>
  );
}
