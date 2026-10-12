<template>
  <a-card class="files-card">
    <template #title>
      共享文件
      <span class="tip">每个项目独立目录，接入的 Hermes 可自由读写</span>
    </template>
    <template #extra>
      <a-button size="small" @click="load"><template #icon><ReloadOutlined /></template></a-button>
    </template>

    <a-upload-dragger :show-upload-list="false" :custom-request="onUpload" multiple style="margin-bottom: 16px">
      <p class="ant-upload-drag-icon"><InboxOutlined /></p>
      <p class="ant-upload-text">点击或拖拽文件到此上传</p>
      <p class="ant-upload-hint">单个文件不超过 200MB</p>
    </a-upload-dragger>

    <a-spin :spinning="loading">
      <a-empty v-if="!files.length" description="暂无文件" />
      <div v-else class="file-list">
        <div v-for="f in files" :key="f.path" class="file-row">
          <span class="file-icon"><FileOutlined /></span>
          <div class="file-info">
            <div class="file-name">{{ f.path }}</div>
            <div class="file-meta">{{ fmtSize(f.size) }} · {{ f.mod_time }}</div>
          </div>
          <a-button size="small" @click="downloadFile(projectId, f.path)">
            <template #icon><DownloadOutlined /></template>
          </a-button>
          <a-popconfirm title="删除该文件？" @confirm="onDelete(f)">
            <a-button size="small" danger><template #icon><DeleteOutlined /></template></a-button>
          </a-popconfirm>
        </div>
      </div>
    </a-spin>
  </a-card>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import {
  InboxOutlined, FileOutlined, DownloadOutlined, DeleteOutlined, ReloadOutlined,
} from '@ant-design/icons-vue'
import { listFiles, uploadFile, deleteFile, downloadFile } from '../../api'

const props = defineProps({ projectId: { type: Number, required: true } })

const files = ref([])
const loading = ref(false)
let timer = null

const load = async () => {
  loading.value = true
  try {
    const { data } = await listFiles(props.projectId)
    files.value = data.files
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  await load()
  timer = setInterval(load, 8000) // agent 可能随时产出文件
})
onUnmounted(() => clearInterval(timer))

const onUpload = async ({ file, onSuccess, onError }) => {
  try {
    await uploadFile(props.projectId, file)
    message.success(`${file.name} 已上传`)
    onSuccess()
    await load()
  } catch (e) {
    onError(e)
  }
}

const onDelete = async (f) => {
  await deleteFile(props.projectId, f.path)
  message.success('已删除')
  await load()
}

const fmtSize = (n) => {
  if (n < 1024) return n + ' B'
  if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1024 / 1024).toFixed(1) + ' MB'
}
</script>

<style scoped>
.files-card { border-radius: 10px; }
.tip { font-size: 12px; color: #999; font-weight: normal; margin-left: 8px; }
.file-list { display: flex; flex-direction: column; gap: 8px; }
.file-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid #f0f0f0;
  border-radius: 8px;
}
.file-row .file-info { flex: 1; min-width: 0; }
.file-icon { color: #5b6cff; font-size: 18px; }
.file-name { word-break: break-all; font-weight: 500; }
.file-meta { color: #aaa; font-size: 12px; }
.file-row :deep(.ant-btn) { min-width: 32px; min-height: 32px; }
</style>
