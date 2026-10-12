<template>
  <a-card class="summary-card">
    <template #title>
      状态总结
      <span class="tip">由执行工作的 Hermes 按需更新，会作为上下文发给每个实例</span>
    </template>
    <template #extra>
      <span v-if="updatedAt" class="time">更新于 {{ fmtTime(updatedAt) }}</span>
      <a-button type="primary" size="small" :loading="saving" @click="onSave" style="margin-left:12px">保存</a-button>
    </template>
    <a-textarea
      v-model:value="content"
      :rows="14"
      placeholder="项目当前进展、关键结论、阻塞点…… Hermes 会通过工具 API 自动维护这里"
    />
  </a-card>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import dayjs from 'dayjs'
import { getSummary, putSummary } from '../../api'

const props = defineProps({ projectId: { type: Number, required: true } })

const content = ref('')
const updatedAt = ref('')
const saving = ref(false)
const editing = ref(false)
let timer = null

const fmtTime = (t) => dayjs(t).format('MM-DD HH:mm')

const load = async () => {
  const { data } = await getSummary(props.projectId)
  // 用户正在编辑时不覆盖输入
  if (!editing.value) {
    content.value = data.summary || ''
  }
  updatedAt.value = data.updated_at
}

const onSave = async () => {
  saving.value = true
  try {
    await putSummary(props.projectId, content.value)
    message.success('已保存')
    editing.value = false
    await load()
  } catch {
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await load()
  // 监听输入状态 + 轮询外部（agent）更新
  watchEditing()
  timer = setInterval(load, 8000)
})
onUnmounted(() => clearInterval(timer))

// 简单方案：输入框 focus/input 期间暂停覆盖
const watchEditing = () => {
  const el = document.querySelector('.summary-card textarea')
  if (!el) return
  el.addEventListener('focus', () => (editing.value = true))
  el.addEventListener('input', () => (editing.value = true))
  el.addEventListener('blur', () => (editing.value = false))
}
</script>

<style scoped>
.summary-card { border-radius: 10px; }
.tip { font-size: 12px; color: #999; font-weight: normal; margin-left: 8px; }
.time { color: #bbb; font-size: 12px; }
</style>
