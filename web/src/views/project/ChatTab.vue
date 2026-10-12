<template>
  <div class="chat-wrap">
    <!-- 执行中指示 -->
    <div v-if="running.length" class="running-bar">
      <a-tag v-for="d in running" :key="d.id" color="processing">
        <LoadingOutlined /> {{ d.instance_name }} 执行中
      </a-tag>
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
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { LoadingOutlined, SendOutlined } from '@ant-design/icons-vue'
import { listMessages, sendMessage, listDispatches } from '../../api'
import dayjs from 'dayjs'

const props = defineProps({
  projectId: { type: Number, required: true },
  members: { type: Array, default: () => [] },
})

const messages = ref([])
const running = ref([])
const input = ref('')
const sending = ref(false)
const initialLoading = ref(true)
const msgBox = ref(null)
const stickBottom = ref(true)
let timer = null

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

onMounted(async () => {
  await loadInitial()
  await poll()
  timer = setInterval(poll, 3000)
})
onUnmounted(() => clearInterval(timer))

watch(() => props.projectId, async () => {
  messages.value = []
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
