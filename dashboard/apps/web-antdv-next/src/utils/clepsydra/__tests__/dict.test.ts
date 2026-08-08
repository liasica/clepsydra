import { describe, expect, it } from 'vitest';

import { BILL_STATUS, DEMAND_STATUS, tagColor } from '../dict';

describe('状态字典', () => {
  it('需求 6 态齐全且动作按角色区分', () => {
    expect(Object.keys(DEMAND_STATUS)).toEqual([
      'draft',
      'pending_estimate',
      'confirmed',
      'in_progress',
      'pending_acceptance',
      'accepted',
    ]);
    expect(DEMAND_STATUS.draft.actions.admin).toEqual([
      'edit',
      'submitEstimate',
      'delete',
    ]);
    expect(DEMAND_STATUS.draft.actions.client).toEqual(['edit']);
    expect(DEMAND_STATUS.pending_estimate.actions.admin).toEqual([
      'edit',
      'submitEstimate',
      'confirmEstimate',
      'delete',
    ]);
    expect(DEMAND_STATUS.pending_estimate.actions.client).toEqual([
      'edit',
      'confirmEstimate',
    ]);
    expect(DEMAND_STATUS.confirmed.actions.admin).toEqual(['start', 'delete']);
    expect(DEMAND_STATUS.in_progress.actions.admin).toEqual([
      'finish',
      'delete',
    ]);
    expect(DEMAND_STATUS.pending_acceptance.actions.admin).toEqual([
      'accept',
      'delete',
    ]);
    expect(DEMAND_STATUS.pending_acceptance.actions.client).toEqual(['accept']);
    expect(DEMAND_STATUS.accepted.actions.admin).toEqual(['delete']);
  });

  it('删除为超管专属，任何状态都不开放给需求方', () => {
    for (const [status, meta] of Object.entries(DEMAND_STATUS)) {
      expect(meta.actions.admin, `需求 ${status}`).toContain('delete');
      expect(meta.actions.client, `需求 ${status}`).not.toContain('delete');
    }
  });

  it('账单 3 态齐全且动作按角色区分', () => {
    expect(Object.keys(BILL_STATUS)).toEqual(['pending', 'unpaid', 'paid']);
    expect(BILL_STATUS.pending.actions.admin).toEqual([
      'confirm',
      'waive',
      'addItem',
      'removeItem',
    ]);
    expect(BILL_STATUS.pending.actions.client).toEqual(['confirm']);
    expect(BILL_STATUS.unpaid.actions.admin).toEqual([
      'pay',
      'waive',
      'addItem',
      'removeItem',
    ]);
    expect(BILL_STATUS.unpaid.actions.client).toEqual([]);
    expect(BILL_STATUS.paid.actions.admin).toEqual([]);
  });

  it('超级管理员拥有全部权限：admin 动作恒为 client 的超集', () => {
    for (const [status, meta] of Object.entries(DEMAND_STATUS)) {
      for (const action of meta.actions.client) {
        expect(meta.actions.admin, `需求 ${status}`).toContain(action);
      }
    }
    for (const [status, meta] of Object.entries(BILL_STATUS)) {
      for (const action of meta.actions.client) {
        expect(meta.actions.admin, `账单 ${status}`).toContain(action);
      }
    }
  });

  it('tagColor 将 Element Plus 语义色映射为 antdv-next Tag 预设状态色', () => {
    expect(tagColor('info')).toBe('default');
    expect(tagColor('primary')).toBe('processing');
    expect(tagColor('warning')).toBe('warning');
    expect(tagColor('success')).toBe('success');
    expect(tagColor('danger')).toBe('error');
  });
});
