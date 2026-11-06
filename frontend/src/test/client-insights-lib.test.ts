import { describe, expect, it } from 'vitest';

import { destinationsToCsv, fillSeries } from '@/lib/clients/insights';

describe('client insights helpers', () => {
  it('neutralises formulas and quotes cells in CSV', () => {
    const csv = destinationsToCsv([
      {
        dest: '=cmd|x',
        network: 'tcp',
        nodeId: 1,
        node: 'a,b',
        up: 1,
        down: 2,
        conns: 3,
        total: 3,
        lastSeen: 1760000000,
        devices: [
          {
            hwid: 'abcd1234ffff',
            short: 'abcd1234',
            label: 'Pixel 8',
            up: 1,
            down: 2,
            lastSeen: 1760000000,
          },
          { hwid: '', short: '', label: '', up: 0, down: 0, lastSeen: 0 },
        ],
      },
    ]);
    const lines = csv.trim().split('\r\n');
    expect(lines[0]).toBe(
      'destination,network,node,upload_bytes,download_bytes,total_bytes,connections,last_used_utc,devices',
    );
    expect(lines[1]).toBe(
      '\'=cmd|x,tcp,"a,b",1,2,3,3,2025-10-09T08:53:20.000Z,Pixel 8 (abcd1234); unknown',
    );
  });

  it('fills empty buckets with zero', () => {
    const r = fillSeries([{ t: 3600, up: 5, down: 6 }], 0, 3 * 3600, 3600);
    expect(r.times).toEqual([0, 3600, 7200]);
    expect(r.up).toEqual([0, 5, 0]);
    expect(r.down).toEqual([0, 6, 0]);
  });
});
