/** 接口数据类型定义，与 internal/api/docs/openapi.yaml 保持一致 */
declare namespace Api {
  /** 通用类型 */
  namespace Common {
    /** 分页参数 */
    interface PaginationParams {
      /** 当前页码 */
      current: number
      /** 每页条数 */
      size: number
      /** 总条数 */
      total: number
    }

    /** 分页响应基础结构 */
    interface PaginatedResponse<T = any> {
      records: T[]
      current: number
      size: number
      total: number
    }
  }

  /** 认证 */
  namespace Auth {
    /** 登录参数 */
    interface LoginParams {
      username: string
      password: string
    }

    /** 精简用户信息，登录与 me 接口共用 */
    interface SimpleUser {
      id: number
      name: string
      role: 'admin' | 'client'
    }

    /** 登录响应 */
    interface LoginData {
      token: string
      user: SimpleUser
    }

    /** 前端会话用户信息，roles 供框架菜单过滤 */
    interface UserInfo extends SimpleUser {
      roles: string[]
    }
  }

  /** 需求 */
  namespace Demand {
    type Status = import('@/utils/clepsydra/dict').DemandStatus

    /** 需求实体 */
    interface Item {
      id: number
      title: string
      description: string
      estimated_half_days: number
      estimate_confirmed_at: string | null
      estimate_confirmed_by: number | null
      planned_start_date: string | null
      actual_start_date: string | null
      actual_end_date: string | null
      actual_half_days: number | null
      status: Status
      accept_deadline: string | null
      accepted_at: string | null
      accepted_by: number | null
      accept_auto: boolean
      accept_locked: boolean
      created_at: string
      updated_at: string
    }

    /** 创建与更新共用请求体 */
    interface SaveParams {
      title: string
      description?: string
      estimated_half_days: number
      planned_start_date?: string
    }

    /** 标记开工请求体 */
    interface StartParams {
      actual_start_date: string
    }

    /** 标记完成请求体 */
    interface FinishParams {
      actual_start_date: string
      actual_end_date: string
      actual_half_days: number
    }
  }

  /** 账单 */
  namespace Bill {
    type Status = import('@/utils/clepsydra/dict').BillStatus

    /** 账单明细行 */
    interface Item {
      id: number
      demand_id: number
      demand_title: string
      demand_status: string
      half_days: number
      amount: number
      billable: boolean
      waived: boolean
      planned_start_date: string | null
      note: string
      created_at: string
    }

    /** 账单实体，items 仅详情接口返回 */
    interface Detail {
      id: number
      period: string
      status: Status
      daily_rate: number
      base_fee: number
      total_half_days: number
      total_amount: number
      shared_at: string | null
      confirm_deadline: string | null
      confirmed_at: string | null
      confirmed_by: number | null
      confirm_auto: boolean
      created_at: string
      updated_at: string
      items?: Item[]
    }
  }

  /** 用户 */
  namespace User {
    /** 用户实体 */
    interface Item {
      id: number
      username: string
      name: string
      role: 'admin' | 'client'
      enabled: boolean
      created_at: string
      updated_at: string
    }

    /** 创建请求体 */
    interface CreateParams {
      username: string
      password: string
      name: string
      role: 'admin' | 'client'
    }

    /** 更新请求体 */
    interface UpdateParams {
      name?: string
      enabled?: boolean
    }
  }

  /** 设置与节假日 */
  namespace Setting {
    /** 设置键值对，值一律为字符串 */
    type Values = Record<string, string>

    /** 节假日记录 */
    interface Holiday {
      id: number
      date: string
      type: 'holiday' | 'workday'
      name: string
    }

    /** 节假日保存条目 */
    interface HolidayEntry {
      date: string
      type: 'holiday' | 'workday'
      name?: string
    }
  }

  /** 审计日志 */
  namespace AuditLog {
    /** 日志实体 */
    interface Item {
      id: number
      actor_id: number
      actor_name: string
      action: string
      target_type: string
      target_id: number
      detail: Record<string, unknown>
      created_at: string
    }

    /** 分页查询参数 */
    interface Query {
      target_type?: string
      target_id?: number
      page?: number
      size?: number
    }

    /** 分页响应 */
    interface ListData {
      total: number
      rows: Item[]
    }
  }

  /** 工作台 */
  namespace Dashboard {
    /** 待办汇总 */
    interface Todos {
      pending_estimate_count: number
      pending_acceptance_count: number
      pending_bill_count: number
      billing_due_date: string
      billing_due_today: boolean
      prev_bill_shared: boolean
    }
  }
}
