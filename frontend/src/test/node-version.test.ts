import { describe, expect, it } from 'vitest';

import { compareVersions, isNewerRelease } from '@/lib/nodes/version';

describe('node release versions', () => {
  it('orders release tags like the node does', () => {
    expect(compareVersions('v1.10.0', 'v1.9.0')).toBe(1);
    expect(compareVersions('v1.2.3', 'v1.2.3')).toBe(0);
    expect(compareVersions('v1.2.3-rc.1', 'v1.2.3')).toBe(-1);
    expect(compareVersions('dev', 'v0.0.1')).toBe(-1);
    expect(compareVersions('dev', 'dev')).toBe(0);
  });

  it('offers an update only for a real, newer release', () => {
    expect(isNewerRelease('v1.2.0', 'v1.1.0')).toBe(true);
    expect(isNewerRelease('v1.2.0', 'dev')).toBe(true);
    expect(isNewerRelease('v1.2.0', 'v1.2.0')).toBe(false);
    expect(isNewerRelease('v1.1.0', 'v1.2.0')).toBe(false);
    expect(isNewerRelease('', 'v1.0.0')).toBe(false);
    expect(isNewerRelease('latest', 'v1.0.0')).toBe(false);
  });
});
