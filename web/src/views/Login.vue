<template>
  <div class="login-wrap">
    <a-card class="login-card">
      <div class="brand">
        <span class="brand-dot">H</span>
        <h1>HermesHub</h1>
      </div>
      <p class="subtitle">Hermes agent 集群协作平台</p>
      <a-form @submit.prevent="onSubmit">
        <a-form-item>
          <a-input-password
            v-model:value="password"
            size="large"
            placeholder="请输入管理密码"
            @press-enter="onSubmit"
          >
            <template #prefix><LockOutlined /></template>
          </a-input-password>
        </a-form-item>
        <a-button type="primary" size="large" block :loading="loading" @click="onSubmit">
          登录
        </a-button>
      </a-form>
    </a-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { message } from 'ant-design-vue'
import { LockOutlined } from '@ant-design/icons-vue'
import api from '../api'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const password = ref('')
const loading = ref(false)

const onSubmit = async () => {
  if (!password.value) {
    message.warning('请输入密码')
    return
  }
  loading.value = true
  try {
    const { data } = await api.post('/login', { password: password.value })
    auth.setToken(data.token)
    router.push({ path: '/' })
  } catch {
    // 拦截器已提示
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-wrap {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: linear-gradient(135deg, #eef0ff 0%, #f6f0ff 100%);
}
.login-card {
  width: 100%;
  max-width: 380px;
  border-radius: 12px;
  box-shadow: 0 8px 32px rgba(91, 108, 255, 0.12);
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  justify-content: center;
}
.brand-dot {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: linear-gradient(135deg, #5b6cff, #8e54e9);
  color: #fff;
  font-weight: 700;
  font-size: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.brand h1 {
  margin: 0;
  font-size: 22px;
}
.subtitle {
  text-align: center;
  color: #888;
  margin: 8px 0 24px;
}
</style>
