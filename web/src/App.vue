<template>
  <a-config-provider :locale="zhCN" :theme="{ token: { colorPrimary: '#5b6cff' } }">
    <router-view />
  </a-config-provider>
</template>

<script setup>
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'

dayjs.locale('zh-cn')
</script>

<style>
html, body, #app {
  height: 100%;
  margin: 0;
  background: #f5f6fa;
}
/* 移动端安全区 */
.safe-bottom {
  padding-bottom: env(safe-area-inset-bottom, 0);
}
</style>
