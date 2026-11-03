import type { DestinationRow, TrafficPoint } from '@/api/queries/useClientInsights';

// A spreadsheet treats a cell starting with one of these as a formula.
function csvCell(value: string | number): string {
  let s = String(value);
  if (s !== '' && '=+-@\t\r'.includes(s[0])) s = `'${s}`;
  return /[",\n\r]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s;
}

export function destinationsToCsv(rows: DestinationRow[]): string {
  const head = [
    'destination',
    'network',
    'node',
    'upload_bytes',
    'download_bytes',
    'total_bytes',
    'connections',
  ];
  const lines = [head.join(',')];
  for (const r of rows) {
    lines.push([r.dest, r.network, r.node, r.up, r.down, r.total, r.conns].map(csvCell).join(','));
  }
  return lines.join('\r\n') + '\r\n';
}

// Fills a series onto a fixed grid so empty buckets read as zero instead of being skipped.
export function fillSeries(
  points: TrafficPoint[],
  from: number,
  to: number,
  step: number,
): { up: number[]; down: number[]; times: number[] } {
  const start = Math.floor(from / step) * step;
  const times: number[] = [];
  for (let t = start; t < to; t += step) times.push(t);
  const byTime = new Map(points.map((p) => [p.t, p]));
  return {
    times,
    up: times.map((t) => byTime.get(t)?.up ?? 0),
    down: times.map((t) => byTime.get(t)?.down ?? 0),
  };
}
