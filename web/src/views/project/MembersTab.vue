<template>
  <a-card class="members-card">
    <template #title>
      项目成员
      <span class="tip">角色描述会作为上下文塞入发给该实例的每条指令</span>
    </template>
    <template #extra>
      <a-button type="primary" size="small" @click="showAdd = true">
        <template #icon><PlusOutlined /></template>
        拉入实例
      </a-button>
    </template>

    <a-empty v-if="!members.length" description="还没有成员，拉入 Hermes 实例开始协作" />
    <div v-else class="member-list">
      <div v-for="m in members" :key="m.id" class="member-row">
        <div class="avatar">{{ (m.instance?.name || '?').slice(0, 1) }}</div>
        <div class="member-info">
          <div class="member-name">
            {{ m.instance?.name }}
            <a-badge :status="m.online ? 'success' : 'default'" :text="m.online ? '在线' : '离线'" />
          </div>
          <div class="member-role">{{ m.role_description || '（未填写角色描述）' }}</div>
        </div>
        <a-button size="small" @click="openEdit(m)">编辑角色</a-button>
        <a-popconfirm title="将该实例移出项目？" @confirm="onRemove(m)">
          <a-button size="small" danger>移出</a-button>
        </a-popconfirm>
      </div>
    </div>

    <!-- 拉入 -->
    <a-modal v-model:open="showAdd" title="拉入 Hermes 实例" @ok="onAdd" :confirm-loading="saving">
      <a-form layout="vertical">
        <a-form-item label="选择实例" required>
          <a-select
            v-model:value="form.instance_id"
            placeholder="选择已注册的实例"
            :options="availableInstances.map(i => ({ value: i.id, label: i.name + (i.online ? '（在线）' : '（离线）') }))"
          />
        </a-form-item>
        <a-form-item label="在本项目中的角色">
          <a-textarea
            v-model:value="form.role_description"
            :rows="3"
            placeholder="如：后端开发，负责 API 设计与实现 / 如：代码审查与测试"
          />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 编辑角色 -->
    <a-modal v-model:open="showEdit" title="编辑角色描述" @ok="onSaveRole" :confirm-loading="saving">
      <a-textarea v-model:value="form.role_description" :rows="4" />
    </a-modal>
  </a-card>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { listInstances, addMember, updateMember, removeMember } from '../../api'

const props = defineProps({
  projectId: { type: Number, required: true },
  members: { type: Array, default: () => [] },
})
const emit = defineEmits(['refresh'])

const allInstances = ref([])
const showAdd = ref(false)
const showEdit = ref(false)
const saving = ref(false)
const current = ref(null)
const form = ref({ instance_id: undefined, role_description: '' })

const availableInstances = computed(() =>
  allInstances.value.filter((i) => !props.members.some((m) => m.instance_id === i.id))
)

onMounted(async () => {
  const { data } = await listInstances()
  allInstances.value = data.instances
})

const onAdd = async () => {
  if (!form.value.instance_id) {
    message.warning('请选择实例')
    return
  }
  saving.value = true
  try {
    await addMember(props.projectId, form.value)
    message.success('已加入项目')
    showAdd.value = false
    form.value = { instance_id: undefined, role_description: '' }
    emit('refresh')
  } catch {
  } finally {
    saving.value = false
  }
}

const openEdit = (m) => {
  current.value = m
  form.value = { instance_id: m.instance_id, role_description: m.role_description }
  showEdit.value = true
}

const onSaveRole = async () => {
  saving.value = true
  try {
    await updateMember(props.projectId, current.value.id, { role_description: form.value.role_description })
    message.success('已保存')
    showEdit.value = false
    emit('refresh')
  } catch {
  } finally {
    saving.value = false
  }
}

const onRemove = async (m) => {
  await removeMember(props.projectId, m.id)
  message.success('已移出')
  emit('refresh')
}
</script>

<style scoped>
.members-card { border-radius: 10px; }
.tip { font-size: 12px; color: #999; font-weight: normal; margin-left: 8px; }
.member-list { display: flex; flex-direction: column; gap: 10px; }
.member-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid #f0f0f0;
  border-radius: 10px;
  flex-wrap: wrap;
}
.avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, #5b6cff, #8e54e9);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  flex-shrink: 0;
}
.member-info { flex: 1; min-width: 200px; }
.member-name { font-weight: 600; display: flex; gap: 8px; align-items: center; }
.member-role { color: #888; font-size: 13px; margin-top: 2px; }
.member-row :deep(.ant-btn) { min-height: 32px; }
</style>
