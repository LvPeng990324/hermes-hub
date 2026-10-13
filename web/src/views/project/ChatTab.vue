<template>
  <div class="chat-wrap">
    <!-- 执行中指示 -->
    <div v-if="running.length || justEnded.length" class="running-bar">
      <a-tag v-if="running.length" color="processing">
        <LoadingOutlined /> {{ runningNames }} 执行中<template v-if="runningElapsed">（已执行 {{ runningElapsed }}）</template>
      </a-tag>
      <template v-else>
        <a-tag v-for="e in justEnded" :key="e.id" color="error">
          <CloseCircleOutlined /> {{ e.name }} {{ e.kind }}（耗时 {{ e.duration }}）
        </a-tag>
      </template>
    </div>

    <!-- 消息流 -->
    <div ref="msgBox" class="msg-box" @scroll="onScroll">
      <a-spin :spinning="initialLoading" style="display:block; padding: 40px 0" />
      <template v-for="m in messages" :key="m.id">
        <!-- 系统消息 -->
        <div v-if="m.sender_type === 'system'" class="msg-system">
          <span>{{ m.content }}</span>
        </div>
        <!-- 用户 / agent 气泡 -->
        <div v-else class="msg-row" :class="{ right: m.sender_type === 'user' }">
          <div class="avatar" :class="m.sender_type">
            {{ m.sender_type === 'user' ? '我' : (m.sender_name || '?').slice(0, 1) }}
          </div>
          <div class="bubble-wrap">
            <div class="msg-meta">
              <span class="sender">{{ m.sender_name }}</span>
              <span class="time">{{ fmtTime(m.created_at) }}</span>
              <a-tag v-if="m.msg_type === 'question'" color="warning" style="margin-left:4px">询问</a-tag>
            </div>
            <div class="bubble" :class="[m.sender_type, { question: m.msg_type === 'question' }]">
              {{ m.content }}
              <div v-if="m.msg_type === 'question' && m.sender_type === 'agent' && !m.replied" class="reply-area">
                <a-button
                  v-for="(opt, i) in msgOptions(m)"
                  :key="i"
                  size="small"
                  :disabled="replying[m.id]"
                  @click="onReplyOption(m, opt)"
                >{{ opt }}</a-button>
                <div class="reply-input">
                  <a-input
                    v-model:value="replyInputs[m.id]"
                    size="small"
                    placeholder="回复…"
                    :disabled="replying[m.id]"
                    @pressEnter="onReplyInput(m)"
                  />
                  <a-button
                    size="small"
                    type="primary"
                    :disabled="replying[m.id] || !(replyInputs[m.id] || '').trim()"
                    @click="onReplyInput(m)"
                  >发送</a-button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </template>
      <div v-if="!initialLoading && !messages.length" class="empty-hint">
        还没有消息。在下方输入指令，用 @ 指定要干活的 Hermes
      </div>
    </div>

    <!-- 输入区 -->
    <div class="input-area safe-bottom">
      <a-mentions
        v-model:value="input"
        :options="mentionOptions"
        :rows="3"
        placeholder="输入指令，@ 指定 Hermes（输入 @ 唤起成员列表）；@用户 无意义哦，你就是用户"
        @keydown="onKeydown"
      />
      <div class="input-actions">
        <span class="hint">{{ mentionHint }}</span>
        <a-button type="primary" :loading="sending" :disabled="!input.trim()" @click="onSend">
          <template #icon><SendOutlined /></template>
          发送
        </a-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { CloseCircleOutlined, LoadingOutlined, SendOutlined } from '@ant-design/icons-vue'
import { listMessages, sendMessage, listDispatches } from '../../api'
import dayjs from 'dayjs'

const props = defineProps({
  projectId: { type: Number, required: true },
  members: { type: Array, default: () => [] },
})

const messages = ref([])
const running = ref([])
const justEnded = ref([])
const runningStart = ref(0)
const nowTick = ref(Date.now())
const replyInputs = reactive({})
const replying = reactive({})
const input = ref('')
const sending = ref(false)
const initialLoading = ref(true)
const msgBox = ref(null)
const stickBottom = ref(true)
let timer = null
let tickTimer = null
let prevRunning = []
const endedTimers = new Map()

const mentionOptions = computed(() =>
  props.members.map((m) => ({
    value: m.instance?.name,
    label: `${m.instance?.name}${m.role_description ? '（' + m.role_description.slice(0, 12) + '）' : ''}`,
  }))
)
const mentionHint = computed(() =>
  props.members.length ? '' : '项目还没有成员，先到「成员」页拉入 Hermes 实例'
)

const fmtTime = (t) => dayjs(t).format('MM-DD HH:mm')

const fmtDuration = (ms) => {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return `${s}秒`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}分${s % 60}秒`
  const h = Math.floor(m / 60)
  return `${h}时${m % 60}分${s % 60}秒`
}

const runningNames = computed(() => running.value.map((d) => d.instance_name).join('、'))
const runningElapsed = computed(() => {
  if (!running.value.length || !runningStart.value) return ''
  return fmtDuration(nowTick.value - runningStart.value)
})

const msgOptions = (m) => {
  if (!m.options) return []
  try {
    const arr = JSON.parse(m.options)
    return Array.isArray(arr) ? arr.filter((x) => typeof x === 'string' && x) : []
  } catch {
    return []
  }
}

const addJustEnded = (d, status) => {
  justEnded.value = justEnded.value.filter((e) => e.id !== d.id)
  justEnded.value.push({
    id: d.id,
    name: d.instance_name,
    kind: status === 'timeout' ? '执行超时' : '执行失败',
    duration: fmtDuration(Date.now() - dayjs(d.created_at).valueOf()),
  })
  if (endedTimers.has(d.id)) clearTimeout(endedTimers.get(d.id))
  endedTimers.set(d.id, setTimeout(() => {
    justEnded.value = justEnded.value.filter((e) => e.id !== d.id)
    endedTimers.delete(d.id)
  }, 4000))
}

const loadInitial = async () => {
  initialLoading.value = true
  try {
    const { data } = await listMessages(props.projectId, { limit: 50 })
    messages.value = data.messages
    await nextTick()
    scrollBottom()
  } finally {
    initialLoading.value = false
  }
}

const poll = async () => {
  try {
    const lastId = messages.value.length ? messages.value[messages.value.length - 1].id : 0
    const [{ data: msgData }, { data: dspData }] = await Promise.all([
      listMessages(props.projectId, { after_id: lastId, limit: 100 }),
      listDispatches(props.projectId, { limit: 20 }),
    ])
    if (msgData.messages.length) {
      messages.value.push(...msgData.messages)
      if (stickBottom.value) {
        await nextTick()
        scrollBottom()
      }
    }
    running.value = dspData.dispatches.filter((d) => d.status === 'pending' || d.status === 'running')
    if (running.value.length) {
      runningStart.value = Math.min(...running.value.map((d) => dayjs(d.created_at).valueOf()))
    } else {
      runningStart.value = 0
    }
    // 终态过渡：上一轮在跑、本轮不在 running 里且状态为 failed/timeout 的派发
    const curIds = new Set(running.value.map((d) => d.id))
    for (const d of prevRunning) {
      if (curIds.has(d.id)) continue
      const full = dspData.dispatches.find((x) => x.id === d.id)
      if (full && (full.status === 'failed' || full.status === 'timeout')) {
        addJustEnded(d, full.status)
      }
    }
    prevRunning = running.value
  } catch {
    // 轮询静默失败，下轮重试
  }
}

const scrollBottom = () => {
  const el = msgBox.value
  if (el) el.scrollTop = el.scrollHeight
}
const onScroll = () => {
  const el = msgBox.value
  if (!el) return
  stickBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 60
}

const onKeydown = (e) => {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    onSend()
  }
}

const onSend = async () => {
  const content = input.value.trim()
  if (!content) return
  sending.value = true
  try {
    const { data } = await sendMessage(props.projectId, content)
    messages.value.push(data.message)
    input.value = ''
    stickBottom.value = true
    await nextTick()
    scrollBottom()
    setTimeout(poll, 300)
  } catch {
  } finally {
    sending.value = false
  }
}

const onReply = async (m, content) => {
  const text = (content || '').trim()
  if (!text || replying[m.id]) return
  replying[m.id] = true
  try {
    const { data } = await sendMessage(props.projectId, text, m.id)
    m.replied = true
    if (data?.message) {
      messages.value.push(data.message)
      stickBottom.value = true
      await nextTick()
      scrollBottom()
    }
    setTimeout(poll, 300)
  } catch {
    // 失败提示由 axios 拦截器统一弹出，按钮/输入框在 finally 中恢复
  } finally {
    replying[m.id] = false
  }
}

const onReplyOption = (m, opt) => onReply(m, opt)

const onReplyInput = async (m) => {
  const text = (replyInputs[m.id] || '').trim()
  if (!text) return
  await onReply(m, text)
  if (m.replied) replyInputs[m.id] = ''
}

onMounted(async () => {
  await loadInitial()
  await poll()
  timer = setInterval(poll, 3000)
  tickTimer = setInterval(() => { nowTick.value = Date.now() }, 1000)
})
onUnmounted(() => {
  clearInterval(timer)
  clearInterval(tickTimer)
  endedTimers.forEach((t) => clearTimeout(t))
  endedTimers.clear()
})

watch(() => props.projectId, async () => {
  messages.value = []
  running.value = []
  justEnded.value = []
  prevRunning = []
  await loadInitial()
})
</script>

<style scoped>
.chat-wrap {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 220px);
  min-height: 420px;
}
.running-bar {
  padding: 6px 0;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.msg-box {
  flex: 1;
  overflow-y: auto;
  background: #fff;
  border-radius: 10px;
  padding: 16px;
}
.msg-system {
  text-align: center;
  margin: 10px 0;
}
.msg-system span {
  background: #f0f1f5;
  color: #999;
  font-size: 12px;
  padding: 3px 12px;
  border-radius: 10px;
}
.msg-row {
  display: flex;
  gap: 10px;
  margin-bottom: 16px;
}
.msg-row.right {
  flex-direction: row-reverse;
}
.avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  color: #fff;
  flex-shrink: 0;
  background: linear-gradient(135deg, #5b6cff, #8e54e9);
}
.avatar.user { background: linear-gradient(135deg, #36cfc9, #4096ff); }
.bubble-wrap {
  max-width: min(72%, 720px);
}
.msg-row.right .bubble-wrap {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
}
.msg-meta {
  font-size: 12px;
  color: #999;
  margin-bottom: 4px;
}
.msg-meta .time { margin-left: 8px; }
.bubble {
  padding: 10px 14px;
  border-radius: 10px;
  background: #f4f5f9;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
  display: inline-block;
  text-align: left;
}
.bubble.user {
  background: #5b6cff;
  color: #fff;
}
.bubble.question {
  background: #fff7e6;
  border: 1px solid #ffd591;
}
.reply-area {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed #ffd591;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.reply-input {
  display: flex;
  gap: 6px;
  flex: 1 1 180px;
  min-width: 160px;
}
.empty-hint {
  text-align: center;
  color: #bbb;
  padding: 48px 0;
}
.input-area {
  margin-top: 10px;
  background: #fff;
  border-radius: 10px;
  padding: 10px;
}
.input-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 8px;
}
.input-actions .hint { color: #bbb; font-size: 12px; }
@media (max-width: 991px) {
  .chat-wrap { height: calc(100vh - 260px); }
  .bubble-wrap { max-width: 82%; }
}
</style>
