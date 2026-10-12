<template>
  <div class="board">
    <div v-for="col in columns" :key="col.key" class="board-col">
      <div class="col-head" :style="{ borderColor: col.color }">
        <span class="col-title">{{ col.title }}</span>
        <span class="col-count">{{ cardsOf(col.key).length }}</span>
        <a-button type="text" size="small" @click="openCreate(col.key)">
          <template #icon><PlusOutlined /></template>
        </a-button>
      </div>
      <div class="col-body">
        <a-card v-for="card in cardsOf(col.key)" :key="card.id" size="small" class="task-card">
          <div class="card-title">{{ card.title }}</div>
          <div v-if="card.description" class="card-desc">{{ card.description }}</div>
          <div class="card-foot">
            <a-tag v-if="card.assignee_name" color="blue">{{ card.assignee_name }}</a-tag>
            <span v-else class="unassigned">未指派</span>
            <a-dropdown trigger="click">
              <a-button type="text" size="small"><MoreOutlined /></a-button>
              <template #overlay>
                <a-menu @click="(e) => onCardAction(e.key, card)">
                  <a-menu-item v-for="c in columns.filter(x => x.key !== card.status)" :key="'move:' + c.key">
                    移到「{{ c.title }}」
                  </a-menu-item>
                  <a-menu-divider />
                  <a-menu-item key="edit">编辑</a-menu-item>
                  <a-menu-item key="delete" style="color:#ff4d4f">删除</a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
          </div>
        </a-card>
        <div v-if="!cardsOf(col.key).length" class="col-empty">暂无任务</div>
      </div>
    </div>

    <a-modal
      v-model:open="showForm"
      :title="form.id ? '编辑任务' : '新建任务'"
      @ok="onSubmit"
      :confirm-loading="saving"
    >
      <a-form layout="vertical">
        <a-form-item label="任务标题" required>
          <a-input v-model:value="form.title" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="form.description" :rows="3" />
        </a-form-item>
        <a-form-item label="状态">
          <a-select v-model:value="form.status" :options="columns.map(c => ({ value: c.key, label: c.title }))" />
        </a-form-item>
        <a-form-item label="负责人（Hermes 实例）">
          <a-select
            v-model:value="form.assignee"
            allow-clear
            placeholder="选择实例"
            :options="members.map(m => ({ value: m.instance?.name, label: m.instance?.name }))"
          />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { PlusOutlined, MoreOutlined } from '@ant-design/icons-vue'
import { listCards, createCard, updateCard, deleteCard } from '../../api'

const props = defineProps({
  projectId: { type: Number, required: true },
  members: { type: Array, default: () => [] },
})

const columns = [
  { key: 'todo', title: '待办', color: '#8c8c8c' },
  { key: 'in_progress', title: '进行中', color: '#1677ff' },
  { key: 'review', title: '待确认', color: '#fa8c16' },
  { key: 'done', title: '完成', color: '#52c41a' },
]

const cards = ref([])
const showForm = ref(false)
const saving = ref(false)
const form = ref({ id: null, title: '', description: '', status: 'todo', assignee: undefined })
let timer = null

const cardsOf = (status) => cards.value.filter((c) => c.status === status)

const load = async () => {
  const { data } = await listCards(props.projectId)
  cards.value = data.cards
}
onMounted(async () => {
  await load()
  timer = setInterval(load, 5000) // agent 会改看板，轮询同步
})
onUnmounted(() => clearInterval(timer))

const openCreate = (status) => {
  form.value = { id: null, title: '', description: '', status, assignee: undefined }
  showForm.value = true
}

const onCardAction = (key, card) => {
  if (key.startsWith('move:')) {
    updateCard(props.projectId, card.id, { status: key.slice(5) }).then(load)
  } else if (key === 'edit') {
    form.value = {
      id: card.id,
      title: card.title,
      description: card.description,
      status: card.status,
      assignee: card.assignee_name || undefined,
    }
    showForm.value = true
  } else if (key === 'delete') {
    Modal.confirm({
      title: '删除该任务？',
      onOk: () => deleteCard(props.projectId, card.id).then(() => { message.success('已删除'); load() }),
    })
  }
}

const onSubmit = async () => {
  if (!form.value.title.trim()) {
    message.warning('请填写任务标题')
    return
  }
  saving.value = true
  try {
    if (form.value.id) {
      await updateCard(props.projectId, form.value.id, form.value)
    } else {
      await createCard(props.projectId, form.value)
    }
    showForm.value = false
    await load()
  } catch {
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.board {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 8px;
  align-items: flex-start;
}
.board-col {
  flex: 1 0 240px;
  min-width: 240px;
  background: #fff;
  border-radius: 10px;
  padding: 10px;
}
.col-head {
  display: flex;
  align-items: center;
  gap: 8px;
  border-top: 3px solid;
  padding-top: 8px;
  margin-bottom: 10px;
}
.col-title { font-weight: 600; }
.col-count {
  background: #f0f1f5;
  border-radius: 8px;
  font-size: 12px;
  padding: 0 8px;
  color: #888;
  flex: 1;
}
.task-card {
  margin-bottom: 8px;
  border-radius: 8px;
}
.card-title { font-weight: 600; margin-bottom: 4px; word-break: break-all; }
.card-desc { color: #888; font-size: 12px; margin-bottom: 6px; white-space: pre-wrap; }
.card-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.unassigned { color: #ccc; font-size: 12px; }
.col-empty {
  text-align: center;
  color: #ccc;
  font-size: 12px;
  padding: 16px 0;
}
</style>
