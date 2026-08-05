<script lang="ts" setup>
import { ref } from 'vue';

import { useVbenModal } from '@vben/common-ui';

import { message, TextArea } from 'antdv-next';

import { saveHolidays } from '#/api/setting';
import { showSuccess } from '#/utils/http/error';

/**
 * 批量导入节假日弹窗
 *
 * 粘贴 holiday-cn（github.com/NateScarlet/holiday-cn）年度 JSON 文件内容，按日期覆盖更新
 */
defineOptions({ name: 'HolidayImportDialog' });

const emit = defineEmits<{
  /** 导入成功 */
  success: [];
}>();

/** holiday-cn 年度文件里单条记录的结构，只取用得到的三个字段 */
interface HolidayCnDay {
  date: string;
  isOffDay: boolean;
  name: string;
}

const importText = ref('');

const [Modal, modalApi] = useVbenModal({
  onConfirm: submit,
  onOpenChange(isOpen) {
    if (!isOpen) {
      return;
    }

    importText.value = '';
  },
});

/** 解析 holiday-cn 年度 JSON 并批量导入，isOffDay 决定映射为休息日还是调休补班 */
async function submit() {
  let entries: Api.Setting.HolidayEntry[];
  try {
    const parsed = JSON.parse(importText.value) as { days?: HolidayCnDay[] };
    if (!Array.isArray(parsed.days) || parsed.days.length === 0) {
      throw new Error('缺少 days 字段');
    }
    entries = parsed.days.map((d) => ({
      date: d.date,
      type: d.isOffDay ? 'holiday' : 'workday',
      name: d.name,
    }));
  } catch {
    message.error('JSON 解析失败，请粘贴完整的 holiday-cn 年度文件内容');
    return;
  }

  modalApi.lock();
  try {
    await saveHolidays(entries);
    showSuccess(`已导入 ${entries.length} 条`);
    emit('success');
    modalApi.close();
  } catch {
    // 错误提示已由请求拦截器统一弹出
  } finally {
    modalApi.unlock();
  }
}
</script>

<template>
  <Modal class="w-[560px]" title="导入 holiday-cn 年度数据">
    <p class="mb-2 text-sm text-gray-400">
      粘贴 holiday-cn（github.com/NateScarlet/holiday-cn）年度 JSON
      文件内容，按日期覆盖更新
    </p>
    <TextArea
      v-model:value="importText"
      :rows="10"
      placeholder="请粘贴 holiday-cn 年度 JSON 文件内容"
    />
  </Modal>
</template>
