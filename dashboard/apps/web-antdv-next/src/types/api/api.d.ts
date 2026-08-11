/**
 * 接口数据类型定义，与 internal/api/docs/openapi.yaml 保持一致
 *
 * Status 类型引用 utils/clepsydra/dict.ts（状态字典，含 label/type/actions），
 * 与旧前端 dashboard/src/types/api/api.d.ts 的写法保持一致
 */
declare namespace Api {
  /** 通用类型 */
  namespace Common {
    /** 分页参数 */
    interface PaginationParams {
      /** 当前页码 */
      current: number;
      /** 每页条数 */
      size: number;
      /** 总条数 */
      total: number;
    }

    /** 分页响应基础结构 */
    interface PaginatedResponse<T = any> {
      records: T[];
      current: number;
      size: number;
      total: number;
    }
  }

  /** 认证 */
  namespace Auth {
    /** 登录参数 */
    interface LoginParams {
      username: string;
      password: string;
    }

    /** 精简用户信息，登录与 me 接口共用 */
    interface SimpleUser {
      id: number;
      name: string;
      role: 'admin' | 'client';
    }

    /** 登录响应 */
    interface LoginData {
      token: string;
      user: SimpleUser;
    }

    /** 前端会话用户信息，roles 供框架菜单过滤 */
    interface UserInfo extends SimpleUser {
      roles: string[];
    }
  }

  /** 项目（轻量标签） */
  namespace Project {
    /** 项目实体，demand_count 仅列表接口有效 */
    interface Item {
      id: number;
      name: string;
      color: string;
      remark: string;
      demand_count: number;
      created_at: string;
      updated_at: string;
    }

    /** 需求 / 账单明细上携带的精简引用；需求走 ent 序列化，可选字段带 omitempty */
    interface Ref {
      id: number;
      name: string;
      color?: string;
    }

    /** 创建 / 更新请求体 */
    interface SaveParams {
      name: string;
      color?: string;
      remark?: string;
    }
  }

  /** 性质标签 */
  namespace Tag {
    /** 标签实体，demand_count 仅列表接口有效；颜色由后端按名称生成并固化 */
    interface Item {
      id: number;
      name: string;
      color: string;
      demand_count: number;
      created_at: string;
      updated_at: string;
    }

    /** 需求上携带的精简引用；需求走 ent 序列化，可选字段带 omitempty */
    interface Ref {
      id: number;
      name: string;
      color?: string;
    }

    /** 创建 / 更新请求体，仅名称，颜色由后端生成，不可指定 */
    interface SaveParams {
      name: string;
    }
  }

  /** 需求 */
  namespace Demand {
    type Status = import('#/utils/clepsydra/dict').DemandStatus;
    type Priority = import('#/utils/clepsydra/dict').DemandPriority;

    /** 需求实体 */
    interface Item {
      id: number;
      title: string;
      description: string;
      estimated_half_days: number;
      estimate_confirmed_at: null | string;
      estimate_confirmed_by: null | number;
      planned_start_date: null | string;
      actual_start_date: null | string;
      actual_end_date: null | string;
      actual_half_days: null | number;
      status: Status;
      /** 优先级，排期参考元数据，默认 normal */
      priority: Priority;
      accept_deadline: null | string;
      accepted_at: null | string;
      accepted_by: null | number;
      accept_auto: boolean;
      accept_locked: boolean;
      created_at: string;
      updated_at: string;
      /** ent 关联预加载结果：项目与性质标签 */
      edges?: {
        projects?: Api.Project.Ref[];
        tags?: Api.Tag.Ref[];
      };
    }

    /**
     * 更新需求请求体，仅标题与描述
     * 预估人天与预计开工日期由提交人天确认（submit-estimate）时填写，不在此处
     */
    interface SaveParams {
      title: string;
      description?: string;
    }

    /**
     * 创建需求请求体；预估三字段是超级管理员专属的可选快捷路径：
     * 填了人天创建即进入 pending_estimate，再带 confirmed 直达 confirmed（确认人记为创建者）
     */
    interface CreateParams extends SaveParams {
      estimated_half_days?: number;
      planned_start_date?: string;
      confirmed?: boolean;
      /** 关联的项目 ID 列表，可选 */
      project_ids?: number[];
      /** 关联的性质标签 ID 列表，可选 */
      tag_ids?: number[];
      /** 优先级，可选，缺省 normal */
      priority?: Priority;
    }

    /** 提交预估人天请求体，pending_estimate 状态下可重复提交修正预估 */
    interface EstimateParams {
      estimated_half_days: number;
      planned_start_date?: string;
    }

    /** 标记开工请求体 */
    interface StartParams {
      actual_start_date: string;
    }

    /** 标记完成请求体 */
    interface FinishParams {
      actual_start_date: string;
      actual_end_date: string;
      actual_half_days: number;
    }
  }

  /** 账单 */
  namespace Bill {
    type Status = import('#/utils/clepsydra/dict').BillStatus;

    /** 账单明细行 */
    interface Item {
      id: number;
      demand_id: number;
      demand_title: string;
      demand_status: string;
      half_days: number;
      amount: number;
      billable: boolean;
      waived: boolean;
      planned_start_date: null | string;
      note: string;
      created_at: string;
      /** 所属需求的项目标签，实时关联（非快照） */
      projects: Api.Project.Ref[];
    }

    /** 账单实体，items 仅详情接口返回 */
    interface Detail {
      id: number;
      name: string;
      period: null | string;
      status: Status;
      daily_rate: number;
      base_fee: number;
      total_half_days: number;
      total_amount: number;
      total_override: boolean;
      confirm_deadline: null | string;
      confirmed_at: null | string;
      confirmed_by: null | number;
      confirm_auto: boolean;
      paid_at: null | string;
      paid_by: null | number;
      created_at: string;
      updated_at: string;
      items?: Item[];
    }

    /** 可加入账单的需求，按加入后的行类型分组 */
    interface SelectableDemands {
      billable: Api.Demand.Item[];
      display: Api.Demand.Item[];
    }

    /** 编辑账单请求体，缺省字段不修改（仅超级管理员） */
    interface UpdateParams {
      name?: string;
      daily_rate?: number;
      base_fee?: number;
      confirm_deadline?: string;
      total_amount?: number;
      reset_total?: boolean;
    }

    /** 编辑账单明细请求体，缺省字段不修改（仅超级管理员） */
    interface UpdateItemParams {
      half_days?: number;
      amount?: number;
      note?: string;
    }
  }

  /** 用户 */
  namespace User {
    /** 用户实体 */
    interface Item {
      id: number;
      username: string;
      name: string;
      role: 'admin' | 'client';
      enabled: boolean;
      created_at: string;
      updated_at: string;
    }

    /** 创建请求体 */
    interface CreateParams {
      username: string;
      password: string;
      name: string;
      role: 'admin' | 'client';
    }

    /** 更新请求体 */
    interface UpdateParams {
      name?: string;
      enabled?: boolean;
    }
  }

  /** 设置与节假日 */
  namespace Setting {
    /** 设置键值对，值一律为字符串 */
    type Values = Record<string, string>;

    /** 节假日记录 */
    interface Holiday {
      id: number;
      date: string;
      type: 'holiday' | 'workday';
      name: string;
    }

    /** 节假日保存条目 */
    interface HolidayEntry {
      date: string;
      type: 'holiday' | 'workday';
      name?: string;
    }
  }

  /** 审计日志 */
  namespace AuditLog {
    /** 日志实体 */
    interface Item {
      id: number;
      actor_id: number;
      actor_name: string;
      action: string;
      target_type: string;
      target_id: number;
      detail: Record<string, unknown>;
      created_at: string;
    }

    /** 分页查询参数 */
    interface Query {
      target_type?: string;
      /** 操作类型筛选，如 demand.create、bill.confirm */
      action?: string;
      target_id?: number;
      page?: number;
      size?: number;
    }

    /** 分页响应 */
    interface ListData {
      total: number;
      rows: Item[];
    }
  }

  /** 工作台 */
  namespace Dashboard {
    /** 待办汇总 */
    interface Todos {
      pending_estimate_count: number;
      pending_acceptance_count: number;
      pending_bill_count: number;
      billing_due_date: string;
      billing_due_today: boolean;
      prev_bill_generated: boolean;
    }
  }
}
