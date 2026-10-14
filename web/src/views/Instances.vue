<template>
  <div>
    <div class="page-head">
      <div>
        <h2>Hermes 实例</h2>
        <p class="desc">注册 Hermes 实例后，在实例所在机器上运行连接器即可接入平台</p>
      </div>
      <a-button type="primary" @click="showCreate = true">
        <template #icon><PlusOutlined /></template>
        注册实例
      </a-button>
    </div>

    <a-spin :spinning="loading">
      <a-empty v-if="!instances.length" description="还没有实例，先注册一个吧" />
      <a-row v-else :gutter="[16, 16]">
        <a-col v-for="inst in instances" :key="inst.id" :xs="24" :sm="12" :lg="8" :xl="6">
          <a-card :title="inst.name" class="inst-card">
            <template #extra>
              <a-badge :status="inst.online ? 'success' : 'default'" :text="inst.online ? '在线' : '离线'" />
            </template>
            <p class="inst-desc">{{ inst.description || '（暂无描述）' }}</p>
            <div class="inst-actions">
              <a-button size="small" @click="openEdit(inst)">编辑</a-button>
              <a-button size="small" @click="showConnectInfo(inst)">接入信息</a-button>
              <a-button size="small" :loading="testingId === inst.id" @click="onTest(inst)">测试</a-button>
              <a-popconfirm title="删除实例会同时移出所有项目，确认？" @confirm="onDelete(inst)">
                <a-button size="small" danger>删除</a-button>
              </a-popconfirm>
            </div>
          </a-card>
        </a-col>
      </a-row>
    </a-spin>

    <!-- 新建 -->
    <a-modal v-model:open="showCreate" title="注册 Hermes 实例" @ok="onCreate" :confirm-loading="creating">
      <a-form layout="vertical">
        <a-form-item label="实例名称" required>
          <a-input v-model:value="form.name" placeholder="如：hermes-01（项目内 @ 用这个名字）" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="form.description" :rows="3" placeholder="这个实例的能力、部署位置等备注" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 编辑 -->
    <a-modal v-model:open="showEdit" title="编辑实例" @ok="onUpdate" :confirm-loading="creating">
      <a-form layout="vertical">
        <a-form-item label="实例名称" required>
          <a-input v-model:value="form.name" />
        </a-form-item>
        <a-form-item label="描述">
          <a-textarea v-model:value="form.description" :rows="3" />
        </a-form-item>
      </a-form>
    </a-modal>

    <!-- 接入信息 -->
    <a-modal v-model:open="showToken" title="连接器接入信息" :footer="null" width="640px">
      <a-alert type="info" show-icon message="在 Hermes 实例所在机器上执行以下步骤" style="margin-bottom: 12px" />
      <p class="mono-label">接入令牌（Token）：</p>
      <a-typography-paragraph copyable class="mono">{{ current?.token }}</a-typography-paragraph>
      <p class="mono-label">1. 创建配置文件 connector.yaml：</p>
      <pre class="code-block">{{ connectorYaml }}</pre>
      <p class="mono-label">2. 启动连接器：</p>
      <pre class="code-block">./hermeshub-connector --config connector.yaml</pre>
      <a-alert type="warning" show-icon message="acp 段填写 Hermes CLI 路径即可（Windows 示例：command: &quot;C:\\Users\\xx\\AppData\\Local\\hermes\\bin\\hermes.exe&quot;）；指令执行前请确保该机器上 hermes 已完成模型配置（hermes setup）。" />
    </a-modal>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { PlusOutlined } from '@ant-design/icons-vue'
import { listInstances, createInstance, updateInstance, deleteInstance, testInstance } from '../api'

const instances = ref([])
const loading = ref(false)
const creating = ref(false)
const testingId = ref(null)
const showCreate = ref(false)
const showEdit = ref(false)
const showToken = ref(false)
const current = ref(null)
const form = ref({ name: '', description: '' })

const connectorYaml = computed(() => {
  const origin = window.location.origin
  return `platform_url: "${origin}"
token: "${current.value?.token || ''}"
timeout_seconds: 1800
reconnect_seconds: 5
acp:
  command: "/path/to/hermes"      # Hermes CLI 路径
  args: ["acp", "--accept-hooks"]
  # work_dir: "D:\\\\work"        # 指令执行的工作目录（可选）`
})

const load = async () => {
  loading.value = true
  try {
    const { data } = await listInstances()
    instances.value = data.instances
  } finally {
    loading.value = false
  }
}
onMounted(load)

const onCreate = async () => {
  if (!form.value.name.trim()) {
    message.warning('请填写实例名称')
    return
  }
  creating.value = true
  try {
    const { data } = await createInstance(form.value)
    message.success('实例已注册')
    showCreate.value = false
    form.value = { name: '', description: '' }
    await load()
    showConnectInfo(data.instance)
  } catch {
  } finally {
    creating.value = false
  }
}

const openEdit = (inst) => {
  current.value = inst
  form.value = { name: inst.name, description: inst.description }
  showEdit.value = true
}

const onUpdate = async () => {
  creating.value = true
  try {
    await updateInstance(current.value.id, form.value)
    message.success('已保存')
    showEdit.value = false
    await load()
  } catch {
  } finally {
    creating.value = false
  }
}

const onDelete = async (inst) => {
  await deleteInstance(inst.id)
  message.success('已删除')
  await load()
}

const showConnectInfo = (inst) => {
  current.value = inst
  showToken.value = true
}

const onTest = async (inst) => {
  testingId.value = inst.id
  try {
    const { data } = await testInstance(inst.id)
    message.success(`连接正常，往返延迟 ${data.latency_ms} ms`)
  } catch {
    // 错误由 axios 拦截器统一提示
  } finally {
    testingId.value = null
  }
}
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
.inst-card { border-radius: 10px; }
.inst-desc {
  color: #666;
  min-height: 44px;
  word-break: break-all;
}
.inst-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.mono-label { font-weight: 600; margin: 12px 0 4px; }
.mono { font-family: monospace; word-break: break-all; }
.code-block {
  background: #f6f8fa;
  border: 1px solid #e8e8e8;
  border-radius: 8px;
  padding: 12px;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre;
}
</style>
