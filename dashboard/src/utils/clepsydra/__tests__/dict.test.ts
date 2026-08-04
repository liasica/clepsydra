import { describe, expect, it } from 'vitest'
import { BILL_STATUS, DEMAND_STATUS } from '../dict'

describe('状态字典', () => {
  it('需求 6 态齐全且动作按角色区分', () => {
    expect(Object.keys(DEMAND_STATUS)).toEqual([
      'draft',
      'pending_estimate',
      'confirmed',
      'in_progress',
      'pending_acceptance',
      'accepted'
    ])
    expect(DEMAND_STATUS.draft.actions.admin).toEqual(['edit', 'submitEstimate'])
    expect(DEMAND_STATUS.draft.actions.client).toEqual([])
    expect(DEMAND_STATUS.pending_estimate.actions.client).toEqual(['confirmEstimate'])
    expect(DEMAND_STATUS.confirmed.actions.admin).toEqual(['start'])
    expect(DEMAND_STATUS.in_progress.actions.admin).toEqual(['finish'])
    expect(DEMAND_STATUS.pending_acceptance.actions.client).toEqual(['accept'])
    expect(DEMAND_STATUS.accepted.actions.admin).toEqual([])
  })

  it('账单 3 态齐全且动作按角色区分', () => {
    expect(Object.keys(BILL_STATUS)).toEqual(['draft', 'pending', 'confirmed'])
    expect(BILL_STATUS.draft.actions.admin).toEqual(['regenerate', 'waive', 'share'])
    expect(BILL_STATUS.pending.actions.admin).toEqual(['revoke'])
    expect(BILL_STATUS.pending.actions.client).toEqual(['confirm'])
    expect(BILL_STATUS.confirmed.actions.admin).toEqual([])
  })
})
