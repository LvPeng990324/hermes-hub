<template>
  <div>
    <div class="page-head">
      <div>
        <h2>项目</h2>
        <p class="desc">创建项目、拉入 Hermes 实例，像群聊一样 @ 它们协作干活</p>
      </div>
      <a-button type="primary" @click="openCreate">
        <template #icon><PlusOutlined /></template>
        创建项目
      </a-button>
    </div>

    <a-spin :spinning="loading">
      <a-empty v-if="!projects.length" description="还没有项目，创建一个开始协作" />
      <a-row v-else :gutter="[16, 16]">
        <a-col v-for="p in projects" :key="p.id" :xs="24" :sm="12" :lg="8" :xl="6">
          <a-card class="proj-card" hoverable @click="goDetail(p)">
            <template #title>
              <span class="proj-name">{{ p.name }}</span>
            </template>
            <p class="proj-goal">{{ p.goal || p.description || '（暂无目标描述）' }}</p>
            <div class="proj-meta">
              <span><RobotOutlined /> {{ p.member_count }} 个实例</span>
              <span><MessageOutlined /> {{ p.message_count }} 条消息</span>
            </div>
            <div class="proj-actions" @click.stop>
              <a-button size="small" @click="openEdit(p)">编辑</a-button>
              <a-popconfirm title="删除项目会清空消息、看板与成员关系，确认？" @confirm="onDelete(p)">
                <a-button size="small" danger>删除</a-button>
              </a-popconfirm>
            </div>
          </a-card>
        </a-col>
      </a-row>
    </a-spin>

    <a-modal
      v-model:open="showForm"
      :title="form.id ? '编辑项目' : '创建项目'"
      @ok="onSubmit"
      :confirm-loading="saving"
      width="560px"
    >
      <a-form layout="vertical">
        <a-form-item label="项目名称" required>
          <a-input v-model:value="form.name" placeholder="如：官网重构" />
        </a-form-item>
        <a-form-item label="项目描述">
          <a-textarea v-model:value="form.description" :rows="2" placeholder="这个项目是做什么的" />
        </a-form-item>
        <a-form-item label="项目目标">
          <a-textarea v-model:value="form.goal" :rows="3" placeholder="要达成什么结果，会作为上下文发给每个 Hermes" />
        </a-form-item>
        <a-form-item label="要求">
          <a-textarea v-model:value="form.requirements" :rows="3" placeholder="约束、规范、验收标准等" />
        </a-form-item>
      </a-form>
    </a-modal>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { PlusOutlined, RobotOutlined, MessageOutlined } from '@ant-design/icons-vue'
import { listProjects, createProject, updateProject, deleteProject } from '../api'

const router = useRouter()
const projects = ref([])
const loading = ref(false)
const saving = ref(false)
const showForm = ref(false)
const form = ref({ id: null, name: '', description: '', goal: '', requirements: '' })

const load = async () => {
  loading.value = true
  try {
    const { data } = await listProjects()
    projects.value = data.projects
  } finally {
    loading.value = false
  }
}
onMounted(load)

const openCreate = () => {
  form.value = { id: null, name: '', description: '', goal: '', requirements: '' }
  showForm.value = true
}
const openEdit = (p) => {
  form.value = { id: p.id, name: p.name, description: p.description, goal: p.goal, requirements: p.requirements }
  showForm.value = true
}

const onSubmit = async () => {
  if (!form.value.name.trim()) {
    message.warning('请填写项目名称')
    return
  }
  saving.value = true
  try {
    if (form.value.id) {
      await updateProject(form.value.id, form.value)
    } else {
      await createProject(form.value)
    }
    message.success('已保存')
    showForm.value = false
    await load()
  } catch {
  } finally {
    saving.value = false
  }
}

const onDelete = async (p) => {
  await deleteProject(p.id)
  message.success('已删除')
  await load()
}

const goDetail = (p) => router.push(`/projects/${p.id}`)
</script>

<style scoped>
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}
.page-head h2 { margin: 0; }
.desc { color: #888; margin: 4px 0 0; }
.proj-card { border-radius: 10px; }
.proj-name { font-size: 15px; }
.proj-goal {
  color: #666;
  min-height: 44px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.proj-meta {
  display: flex;
  gap: 16px;
  color: #999;
  font-size: 13px;
  margin-bottom: 12px;
}
.proj-actions { display: flex; gap: 8px; }
</style>
