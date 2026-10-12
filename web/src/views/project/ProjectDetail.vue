<template>
  <div>
    <div class="proj-head">
      <a-button type="text" @click="$router.push('/projects')">
        <template #icon><ArrowLeftOutlined /></template>
      </a-button>
      <div class="proj-title">
        <h2>{{ project?.name || '...' }}</h2>
        <span v-if="project?.goal" class="proj-goal">{{ project.goal }}</span>
      </div>
    </div>

    <a-tabs v-model:activeKey="tab" class="proj-tabs">
      <a-tab-pane key="chat">
        <template #tab><MessageOutlined /> 会话</template>
        <ChatTab v-if="loaded" :project-id="projectId" :members="members" @refresh-members="loadMembers" />
      </a-tab-pane>
      <a-tab-pane key="board">
        <template #tab><ScheduleOutlined /> 看板</template>
        <BoardTab v-if="loaded" :project-id="projectId" :members="members" />
      </a-tab-pane>
      <a-tab-pane key="summary">
        <template #tab><FileTextOutlined /> 状态总结</template>
        <SummaryTab v-if="loaded" :project-id="projectId" />
      </a-tab-pane>
      <a-tab-pane key="files">
        <template #tab><FolderOpenOutlined /> 共享文件</template>
        <FilesTab v-if="loaded" :project-id="projectId" />
      </a-tab-pane>
      <a-tab-pane key="members">
        <template #tab><TeamOutlined /> 成员</template>
        <MembersTab v-if="loaded" :project-id="projectId" :members="members" @refresh="loadMembers" />
      </a-tab-pane>
    </a-tabs>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  ArrowLeftOutlined, MessageOutlined, ScheduleOutlined,
  FileTextOutlined, FolderOpenOutlined, TeamOutlined,
} from '@ant-design/icons-vue'
import { getProject, listMembers } from '../../api'
import ChatTab from './ChatTab.vue'
import BoardTab from './BoardTab.vue'
import SummaryTab from './SummaryTab.vue'
import FilesTab from './FilesTab.vue'
import MembersTab from './MembersTab.vue'

const route = useRoute()
const projectId = computed(() => Number(route.params.id))
const project = ref(null)
const members = ref([])
const loaded = ref(false)
const tab = ref('chat')

const loadProject = async () => {
  const { data } = await getProject(projectId.value)
  project.value = data.project
}
const loadMembers = async () => {
  const { data } = await listMembers(projectId.value)
  members.value = data.members
}

onMounted(async () => {
  await Promise.all([loadProject(), loadMembers()])
  loaded.value = true
})
</script>

<style scoped>
.proj-head {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  margin-bottom: 4px;
}
.proj-title h2 { margin: 0; }
.proj-goal {
  color: #888;
  font-size: 13px;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 60vw;
}
.proj-tabs :deep(.ant-tabs-nav) {
  margin-bottom: 12px;
}
@media (max-width: 991px) {
  .proj-goal { max-width: 70vw; }
}
</style>
