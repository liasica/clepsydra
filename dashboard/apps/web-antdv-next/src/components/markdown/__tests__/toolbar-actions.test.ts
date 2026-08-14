import type { Ctx } from '@milkdown/kit/ctx';

import { describe, expect, it } from 'vitest';

import { EMPTY_TOOLBAR_STATE, readToolbarState } from '../toolbar-actions';

describe('readToolbarState', () => {
  it('editorViewCtx 未就绪（view.state 为空）时回落空状态而不是抛错', () => {
    // Milkdown 在 EditorView 构造完成后才往 ctx 注入真实 view；构造期间 NodeView
    // 派发的事务会同步触发 listener → readToolbarState，此时拿到的是占位对象。
    // 这里模拟该时刻：ctx.get 返回没有 state 的对象
    const ctx = { get: () => ({}) } as unknown as Ctx;

    expect(() => readToolbarState(ctx)).not.toThrow();
    expect(readToolbarState(ctx)).toEqual(EMPTY_TOOLBAR_STATE);
  });
});
