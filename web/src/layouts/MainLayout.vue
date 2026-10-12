<template>
  <a-layout style="min-height: 100vh">
    <a-layout-sider
      v-model:collapsed="collapsed"
      :trigger="null"
      :breakpoint="'lg'"
      :collapsed-width="isMobile ? 0 : 80"
      theme="dark"
      width="208"
    >
      <div class="logo">
        <span class="logo-dot">H</span>
        <span v-if="!collapsed" class="logo-text">HermesHub</span>
      </div>
      <a-menu theme="dark" mode="inline" :selected-keys="selectedKeys" @click="onMenuClick">
        <a-menu-item key="/projects">
          <template #icon><ProjectOutlined /></template>
          项目
        </a-menu-item>
        <a-menu-item key="/instances">
          <template #icon><RobotOutlined /></template>
          Hermes 实例
        </a-menu-item>
      </a-menu>
    </a-layout-sider>

    <!-- 移动端抽屉遮罩 -->
    <div v-if="isMobile && !collapsed" class="drawer-mask" @click="collapsed = true" />

    <a-layout>
      <a-layout-header class="header">
        <MenuUnfoldOutlined v-if="collapsed" class="trigger" @click="collapsed = false" />
        <MenuFoldOutlined v-else class="trigger" @click="collapsed = true" />
        <span class="header-title">{{ title }}</span>
        <a-button type="text" @click="onLogout">
          <template #icon><LogoutOutlined /></template>
          <span v-if="!isMobile">退出</span>
        </a-button>
      </a-layout-header>
      <a-layout-content class="content">
        <router-view />
      </a-layout-content>
    </a-layout>
  </a-layout>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  ProjectOutlined, RobotOutlined, LogoutOutlined,
  MenuFoldOutlined, MenuUnfoldOutlined,
} from '@ant-design/icons-vue'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const collapsed = ref(false)

const width = ref(window.innerWidth)
const onResize = () => { width.value = window.innerWidth }
onMounted(() => {
  window.addEventListener('resize', onResize)
  collapsed.value = width.value < 992
})
onUnmounted(() => window.removeEventListener('resize', onResize))

const isMobile = computed(() => width.value < 992)
const selectedKeys = computed(() => {
  if (route.path.startsWith('/instances')) return ['/instances']
  return ['/projects']
})
const title = computed(() => (route.path.startsWith('/instances') ? 'Hermes 实例' : '项目'))

const onMenuClick = ({ key }) => {
  router.push(key)
  if (isMobile.value) collapsed.value = true
}
const onLogout = () => {
  auth.logout()
  router.push({ name: 'login' })
}
</script>

<style scoped>
.logo {
  display: flex;
  align-items: center;
  gap: 10px;
  height: 56px;
  padding: 0 16px;
  overflow: hidden;
}
.logo-dot {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(135deg, #5b6cff, #8e54e9);
  color: #fff;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.logo-text {
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  white-space: nowrap;
}
.header {
  background: #fff;
  padding: 0 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  box-shadow: 0 1px 4px rgba(0, 21, 41, 0.06);
  position: sticky;
  top: 0;
  z-index: 10;
}
.trigger {
  font-size: 17px;
  cursor: pointer;
  min-width: 44px;
  min-height: 44px;
  display: inline-flex;
  align-items: center;
}
.header-title {
  font-size: 16px;
  font-weight: 600;
  flex: 1;
}
.content {
  padding: 16px;
}
.drawer-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  z-index: 9;
}
@media (max-width: 991px) {
  .content {
    padding: 12px;
  }
  :deep(.ant-layout-sider) {
    position: fixed !important;
    left: 0;
    top: 0;
    bottom: 0;
    z-index: 20;
    height: 100vh;
  }
}
</style>
