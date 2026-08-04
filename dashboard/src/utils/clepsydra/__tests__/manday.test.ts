import { describe, expect, it } from 'vitest'
import { formatAmount, formatManday, halfDaysToManday, mandayToHalfDays } from '../manday'

describe('人天换算', () => {
  it('半天数转人天', () => {
    expect(halfDaysToManday(16)).toBe(8)
    expect(halfDaysToManday(1)).toBe(0.5)
    expect(halfDaysToManday(0)).toBe(0)
  })

  it('人天转半天数', () => {
    expect(mandayToHalfDays(8)).toBe(16)
    expect(mandayToHalfDays(0.5)).toBe(1)
  })

  it('格式化人天，空值显示占位符', () => {
    expect(formatManday(16)).toBe('8 人天')
    expect(formatManday(1)).toBe('0.5 人天')
    expect(formatManday(null)).toBe('—')
    expect(formatManday(undefined)).toBe('—')
  })

  it('格式化金额为千分位元，空值显示占位符', () => {
    expect(formatAmount(21600)).toBe('¥21,600')
    expect(formatAmount(0)).toBe('¥0')
    expect(formatAmount(null)).toBe('—')
  })
})
