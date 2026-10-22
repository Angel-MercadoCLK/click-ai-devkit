// The Click "spark" as a small character: the orange core with a face, burst rays, the white companion star and a dust
// dot (the same parts as the devkit's terminal logo, internal/menu/menu.go renderSpark). A dark disc behind it keeps
// the white star and the orange core readable over any desktop background; the ring around the disc is the capture
// indicator, and the inner dashed ring shows that "Voz" is listening, so listening never hides capture.

const RAYS = Array.from({ length: 8 }, (_, index) => {
  const angle = (index * Math.PI) / 4;
  return { x1: 48 + Math.cos(angle) * 28, y1: 48 + Math.sin(angle) * 28, x2: 48 + Math.cos(angle) * 35, y2: 48 + Math.sin(angle) * 35 };
});

export function Spark() {
  return (
    <svg className="spark" viewBox="0 0 96 96" width="96" height="96" aria-hidden="true" focusable="false">
      <circle className="spark-disc" cx="48" cy="48" r="43" />
      <circle className="spark-ring" cx="48" cy="48" r="45.5" fill="none" />
      <circle className="spark-listen" cx="48" cy="48" r="39" fill="none" />
      <g className="spark-rays">
        {RAYS.map((ray, index) => <line key={index} x1={ray.x1} y1={ray.y1} x2={ray.x2} y2={ray.y2} />)}
      </g>
      <g className="spark-body">
        <path className="spark-points" d="M48 20 Q52 44 76 48 Q52 52 48 76 Q44 52 20 48 Q44 44 48 20Z" />
        <circle className="spark-core" cx="48" cy="48" r="14" />
        <g className="spark-eyes">
          <ellipse cx="43" cy="46" rx="2.2" ry="2.8" />
          <ellipse cx="53" cy="46" rx="2.2" ry="2.8" />
        </g>
        <path className="spark-mouth" d="M43.5 52 Q48 56.5 52.5 52" fill="none" />
      </g>
      <path className="spark-companion" d="M72 14 Q73 20 79 21 Q73 22 72 28 Q71 22 65 21 Q71 20 72 14Z" />
      <circle className="spark-dust" cx="22" cy="71" r="2" />
    </svg>
  );
}
