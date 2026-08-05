import { describe, expect, it } from 'vitest';

import { formatDate, formatDateTime } from '../date';

describe('日期格式化', () => {
  it('从 ISO 时间截取日期部分', () => {
    expect(formatDate('2026-08-04T10:20:30+08:00')).toBe('2026-08-04');
    expect(formatDate('2026-08-04')).toBe('2026-08-04');
    expect(formatDate(null)).toBe('—');
    expect(formatDate(undefined)).toBe('—');
  });

  it('将 ISO 时间格式化到分钟', () => {
    expect(formatDateTime('2026-08-04T10:20:30+08:00')).toBe(
      '2026-08-04 10:20',
    );
    expect(formatDateTime(null)).toBe('—');
  });
});
