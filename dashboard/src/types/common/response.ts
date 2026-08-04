/**
 * API 响应类型定义模块
 *
 * 提供统一的 API 响应结构类型定义
 *
 * ## 主要功能
 *
 * - 基础响应结构定义
 * - 泛型支持（适配不同数据类型）
 * - 统一的响应格式约束
 *
 * ## 使用场景
 *
 * - API 请求响应类型约束
 * - 接口数据类型定义
 * - 响应数据解析
 *
 * @module types/common/response
 * @author Art Design Pro Team
 */

/** 后端统一响应包装 */
export interface BaseResponse<T = unknown> {
  /** 业务状态码，0 表示成功 */
  code: number
  /** 提示信息 */
  message: string
  /** 业务数据 */
  data: T
}
