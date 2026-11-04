// Release tags look like v1.2.3 (optionally v1.2.3-rc.1). Anything else (e.g. "dev") counts as older than any release.
const TAG = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z.]+))?$/;

function parse(v: string | undefined): { n: number[]; pre: string } | null {
  const m = v ? TAG.exec(v) : null;
  return m ? { n: [Number(m[1]), Number(m[2]), Number(m[3])], pre: m[4] ?? '' } : null;
}

// -1, 0 or 1, like the node's own comparison.
export function compareVersions(a: string | undefined, b: string | undefined): number {
  const x = parse(a);
  const y = parse(b);
  if (!x && !y) return 0;
  if (!x) return -1;
  if (!y) return 1;
  for (let i = 0; i < 3; i++) {
    if (x.n[i] !== y.n[i]) return x.n[i] < y.n[i] ? -1 : 1;
  }
  if (x.pre === y.pre) return 0;
  if (x.pre === '') return 1;
  if (y.pre === '') return -1;
  return x.pre < y.pre ? -1 : 1;
}

export function isNewerRelease(latest: string | undefined, current: string | undefined): boolean {
  return !!parse(latest) && compareVersions(latest, current) > 0;
}
